package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/onlineexam/onlineexam/internal/constants"
	"github.com/onlineexam/onlineexam/internal/dto"
	"github.com/onlineexam/onlineexam/internal/model"
	"github.com/onlineexam/onlineexam/internal/repository"
	"github.com/onlineexam/onlineexam/internal/util"
)

// ExamExtensionService 个别考生补时服务：教师在交卷前登记补时、开考前撤销；学生查询个人补时。
// 业务规则：
//   - 仅已发布（published/ongoing）试卷可登记，且须在交卷前（now < 试卷 EndAt）；
//   - 补时限于 5~60 分钟，须登记分钟数与原因；
//   - 一名考生一场考试只能有一条有效（active）记录，重复/并发请求不能新增（唯一索引兜底）；
//   - 开考前（now < StartAt）教师可撤销，开考后冻结；
//   - 开考与收卷按个人截止时间，其他考生不受影响，试卷总分与标准答案不变。
type ExamExtensionService struct {
	repo   repository.ExamExtensionRepository
	exam   *ExamService
	user   *UserService
	record *ExamRecordService
	logger *slog.Logger
}

// NewExamExtensionService 构造补时服务。
func NewExamExtensionService(
	repo repository.ExamExtensionRepository,
	exam *ExamService,
	user *UserService,
	record *ExamRecordService,
	logger *slog.Logger,
) *ExamExtensionService {
	return &ExamExtensionService{repo: repo, exam: exam, user: user, record: record, logger: logger}
}

