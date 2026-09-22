package service

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/onlineexam/onlineexam/internal/constants"
	"github.com/onlineexam/onlineexam/internal/dto"
	"github.com/onlineexam/onlineexam/internal/model"
	"github.com/onlineexam/onlineexam/internal/util"
)

// extensionTestBed 装配补时测试所需的全套 service 与共享仓储。
type extensionTestBed struct {
	examSvc      *ExamService
	recordSvc    *ExamRecordService
	extensionSvc *TimeExtensionService
	examRepo     *fakeExamRepo
	userRepo     *fakeUserRepo
	teacher      primitive.ObjectID
	student      *model.User
}

func newExtensionTestBed(t *testing.T) *extensionTestBed {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	questionRepo := newFakeQuestionRepo()
	questionSvc := NewQuestionService(questionRepo, logger)
	examRepo := newFakeExamRepo()
	userRepo := newFakeUserRepo()
	examSvc := NewExamService(examRepo, questionSvc, logger)
	extensionSvc := NewTimeExtensionService(newFakeExtensionRepo(), examRepo, userRepo, nil, logger)
	recordSvc := NewExamRecordService(newFakeRecordRepo(), examSvc, extensionSvc, logger)
	extensionSvc.SetActiveRecordLookup(recordSvc)

	teacher := primitive.NewObjectID()
	student := &model.User{ID: primitive.NewObjectID(), Name: "张同学", Email: "stu@example.com", Role: constants.RoleStudent, Status: constants.UserStatusActive}
	if err := userRepo.Create(context.Background(), student); err != nil {
		t.Fatalf("seed student: %v", err)
	}
	return &extensionTestBed{examSvc: examSvc, recordSvc: recordSvc, extensionSvc: extensionSvc, examRepo: examRepo, userRepo: userRepo, teacher: teacher, student: student}
}

