package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/onlineexam/onlineexam/internal/constants"
	"github.com/onlineexam/onlineexam/internal/dto"
	"github.com/onlineexam/onlineexam/internal/model"
	"github.com/onlineexam/onlineexam/internal/repository"
	"github.com/onlineexam/onlineexam/internal/util"
)

// ActiveRecordLookup 进行中答卷查询/延期能力（由 ExamRecordService 实现，避免与 record 服务循环依赖）。
type ActiveRecordLookup interface {
	// FindActiveRecord 查找某考生在某场考试的进行中答卷，无则返回 nil。
	FindActiveRecord(ctx context.Context, examID, studentID primitive.ObjectID) *model.ExamRecord
	// HasFinishedRecord 判断考生在该场考试是否已有已交卷（submitted/graded）答卷。
	HasFinishedRecord(ctx context.Context, examID, studentID primitive.ObjectID) bool
	// ApplyExtension 将补时分钟并入进行中答卷的个人截止时间快照（幂等：以当前截止时间为基准）。
	ApplyExtension(ctx context.Context, rec *model.ExamRecord, extraMin int, now time.Time) error
}

// TimeExtensionService 个别考生补时服务：交卷前登记补时分钟与原因、开考前撤销、
// 个人截止时间计算、考试详情/记录的补时信息装配。
// 不变量：一名考生一场考试仅一条 active 记录（应用层校验 + 部分唯一索引兜底并发）。
type TimeExtensionService struct {
	repo     repository.TimeExtensionRepository
	examRepo repository.ExamRepository
	userRepo repository.UserRepository
	records  ActiveRecordLookup // 可选：main 装配 recordSvc 后注入
	logger   *slog.Logger
}

// NewTimeExtensionService 构造补时服务。
func NewTimeExtensionService(repo repository.TimeExtensionRepository, examRepo repository.ExamRepository, userRepo repository.UserRepository, records ActiveRecordLookup, logger *slog.Logger) *TimeExtensionService {
	return &TimeExtensionService{repo: repo, examRepo: examRepo, userRepo: userRepo, records: records, logger: logger}
}

// SetActiveRecordLookup 注入进行中答卷查询/延期能力（main 中 recordSvc 先于补时服务创建，此处补设）。
func (s *TimeExtensionService) SetActiveRecordLookup(lookup ActiveRecordLookup) {
	s.records = lookup
}