// Grant 教师给个别考生登记补时。
func (s *ExamExtensionService) Grant(ctx context.Context, examID, studentID primitive.ObjectID, extraMinutes int, reason, operator string) (*model.ExamExtension, error) {
	exam, err := s.exam.GetByID(ctx, examID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	// 试卷须已发布且尚未到交卷时间（交卷前登记）。
	if exam.Status != constants.ExamStatusPublished && exam.Status != constants.ExamStatusOngoing {
		return nil, util.NewAppError(constants.CodeExtensionExamState, fmt.Sprintf(constants.MsgExtensionExamState, examID.Hex(), exam.Status))
	}
	if !now.Before(exam.EndAt) {
		return nil, util.NewAppError(constants.CodeExtensionExamState, fmt.Sprintf(constants.MsgExtensionExamState, examID.Hex(), exam.Status))
	}
	// 越界请求不能新增：补时限于 5~60 分钟（binding 已校验，service 再兜底）。
	if !constants.IsValidExtensionMinutes(extraMinutes) {
		return nil, util.NewAppError(constants.CodeExtensionMinutesErr, fmt.Sprintf(constants.MsgExtensionMinutes, extraMinutes, constants.ExtensionMinMinutes, constants.ExtensionMaxMinutes))
	}
	// 目标必须是存在的学生。
	student, err := s.user.GetByID(ctx, studentID)
	if err != nil || student.Role != constants.RoleStudent {
		s.logger.Warn(constants.LogExtensionDenied, "exam_id", examID.Hex(), "student", studentID.Hex(), "extra_minutes", extraMinutes, "reason", "invalid_student", "operator", operator)
		return nil, util.NewAppError(constants.CodeExtensionStudentErr, fmt.Sprintf(constants.MsgExtensionStudent, studentID.Hex()))
	}

	// 重复请求不能新增：一名考生一场考试只能有一条有效记录。
	if existing, ferr := s.repo.FindActive(ctx, examID, studentID); ferr == nil && existing != nil {
		s.logger.Warn(constants.LogExtensionDenied, "exam_id", examID.Hex(), "student", studentID.Hex(), "extra_minutes", extraMinutes, "reason", "duplicate", "operator", operator)
		return nil, util.NewAppError(constants.CodeExtensionExists, fmt.Sprintf(constants.MsgExtensionExists, studentID.Hex(), examID.Hex()))
	}

	ext := &model.ExamExtension{
		ID:           primitive.NewObjectID(),
		ExamID:       exam.ID,
		ExamTitle:    exam.Title,
		StudentID:    student.ID,
		StudentName:  student.Name,
		ExtraMinutes: extraMinutes,
		Reason:       reason,
		Status:       constants.ExtensionStatusActive,
		GrantedBy:    operator,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.repo.Create(ctx, ext); err != nil {
		// 并发请求由唯一索引（partial: status=active）兜底，转成冲突业务错误。
		if errors.Is(err, repository.ErrConflict) {
			s.logger.Warn(constants.LogExtensionDenied, "exam_id", examID.Hex(), "student", studentID.Hex(), "extra_minutes", extraMinutes, "reason", "concurrent", "operator", operator)
			return nil, util.NewAppError(constants.CodeExtensionExists, fmt.Sprintf(constants.MsgExtensionExists, studentID.Hex(), examID.Hex()))
		}
		return nil, fmt.Errorf("exam extension service grant: %w", err)
	}

	// 同步进行中答卷的补时快照（已开考考生倒计时立即延长；未开考将在 StartExam 时读取）。
	if err := s.record.SyncExtensionSnapshot(ctx, examID, studentID, extraMinutes); err != nil {
		s.logger.Warn("同步答卷补时快照失败", "error", err.Error())
	}

	s.logger.Info(constants.LogExtensionGranted, "extension_id", ext.ID.Hex(), "exam_id", examID.Hex(), "student", student.Name, "extra_minutes", extraMinutes, "operator", operator)
	return ext, nil
}

// Revoke 开考前教师撤销补时；开考后冻结不可撤销。
func (s *ExamExtensionService) Revoke(ctx context.Context, extensionID primitive.ObjectID, operator string) (*model.ExamExtension, error) {
	ext, err := s.repo.FindByID(ctx, extensionID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(constants.CodeExtensionNotFound, fmt.Sprintf(constants.MsgExtensionNotFound, extensionID.Hex()))
		}
		return nil, fmt.Errorf("exam extension service revoke find: %w", err)
	}
	if ext.Status != constants.ExtensionStatusActive {
		return nil, util.NewAppError(constants.CodeExtensionNotFound, fmt.Sprintf(constants.MsgExtensionNotFound, extensionID.Hex()))
	}
	exam, err := s.exam.GetByID(ctx, ext.ExamID)
	if err != nil {
		return nil, err
	}
	// 开考前可撤销，开考后冻结。
	if !time.Now().Before(exam.StartAt) {
		return nil, util.NewAppError(constants.CodeExtensionFrozen, fmt.Sprintf(constants.MsgExtensionFrozen, ext.ExamID.Hex()))
	}
	now := time.Now()
	ext.Status = constants.ExtensionStatusRevoked
	ext.RevokedBy = operator
	ext.RevokedAt = &now
	ext.UpdatedAt = now
	if err := s.repo.Update(ctx, ext); err != nil {
		return nil, fmt.Errorf("exam extension service revoke: %w", err)
	}
	// 撤销后进行中答卷理论上不存在（开考前），仍同步兜底清零。
	if err := s.record.SyncExtensionSnapshot(ctx, ext.ExamID, ext.StudentID, 0); err != nil {
		s.logger.Warn("撤销后同步答卷补时快照失败", "error", err.Error())
	}
	s.logger.Info(constants.LogExtensionRevoked, "extension_id", ext.ID.Hex(), "exam_id", ext.ExamID.Hex(), "student", ext.StudentName, "operator", operator)
	return ext, nil
}

// ListByExam 教师查询某场考试全部补时记录。
func (s *ExamExtensionService) ListByExam(ctx context.Context, examID primitive.ObjectID) ([]*model.ExamExtension, error) {
	if _, err := s.exam.GetByID(ctx, examID); err != nil {
		return nil, err
	}
	list, err := s.repo.ListByExam(ctx, examID)
	if err != nil {
		return nil, fmt.Errorf("exam extension service list by exam: %w", err)
	}
	return list, nil
}

// ListStudents 返回启用状态的学生选项（教师登记补时选择考生用；不暴露密码等敏感字段）。
func (s *ExamExtensionService) ListStudents(ctx context.Context, keyword string) ([]dto.StudentOption, error) {
	filter := bson.M{"role": constants.RoleStudent, "status": constants.UserStatusActive}
	list, _, err := s.user.List(ctx, filter, 1, 500)
	if err != nil {
		return nil, fmt.Errorf("exam extension service list students: %w", err)
	}
	kw := strings.ToLower(strings.TrimSpace(keyword))
	out := make([]dto.StudentOption, 0, len(list))
	for _, u := range list {
		if kw != "" && !strings.Contains(strings.ToLower(u.Name), kw) && !strings.Contains(strings.ToLower(u.Email), kw) {
			continue
		}
		out = append(out, dto.StudentOption{ID: u.ID.Hex(), Name: u.Name, Email: u.Email, Status: u.Status})
	}
	return out, nil
}

// GetMine 学生查询本人在某场考试的有效补时记录（含个人截止时间），无记录返回 nil。
func (s *ExamExtensionService) GetMine(ctx context.Context, examID, studentID primitive.ObjectID) (*dto.MyExtensionResponse, error) {
	exam, err := s.exam.GetByID(ctx, examID)
	if err != nil {
		return nil, err
	}
	ext, err := s.repo.FindActive(ctx, examID, studentID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("exam extension service get mine: %w", err)
	}
	// 未开考口径：个人截止时间 = 原结束时间 + 补时（其他考生不受影响）。
	personalEnd := exam.EndAt.Add(time.Duration(ext.ExtraMinutes) * time.Minute)
	resp := &dto.MyExtensionResponse{
		ExamID:        examID.Hex(),
		ExamTitle:     exam.Title,
		ExtraMinutes:  ext.ExtraMinutes,
		Status:        ext.Status,
		StatusText:    util.ExtensionStatusText(ext.Status),
		DurationMin:   exam.DurationMin + ext.ExtraMinutes,
		PersonalEndAt: personalEnd,
	}
	// 已开考口径：个人截止时间 = 开考时间 + (试卷时长 + 补时)。
	if rec, rerr := s.record.repo.FindActiveByExamAndStudent(ctx, examID, studentID); rerr == nil && rec != nil {
		resp.HasInProgress = true
		resp.StartedAt = rec.StartedAt
		if !rec.DeadlineAt.IsZero() {
			resp.DeadlineAt = rec.DeadlineAt
		} else {
			resp.DeadlineAt = PersonalDeadline(rec.StartedAt, exam.DurationMin, ext.ExtraMinutes)
		}
	}
	return resp, nil
}