func (b *extensionTestBed) createPublishedExam(t *testing.T, startAt, endAt time.Time, durationMin int) *model.Exam {
	t.Helper()
	q, err := b.examSvc.question.Create(context.Background(), &dto.CreateQuestionRequest{
		Type: "single", Subject: "数学", KnowledgePoints: []string{"代数"},
		Difficulty: "easy", Content: "1+1=?", Answer: "B", Score: 5,
		Options: []dto.OptionInput{{Key: "A", Text: "1"}, {Key: "B", Text: "2"}},
	}, b.teacher)
	if err != nil {
		t.Fatalf("seed question: %v", err)
	}
	exam, err := b.examSvc.Create(context.Background(), &dto.CreateExamRequest{
		Title: "补时测试卷", Subject: "数学", DurationMin: durationMin, PassScore: 60,
		StartAt: startAt, EndAt: endAt,
		Questions: []dto.ExamQuestionInput{{QuestionID: q.ID.Hex()}},
	}, b.teacher)
	if err != nil {
		t.Fatalf("create exam: %v", err)
	}
	if _, err := b.examSvc.Publish(context.Background(), exam.ID, "t@example.com"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	return exam
}

// TestGrantAndRevokeBeforeStart：开考前可登记、可撤销；重复/越界/非学生请求被拒。
func TestGrantAndRevokeBeforeStart(t *testing.T) {
	b := newExtensionTestBed(t)
	now := time.Now()
	exam := b.createPublishedExam(t, now.Add(time.Hour), now.Add(3*time.Hour), 60)
	ctx := context.Background()

	// 正常登记
	te, err := b.extensionSvc.Grant(ctx, exam.ID, &dto.GrantTimeExtensionRequest{
		StudentID: b.student.ID.Hex(), ExtraMinutes: 15, Reason: "身体不适需要延时",
	}, b.teacher, "t@example.com")
	if err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	if te.Status != constants.TimeExtensionStatusActive || te.ExtraMinutes != 15 {
		t.Fatalf("grant result = %+v", te)
	}

	// 重复登记：一名考生一场考试仅一条有效记录
	if _, err := b.extensionSvc.Grant(ctx, exam.ID, &dto.GrantTimeExtensionRequest{
		StudentID: b.student.ID.Hex(), ExtraMinutes: 20, Reason: "再次申请",
	}, b.teacher, "t@example.com"); err == nil {
		t.Fatal("重复登记应失败")
	} else if appErr, ok := err.(*util.AppError); !ok || appErr.Code != constants.CodeExtensionExists {
		t.Fatalf("重复登记错误码 = %v, want CodeExtensionExists", err)
	}

	// 越界分钟数：下限 5
	if _, err := b.extensionSvc.Grant(ctx, exam.ID, &dto.GrantTimeExtensionRequest{
		StudentID: primitive.NewObjectID().Hex(), ExtraMinutes: 4, Reason: "越界下限",
	}, b.teacher, "t@example.com"); err == nil {
		t.Fatal("补时 4 分钟（低于 5）应失败")
	}
	// 越界分钟数：上限 60（用另一个考生避免唯一约束）
	other := &model.User{ID: primitive.NewObjectID(), Name: "李同学", Email: "li@example.com", Role: constants.RoleStudent, Status: constants.UserStatusActive}
	_ = b.userRepo.Create(ctx, other)
	if _, err := b.extensionSvc.Grant(ctx, exam.ID, &dto.GrantTimeExtensionRequest{
		StudentID: other.ID.Hex(), ExtraMinutes: 61, Reason: "越界上限",
	}, b.teacher, "t@example.com"); err == nil {
		t.Fatal("补时 61 分钟（超过 60）应失败")
	}
	// 非学生角色
	teacherUser := &model.User{ID: primitive.NewObjectID(), Name: "王老师", Email: "w@example.com", Role: constants.RoleTeacher, Status: constants.UserStatusActive}
	_ = b.userRepo.Create(ctx, teacherUser)
	if _, err := b.extensionSvc.Grant(ctx, exam.ID, &dto.GrantTimeExtensionRequest{
		StudentID: teacherUser.ID.Hex(), ExtraMinutes: 15, Reason: "给老师补时",
	}, b.teacher, "t@example.com"); err == nil {
		t.Fatal("给非学生角色登记补时应失败")
	}

	// 开考前可撤销
	revoked, err := b.extensionSvc.Revoke(ctx, te.ID, "t@example.com")
	if err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if revoked.Status != constants.TimeExtensionStatusRevoked || revoked.RevokedAt == nil {
		t.Fatalf("revoke result = %+v", revoked)
	}
	// 撤销后可重新登记（旧记录已非 active，不占唯一名额）
	if _, err := b.extensionSvc.Grant(ctx, exam.ID, &dto.GrantTimeExtensionRequest{
		StudentID: b.student.ID.Hex(), ExtraMinutes: 10, Reason: "重新登记",
	}, b.teacher, "t@example.com"); err != nil {
		t.Fatalf("撤销后重新登记应成功: %v", err)
	}
}

// TestRevokeFrozenAfterStart：开考后冻结，撤销被拒。
func TestRevokeFrozenAfterStart(t *testing.T) {
	b := newExtensionTestBed(t)
	now := time.Now()
	exam := b.createPublishedExam(t, now.Add(-time.Hour), now.Add(2*time.Hour), 60)
	ctx := context.Background()

	te, err := b.extensionSvc.Grant(ctx, exam.ID, &dto.GrantTimeExtensionRequest{
		StudentID: b.student.ID.Hex(), ExtraMinutes: 20, Reason: "开考后补时",
	}, b.teacher, "t@example.com")
	if err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	if _, err := b.extensionSvc.Revoke(ctx, te.ID, "t@example.com"); err == nil {
		t.Fatal("开考后撤销补时应被冻结拒绝")
	} else if appErr, ok := err.(*util.AppError); !ok || appErr.Code != constants.CodeExtensionFrozen {
		t.Fatalf("开考后撤销错误码 = %v, want CodeExtensionFrozen", err)
	}
}

// TestGrantOnDraftRejected：未发布试卷不能登记补时。
func TestGrantOnDraftRejected(t *testing.T) {
	b := newExtensionTestBed(t)
	now := time.Now()
	q, _ := b.examSvc.question.Create(context.Background(), &dto.CreateQuestionRequest{
		Type: "single", Subject: "数学", KnowledgePoints: []string{"代数"},
		Difficulty: "easy", Content: "1+1=?", Answer: "B", Score: 5,
		Options: []dto.OptionInput{{Key: "A", Text: "1"}, {Key: "B", Text: "2"}},
	}, b.teacher)
	exam, _ := b.examSvc.Create(context.Background(), &dto.CreateExamRequest{
		Title: "草稿卷", Subject: "数学", DurationMin: 60,
		StartAt: now.Add(time.Hour), EndAt: now.Add(3 * time.Hour),
		Questions: []dto.ExamQuestionInput{{QuestionID: q.ID.Hex()}},
	}, b.teacher)
	if _, err := b.extensionSvc.Grant(context.Background(), exam.ID, &dto.GrantTimeExtensionRequest{
		StudentID: b.student.ID.Hex(), ExtraMinutes: 15, Reason: "草稿补时",
	}, b.teacher, "t@example.com"); err == nil {
		t.Fatal("草稿试卷不应允许登记补时")
	}
}

// TestStartWithExtensionDeadlineAndAutoSubmit：补时考生开考快照补时、个人截止时间生效、
// 自动提交按个人截止时间；其他考生与试卷总分/答案不受影响。
func TestStartWithExtensionDeadlineAndAutoSubmit(t *testing.T) {
	b := newExtensionTestBed(t)
	now := time.Now()
	// 考试窗口宽裕，避免 end_at 截断个人时长；时长 30 分钟
	exam := b.createPublishedExam(t, now.Add(-2*time.Hour), now.Add(24*time.Hour), 30)
	ctx := context.Background()

	// 无补时考生先开考
	plain := &model.User{ID: primitive.NewObjectID(), Name: "普通同学", Email: "plain@example.com", Role: constants.RoleStudent, Status: constants.UserStatusActive}
	_ = b.userRepo.Create(ctx, plain)
	plainRec, err := b.recordSvc.StartExam(ctx, exam.ID, plain.ID, "普通同学")
	if err != nil {
		t.Fatalf("plain StartExam: %v", err)
	}

	// 给目标考生登记 30 分钟补时（考生尚未开考）
	if _, err := b.extensionSvc.Grant(ctx, exam.ID, &dto.GrantTimeExtensionRequest{
		StudentID: b.student.ID.Hex(), ExtraMinutes: 30, Reason: "慢速作答",
	}, b.teacher, "t@example.com"); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	rec, err := b.recordSvc.StartExam(ctx, exam.ID, b.student.ID, "张同学")
	if err != nil {
		t.Fatalf("StartExam() error = %v", err)
	}
	if rec.ExtraMinutes != 30 {
		t.Fatalf("snapshot extra minutes = %d, want 30", rec.ExtraMinutes)
	}
	if rec.DeadlineAt == nil {
		t.Fatal("补时考生个人截止时间快照为空")
	}
	// 个人截止 = 开考 + 30 分钟时长 + 30 分钟补时
	wantDeadline := rec.StartedAt.Add(60 * time.Minute)
	if diff := rec.DeadlineAt.Sub(wantDeadline); diff > time.Second || diff < -time.Second {
		t.Fatalf("补时考生截止 = %v, want %v", *rec.DeadlineAt, wantDeadline)
	}
	// 普通考生截止 = 开考 + 30 分钟，未受补时影响
	plainWant := plainRec.StartedAt.Add(30 * time.Minute)
	if plainRec.DeadlineAt == nil || plainRec.DeadlineAt.Sub(plainWant).Abs() > time.Second {
		t.Fatalf("普通考生截止被补时影响: %v want %v", plainRec.DeadlineAt, plainWant)
	}
	// 试卷总分与标准答案不变
	if exam.TotalScore != 5 || rec.Questions[0].CorrectAnswer != "B" {
		t.Fatal("补时不应改变试卷总分或标准答案")
	}

	// 模拟已开考 40 分钟：普通考生（截止 30 分）应被自动提交，补时考生（截止 60 分）保留
	fortyMinAgo := now.Add(-40 * time.Minute)
	plainRec.StartedAt = fortyMinAgo
	d1 := fortyMinAgo.Add(30 * time.Minute)
	plainRec.DeadlineAt = &d1
	if err := b.recordSvc.repo.Update(ctx, plainRec); err != nil {
		t.Fatalf("update plain rec: %v", err)
	}
	rec.StartedAt = fortyMinAgo
	d2 := fortyMinAgo.Add(60 * time.Minute)
	rec.DeadlineAt = &d2
	if err := b.recordSvc.repo.Update(ctx, rec); err != nil {
		t.Fatalf("update extension rec: %v", err)
	}
	count, err := b.recordSvc.AutoSubmitExpired(ctx, now)
	if err != nil {
		t.Fatalf("AutoSubmitExpired: %v", err)
	}
	if count != 1 {
		t.Fatalf("auto submit count = %d, want 1（仅普通考生超时）", count)
	}
	updatedPlain, _ := b.recordSvc.repo.FindByID(ctx, plainRec.ID)
	updatedExt, _ := b.recordSvc.repo.FindByID(ctx, rec.ID)
	if updatedPlain.Status != constants.RecordStatusSubmitted || !updatedPlain.AutoSubmitted {
		t.Fatal("普通考生应已自动提交")
	}
	if updatedExt.Status != constants.RecordStatusInProgress {
		t.Fatal("补时考生未到个人截止时间，不应被自动提交")
	}
}

// TestGrantDuringAttemptExtendsDeadline：考生开考后登记补时，进行中答卷截止时间立即顺延。
func TestGrantDuringAttemptExtendsDeadline(t *testing.T) {
	b := newExtensionTestBed(t)
	now := time.Now()
	exam := b.createPublishedExam(t, now.Add(-time.Hour), now.Add(24*time.Hour), 60)
	ctx := context.Background()

	rec, err := b.recordSvc.StartExam(ctx, exam.ID, b.student.ID, "张同学")
	if err != nil {
		t.Fatalf("StartExam: %v", err)
	}
	originalDeadline := *rec.DeadlineAt
	if _, err := b.extensionSvc.Grant(ctx, exam.ID, &dto.GrantTimeExtensionRequest{
		StudentID: b.student.ID.Hex(), ExtraMinutes: 15, Reason: "考中突发情况",
	}, b.teacher, "t@example.com"); err != nil {
		t.Fatalf("Grant during attempt: %v", err)
	}
	updated, _ := b.recordSvc.repo.FindByID(ctx, rec.ID)
	if updated.ExtraMinutes != 15 {
		t.Fatalf("extra minutes = %d, want 15", updated.ExtraMinutes)
	}
	want := originalDeadline.Add(15 * time.Minute)
	if updated.DeadlineAt == nil || updated.DeadlineAt.Sub(want).Abs() > time.Second {
		t.Fatalf("开考后登记补时截止时间未顺延: %v want %v", updated.DeadlineAt, want)
	}
}

// TestGrantAfterPersonalDeadlineRejected：交卷后不能补登。
func TestGrantAfterPersonalDeadlineRejected(t *testing.T) {
	b := newExtensionTestBed(t)
	now := time.Now()
	// 考试已结束（end_at 在过去），考生也没有答卷
	exam := b.createPublishedExam(t, now.Add(-2*time.Hour), now.Add(-time.Minute), 60)
	if _, err := b.extensionSvc.Grant(context.Background(), exam.ID, &dto.GrantTimeExtensionRequest{
		StudentID: b.student.ID.Hex(), ExtraMinutes: 15, Reason: "交卷后补登",
	}, b.teacher, "t@example.com"); err == nil {
		t.Fatal("交卷截止后不应允许补登补时")
	}
}

// TestGrantAfterSubmitRejected：考生已交卷（submitted）后不允许再登记补时。
func TestGrantAfterSubmitRejected(t *testing.T) {
	b := newExtensionTestBed(t)
	now := time.Now()
	exam := b.createPublishedExam(t, now.Add(-time.Hour), now.Add(24*time.Hour), 60)
	ctx := context.Background()

	rec, err := b.recordSvc.StartExam(ctx, exam.ID, b.student.ID, "张同学")
	if err != nil {
		t.Fatalf("StartExam: %v", err)
	}
	if _, err := b.recordSvc.Submit(ctx, rec.ID, []dto.AnswerInput{{QuestionID: rec.Questions[0].QuestionID.Hex(), Answer: "B"}}, 0, nil, false); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if _, err := b.extensionSvc.Grant(ctx, exam.ID, &dto.GrantTimeExtensionRequest{
		StudentID: b.student.ID.Hex(), ExtraMinutes: 15, Reason: "交卷后补登",
	}, b.teacher, "t@example.com"); err == nil {
		t.Fatal("考生已交卷后不应允许登记补时")
	}
}
