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

// ExamExtensionRepository 个别考生补时记录仓储接口。
type ExamExtensionRepository interface {
	Create(ctx context.Context, e *model.ExamExtension) error
	Update(ctx context.Context, e *model.ExamExtension) error
	FindByID(ctx context.Context, id primitive.ObjectID) (*model.ExamExtension, error)
	// FindActive 查找某考生在某场考试中唯一一条有效（active）补时记录。
	FindActive(ctx context.Context, examID, studentID primitive.ObjectID) (*model.ExamExtension, error)
	// ListByExam 列出某场考试全部补时记录（含已撤销，按登记时间倒序）。
	ListByExam(ctx context.Context, examID primitive.ObjectID) ([]*model.ExamExtension, error)
	// ListActiveByExam 列出某场考试全部有效补时记录（教师考试详情展示用）。
	ListActiveByExam(ctx context.Context, examID primitive.ObjectID) ([]*model.ExamExtension, error)
}

// MongoExamExtensionRepository MongoDB 补时记录仓储实现。
type MongoExamExtensionRepository struct {
	coll *mongo.Collection
}

// NewMongoExamExtensionRepository 构造补时记录仓储。
func NewMongoExamExtensionRepository(db *mongo.Database) *MongoExamExtensionRepository {
	return &MongoExamExtensionRepository{coll: db.Collection("exam_extensions")}
}

func (r *MongoExamExtensionRepository) Create(ctx context.Context, e *model.ExamExtension) error {
	_, err := r.coll.InsertOne(ctx, e)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("create exam extension: %w", ErrConflict)
		}
		return fmt.Errorf("create exam extension: %w", err)
	}
	return nil
}

func (r *MongoExamExtensionRepository) Update(ctx context.Context, e *model.ExamExtension) error {
	res, err := r.coll.ReplaceOne(ctx, bson.M{"_id": e.ID}, e)
	if err != nil {
		return fmt.Errorf("update exam extension: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("update exam extension: %w", ErrNotFound)
	}
	return nil
}

func (r *MongoExamExtensionRepository) FindByID(ctx context.Context, id primitive.ObjectID) (*model.ExamExtension, error) {
	var e model.ExamExtension
	if err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&e); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("find exam extension by id: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("find exam extension by id: %w", err)
	}
	return &e, nil
}

func (r *MongoExamExtensionRepository) FindActive(ctx context.Context, examID, studentID primitive.ObjectID) (*model.ExamExtension, error) {
	var e model.ExamExtension
	err := r.coll.FindOne(ctx, bson.M{
		"exam_id":    examID,
		"student_id": studentID,
		"status":     model.ExtensionStatusActive,
	}).Decode(&e)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("find active exam extension: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("find active exam extension: %w", err)
	}
	return &e, nil
}

func (r *MongoExamExtensionRepository) ListByExam(ctx context.Context, examID primitive.ObjectID) ([]*model.ExamExtension, error) {
	cur, err := r.coll.Find(ctx, bson.M{"exam_id": examID}, options.Find().SetSort(bson.M{"created_at": -1}))
	if err != nil {
		return nil, fmt.Errorf("list exam extensions by exam: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()
	var list []*model.ExamExtension
	if err := cur.All(ctx, &list); err != nil {
		return nil, fmt.Errorf("decode exam extensions: %w", err)
	}
	return list, nil
}

func (r *MongoExamExtensionRepository) ListActiveByExam(ctx context.Context, examID primitive.ObjectID) ([]*model.ExamExtension, error) {
	cur, err := r.coll.Find(ctx, bson.M{
		"exam_id": examID,
		"status":  model.ExtensionStatusActive,
	}, options.Find().SetSort(bson.M{"created_at": -1}))
	if err != nil {
		return nil, fmt.Errorf("list active exam extensions by exam: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()
	var list []*model.ExamExtension
	if err := cur.All(ctx, &list); err != nil {
		return nil, fmt.Errorf("decode active exam extensions: %w", err)
	}
	return list, nil
}