// Grant 为个别考生登记补时。
// 规则：试卷须已发布（published/ongoing，draft/closed/finished 不允许）；
// 补时 5~60 分钟且原因必填；必须在该考生个人交卷截止之前登记（交卷后不可补登）；
// 一场考试同一考生只能有一条有效记录（重复/并发请求不新增）。
func (s *TimeExtensionService) Grant(ctx context.Context, examID primitive.ObjectID, req *dto.GrantTimeExtensionRequest, operatorID primitive.ObjectID, operatorName string) (*model.TimeExtension, error) {
	exam, err := s.examRepo.FindByID(ctx, examID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(constants.CodeExamNotFound, fmt.Sprintf(constants.MsgExamNotFound, examID.Hex()))
		}
		return nil, fmt.Errorf("time extension service grant find exam: %w", err)
	}
	if exam.Status != constants.ExamStatusPublished && exam.Status != constants.ExamStatusOngoing {
		return nil, util.NewAppError(constants.CodeExamStatusErr, fmt.Sprintf(constants.MsgExamStatusInvalid, exam.Status, exam.Status, "grant-time-extension"))
	}
	if !constants.IsValidExtraMinutes(req.ExtraMinutes) {
		return nil, util.NewAppError(constants.CodeExtensionBadRequest, fmt.Sprintf(constants.MsgExtensionMinutesInvalid, req.ExtraMinutes, constants.TimeExtensionMinMinutes, constants.TimeExtensionMaxMinutes))
	}

	studentID, err := primitive.ObjectIDFromHex(req.StudentID)
	if err != nil {
		return nil, util.NewAppError(constants.CodeExtensionBadRequest, fmt.Sprintf(constants.MsgExtensionStudentInvalid, req.StudentID))
	}
	student, err := s.userRepo.FindByID(ctx, studentID)
	if err != nil || student.Role != constants.RoleStudent {
		return nil, util.NewAppError(constants.CodeExtensionBadRequest, fmt.Sprintf(constants.MsgExtensionStudentInvalid, req.StudentID))
	}

	// 已存在有效记录：重复请求不新增（一名考生一场考试仅一条有效记录）
	if existing, err := s.repo.FindActiveByExamAndStudent(ctx, examID, studentID); err == nil && existing != nil {
		return nil, util.NewAppError(constants.CodeExtensionExists, fmt.Sprintf(constants.MsgExtensionExists, studentID.Hex(), examID.Hex()))
	}

	// 交卷前登记：存在进行中答卷时不得晚于其个人截止时间；未开考则不得晚于统一结束时间。
	now := time.Now()
	var activeRecord *model.ExamRecord
	if s.records != nil {
		activeRecord = s.records.FindActiveRecord(ctx, examID, studentID)
		// 该考生已交卷（submitted/graded）：不允许事后补登
		if activeRecord == nil && s.records.HasFinishedRecord(ctx, examID, studentID) {
			return nil, util.NewAppError(constants.CodeRecordAlreadyDone, constants.MsgRecordExpired)
		}
	}
	registerDeadline := exam.EndAt
	if activeRecord != nil {
		registerDeadline = RecordPersonalDeadline(activeRecord, exam.DurationMin)
	}
	if !now.Before(registerDeadline) {
		return nil, util.NewAppError(constants.CodeRecordExpired, constants.MsgRecordExpired)
	}

	te := &model.TimeExtension{
		ID:            primitive.NewObjectID(),
		ExamID:        exam.ID,
		ExamTitle:     exam.Title,
		StudentID:     studentID,
		StudentName:   student.Name,
		ExtraMinutes:  req.ExtraMinutes,
		Reason:        req.Reason,
		Status:        constants.TimeExtensionStatusActive,
		GrantedBy:     operatorID,
		GrantedByName: operatorName,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.repo.Create(ctx, te); err != nil {
		if errors.Is(err, repository.ErrConflict) {
			// 并发登记：部分唯一索引拒绝第二条有效记录
			return nil, util.NewAppError(constants.CodeExtensionExists, fmt.Sprintf(constants.MsgExtensionExists, studentID.Hex(), examID.Hex()))
		}
		return nil, fmt.Errorf("time extension service grant create: %w", err)
	}

	// 考生已在考：立即延长其进行中答卷的个人截止时间（“开考后冻结”仅指撤销/修改登记，收卷仍按个人截止时间）
	if activeRecord != nil && s.records != nil {
		if err := s.records.ApplyExtension(ctx, activeRecord, te.ExtraMinutes, now); err != nil {
			s.logger.Warn("延长进行中答卷截止时间失败", "error", err.Error(), "record_id", activeRecord.ID.Hex())
		}
	}

	s.logger.Info(constants.LogTimeExtensionGranted, "exam_id", examID.Hex(), "student", student.Name, "extra_minutes", te.ExtraMinutes, "operator", operatorName)
	return te, nil
}

// Revoke 撤销补时记录：仅开考前允许（开考后冻结）。
// 开考前不存在答卷，因此撤销不影响任何收卷截止时间。
func (s *TimeExtensionService) Revoke(ctx context.Context, id primitive.ObjectID, operatorName string) (*model.TimeExtension, error) {
	te, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(constants.CodeExtensionNotFound, fmt.Sprintf(constants.MsgExtensionNotFound, id.Hex()))
		}
		return nil, fmt.Errorf("time extension service revoke find: %w", err)
	}
	if te.Status != constants.TimeExtensionStatusActive {
		return nil, util.NewAppError(constants.CodeExtensionNotFound, fmt.Sprintf(constants.MsgExtensionNotFound, id.Hex()))
	}
	exam, err := s.examRepo.FindByID(ctx, te.ExamID)
	if err != nil {
		return nil, fmt.Errorf("time extension service revoke find exam: %w", err)
	}
	// 开考后冻结：统一开考时间到达后不可撤销
	if !time.Now().Before(exam.StartAt) {
		return nil, util.NewAppError(constants.CodeExtensionFrozen, constants.MsgExtensionFrozen)
	}
	now := time.Now()
	te.Status = constants.TimeExtensionStatusRevoked
	te.RevokedAt = &now
	te.UpdatedAt = now
	if err := s.repo.Update(ctx, te); err != nil {
		return nil, fmt.Errorf("time extension service revoke update: %w", err)
	}
	s.logger.Info(constants.LogTimeExtensionRevoked, "extension_id", te.ID.Hex(), "exam_id", te.ExamID.Hex(), "student", te.StudentName, "operator", operatorName)
	return te, nil
}

