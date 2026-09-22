package repository

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/onlineexam/onlineexam/internal/model"
)

// TimeExtensionRepository 个别考生补时记录仓储接口。
type TimeExtensionRepository interface {
	Create(ctx context.Context, te *model.TimeExtension) error
	Update(ctx context.Context, te *model.TimeExtension) error
	FindByID(ctx context.Context, id primitive.ObjectID) (*model.TimeExtension, error)
	// FindActiveByExamAndStudent 查询某考生在某场考试中的有效（active）补时记录。
	FindActiveByExamAndStudent(ctx context.Context, examID, studentID primitive.ObjectID) (*model.TimeExtension, error)
	// ListByExam 按试卷查询补时记录（教师/管理员可含已撤销记录）。
	ListByExam(ctx context.Context, examID primitive.ObjectID) ([]*model.TimeExtension, error)
	// ListActiveByExam 批量查询某场考试全部有效补时记录（开考快照/自动提交计算用）。
	ListActiveByExam(ctx context.Context, examID primitive.ObjectID) ([]*model.TimeExtension, error)
}

// MongoTimeExtensionRepository MongoDB 补时记录仓储实现。
type MongoTimeExtensionRepository struct {
	coll *mongo.Collection
}

// NewMongoTimeExtensionRepository 构造补时记录仓储。
func NewMongoTimeExtensionRepository(db *mongo.Database) *MongoTimeExtensionRepository {
	return &MongoTimeExtensionRepository{coll: db.Collection("time_extensions")}
}

func (r *MongoTimeExtensionRepository) Create(ctx context.Context, te *model.TimeExtension) error {
	_, err := r.coll.InsertOne(ctx, te)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			// 部分唯一索引 (exam_id, student_id) where status=active 兜底重复/并发请求
			return fmt.Errorf("create time extension: %w", ErrConflict)
		}
		return fmt.Errorf("create time extension: %w", err)
	}
	return nil
}

func (r *MongoTimeExtensionRepository) Update(ctx context.Context, te *model.TimeExtension) error {
	res, err := r.coll.ReplaceOne(ctx, bson.M{"_id": te.ID}, te)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("update time extension: %w", ErrConflict)
		}
		return fmt.Errorf("update time extension: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("update time extension: %w", ErrNotFound)
	}
	return nil
}

func (r *MongoTimeExtensionRepository) FindByID(ctx context.Context, id primitive.ObjectID) (*model.TimeExtension, error) {
	var te model.TimeExtension
	if err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&te); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("find time extension by id: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("find time extension by id: %w", err)
	}
	return &te, nil
}

func (r *MongoTimeExtensionRepository) FindActiveByExamAndStudent(ctx context.Context, examID, studentID primitive.ObjectID) (*model.TimeExtension, error) {
	var te model.TimeExtension
	err := r.coll.FindOne(ctx, bson.M{
		"exam_id":    examID,
		"student_id": studentID,
		"status":     timeExtensionStatusActive(),
	}).Decode(&te)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("find active time extension: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("find active time extension: %w", err)
	}
	return &te, nil
}

func (r *MongoTimeExtensionRepository) ListByExam(ctx context.Context, examID primitive.ObjectID) ([]*model.TimeExtension, error) {
	opts := options.Find().SetSort(bson.M{"created_at": -1})
	cur, err := r.coll.Find(ctx, bson.M{"exam_id": examID}, opts)
	if err != nil {
		return nil, fmt.Errorf("list time extensions by exam: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()
	var list []*model.TimeExtension
	if err := cur.All(ctx, &list); err != nil {
		return nil, fmt.Errorf("decode time extensions: %w", err)
	}
	return list, nil
}

func (r *MongoTimeExtensionRepository) ListActiveByExam(ctx context.Context, examID primitive.ObjectID) ([]*model.TimeExtension, error) {
	cur, err := r.coll.Find(ctx, bson.M{"exam_id": examID, "status": timeExtensionStatusActive()})
	if err != nil {
		return nil, fmt.Errorf("list active time extensions by exam: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()
	var list []*model.TimeExtension
	if err := cur.All(ctx, &list); err != nil {
		return nil, fmt.Errorf("decode active time extensions: %w", err)
	}
	return list, nil
}

// timeExtensionStatusActive 避免 repository 直接依赖 constants（与 exam_record_repository.go 同约定）。
// 注意：该状态枚举同时在 constants/enums.go、model/time_extension.go、service、formatters、前端 constants 中出现。
func timeExtensionStatusActive() string {
	return "active"
}