// ListByExam 教师/管理员查看某场考试的全部补时记录（含已撤销），并填充个人截止时间。
func (s *TimeExtensionService) ListByExam(ctx context.Context, examID primitive.ObjectID) ([]dto.TimeExtensionResponse, error) {
	list, err := s.repo.ListByExam(ctx, examID)
	if err != nil {
		return nil, fmt.Errorf("time extension service list by exam: %w", err)
	}
	exam, err := s.examRepo.FindByID(ctx, examID)
	if err != nil {
		return nil, fmt.Errorf("time extension service list find exam: %w", err)
	}
	out := make([]dto.TimeExtensionResponse, 0, len(list))
	for _, te := range list {
		resp := dto.ToTimeExtensionResponse(te)
		deadline := s.deadlineFor(ctx, exam, te)
		resp.DeadlineAt = &deadline
		out = append(out, resp)
	}
	return out, nil
}

// ListMineByExam 学生查看本人在某场考试的补时记录（仅 active）。
func (s *TimeExtensionService) ListMineByExam(ctx context.Context, examID, studentID primitive.ObjectID) ([]dto.TimeExtensionResponse, error) {
	te, err := s.repo.FindActiveByExamAndStudent(ctx, examID, studentID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return []dto.TimeExtensionResponse{}, nil
		}
		return nil, fmt.Errorf("time extension service list mine: %w", err)
	}
	exam, err := s.examRepo.FindByID(ctx, examID)
	if err != nil {
		return nil, fmt.Errorf("time extension service list mine find exam: %w", err)
	}
	resp := dto.ToTimeExtensionResponse(te)
	deadline := s.deadlineFor(ctx, exam, te)
	resp.DeadlineAt = &deadline
	return []dto.TimeExtensionResponse{resp}, nil
}

// ActiveByExam 批量返回某场考试全部有效补时记录（供开考快照/自动提交使用）。
func (s *TimeExtensionService) ActiveByExam(ctx context.Context, examID primitive.ObjectID) ([]*model.TimeExtension, error) {
	list, err := s.repo.ListActiveByExam(ctx, examID)
	if err != nil {
		return nil, fmt.Errorf("time extension service active by exam: %w", err)
	}
	return list, nil
}

// FindActive 查询某考生在某场考试的有效补时记录，无则返回 nil（供开考快照使用）。
func (s *TimeExtensionService) FindActive(ctx context.Context, examID, studentID primitive.ObjectID) *model.TimeExtension {
	te, err := s.repo.FindActiveByExamAndStudent(ctx, examID, studentID)
	if err != nil {
		return nil
	}
	return te
}

// deadlineFor 计算补时考生的个人截止时间：已开考取答卷截止时间快照，未开考取计划截止时间。
func (s *TimeExtensionService) deadlineFor(ctx context.Context, exam *model.Exam, te *model.TimeExtension) time.Time {
	if s.records != nil {
		if rec := s.records.FindActiveRecord(ctx, exam.ID, te.StudentID); rec != nil {
			return RecordPersonalDeadline(rec, exam.DurationMin)
		}
	}
	return PlannedPersonalDeadline(exam, te.ExtraMinutes)
}

// PlannedPersonalDeadline 未开考考生的计划个人截止时间 = 统一结束时间 + 补时。
func PlannedPersonalDeadline(exam *model.Exam, extraMinutes int) time.Time {
	return exam.EndAt.Add(time.Duration(extraMinutes) * time.Minute)
}

// RecordPersonalDeadline 答卷个人收卷截止时间：有快照用快照；
// 旧记录（deadline_at 为空）按 开始时间 + 考试时长 兼容计算。
func RecordPersonalDeadline(rec *model.ExamRecord, durationMin int) time.Time {
	return dto.EnsureRecordDeadline(rec, durationMin)
}
