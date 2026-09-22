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
	"github.com/onlineexam/onlineexam/internal/repository"
	"github.com/onlineexam/onlineexam/internal/util"
)

// fakeExtensionRepo 内存版补时记录仓储（并发唯一冲突可通过 failNextCreate 模拟）。
type fakeExtensionRepo struct {
	items          map[string]*model.ExamExtension
	failNextCreate bool
}

func newFakeExtensionRepo() *fakeExtensionRepo {
	return &fakeExtensionRepo{items: make(map[string]*model.ExamExtension)}
}

func (f *fakeExtensionRepo) Create(_ context.Context, e *model.ExamExtension) error {
	if f.failNextCreate {
		f.failNextCreate = false
		return repository.ErrConflict
	}
	f.items[e.ID.Hex()] = e
	return nil
}
func (f *fakeExtensionRepo) Update(_ context.Context, e *model.ExamExtension) error {
	if _, ok := f.items[e.ID.Hex()]; !ok {
		return repository.ErrNotFound
	}
	f.items[e.ID.Hex()] = e
	return nil
}
func (f *fakeExtensionRepo) FindByID(_ context.Context, id primitive.ObjectID) (*model.ExamExtension, error) {
	if e, ok := f.items[id.Hex()]; ok {
		return e, nil
	}
	return nil, repository.ErrNotFound
}
func (f *fakeExtensionRepo) FindActive(_ context.Context, examID, studentID primitive.ObjectID) (*model.ExamExtension, error) {
	for _, e := range f.items {
		if e.ExamID == examID && e.StudentID == studentID && e.Status == constants.ExtensionStatusActive {
			return e, nil
		}
	}
	return nil, repository.ErrNotFound
}
func (f *fakeExtensionRepo) ListByExam(_ context.Context, examID primitive.ObjectID) ([]*model.ExamExtension, error) {
	var out []*model.ExamExtension
	for _, e := range f.items {
		if e.ExamID == examID {
			out = append(out, e)
		}
	}
	return out, nil
}
func (f *fakeExtensionRepo) ListActiveByExam(_ context.Context, examID primitive.ObjectID) ([]*model.ExamExtension, error) {
	var out []*model.ExamExtension
	for _, e := range f.items {
		if e.ExamID == examID && e.Status == constants.ExtensionStatusActive {
			out = append(out, e)
		}
	}
	return out, nil
}

// extensionTestDeps 组装补时服务测试依赖。
type extensionTestDeps struct {
	svc       *ExamExtensionService
	recordSvc *ExamRecordService
	userSvc   *UserService
	examRepo  *fakeExamRepo
	extRepo   *fakeExtensionRepo
	teacher   primitive.ObjectID
	student   *model.User
	exam      *model.Exam
}

func setupExtensionDeps(t *testing.T, startAt, endAt time.Time) extensionTestDeps {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	userRepo := newFakeUserRepo()
	userSvc := newTestUserSvc(userRepo)
	teacher := primitive.NewObjectID()
	student := &model.User{ID: primitive.NewObjectID(), Name: "李同学", Email: "stu@example.com", Role: constants.RoleStudent, Status: constants.UserStatusActive}
	_ = userRepo.Create(context.Background(), student)

	questionRepo := newFakeQuestionRepo()
	questionSvc := NewQuestionService(questionRepo, logger)
	q, err := questionSvc.Create(context.Background(), &dto.CreateQuestionRequest{
		Type: "single", Subject: "数学", KnowledgePoints: []string{"代数"},
		Difficulty: "easy", Content: "1+1=?", Answer: "B", Score: 5,
		Options: []dto.OptionInput{{Key: "A", Text: "1"}, {Key: "B", Text: "2"}},
	}, teacher)
	if err != nil {
		t.Fatalf("create question: %v", err)
	}

	examRepo := newFakeExamRepo()
	examSvc := NewExamService(examRepo, questionSvc, logger)
	exam, err := examSvc.Create(context.Background(), &dto.CreateExamRequest{
		Title: "补时测试卷", Subject: "数学", DurationMin: 30, PassScore: 60,
		StartAt: startAt, EndAt: endAt,
		Questions: []dto.ExamQuestionInput{{QuestionID: q.ID.Hex()}},
	}, teacher)
	if err != nil {
		t.Fatalf("create exam: %v", err)
	}
	if _, err := examSvc.Publish(context.Background(), exam.ID, "t@example.com"); err != nil {
		t.Fatalf("publish: %v", err)
	}

	recordRepo := newFakeRecordRepo()
	recordSvc := NewExamRecordService(recordRepo, examSvc, logger)
	extRepo := newFakeExtensionRepo()
	recordSvc.SetExtensionRepo(extRepo)
	svc := NewExamExtensionService(extRepo, examSvc, userSvc, recordSvc, logger)

	return extensionTestDeps{
		svc: svc, recordSvc: recordSvc, userSvc: userSvc, examRepo: examRepo, extRepo: extRepo,
		teacher: teacher, student: student, exam: examRepo.exams[exam.ID.Hex()],
	}
}

func TestGrantExtensionSuccess(t *testing.T) {
	now := time.Now()
	d := setupExtensionDeps(t, now.Add(-time.Hour), now.Add(time.Hour))

	ext, err := d.svc.Grant(context.Background(), d.exam.ID, d.student.ID, 20, "网络故障", "t@example.com")
	if err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	if ext.ExtraMinutes != 20 || ext.Status != constants.ExtensionStatusActive {
		t.Fatalf("extension = %+v", ext)
	}

	mine, err := d.svc.GetMine(context.Background(), d.exam.ID, d.student.ID)
	if err != nil {
		t.Fatalf("GetMine() error = %v", err)
	}
	if mine == nil || mine.ExtraMinutes != 20 || mine.DurationMin != 50 {
		t.Fatalf("mine = %+v", mine)
	}
}

func TestGrantDuplicateRejected(t *testing.T) {
	now := time.Now()
	d := setupExtensionDeps(t, now.Add(-time.Hour), now.Add(time.Hour))

	if _, err := d.svc.Grant(context.Background(), d.exam.ID, d.student.ID, 10, "原因A", "t@example.com"); err != nil {
		t.Fatalf("first Grant() error = %v", err)
	}
	// 重复请求不能新增。
	_, err := d.svc.Grant(context.Background(), d.exam.ID, d.student.ID, 15, "原因B", "t@example.com")
	if appErr, ok := err.(*util.AppError); !ok || appErr.Code != constants.CodeExtensionExists {
		t.Fatalf("duplicate grant err = %v, want CodeExtensionExists", err)
	}
}

func TestGrantConcurrentRejected(t *testing.T) {
	now := time.Now()
	d := setupExtensionDeps(t, now.Add(-time.Hour), now.Add(time.Hour))
	// 模拟并发：唯一索引冲突。
	d.extRepo.failNextCreate = true
	_, err := d.svc.Grant(context.Background(), d.exam.ID, d.student.ID, 10, "并发", "t@example.com")
	if appErr, ok := err.(*util.AppError); !ok || appErr.Code != constants.CodeExtensionExists {
		t.Fatalf("concurrent grant err = %v, want CodeExtensionExists", err)
	}
}

func TestGrantOutOfRangeRejected(t *testing.T) {
	now := time.Now()
	d := setupExtensionDeps(t, now.Add(-time.Hour), now.Add(time.Hour))
	for _, min := range []int{0, 4, 61, 100} {
		_, err := d.svc.Grant(context.Background(), d.exam.ID, d.student.ID, min, "越界", "t@example.com")
		if appErr, ok := err.(*util.AppError); !ok || appErr.Code != constants.CodeExtensionMinutesErr {
			t.Fatalf("min=%d err = %v, want CodeExtensionMinutesErr", min, err)
		}
	}
}

func TestGrantAfterSubmitWindowRejected(t *testing.T) {
	now := time.Now()
	// 已过交卷时间（EndAt）不能登记。
	d := setupExtensionDeps(t, now.Add(-2*time.Hour), now.Add(-time.Minute))
	_, err := d.svc.Grant(context.Background(), d.exam.ID, d.student.ID, 20, "迟交", "t@example.com")
	if appErr, ok := err.(*util.AppError); !ok || appErr.Code != constants.CodeExtensionExamState {
		t.Fatalf("late grant err = %v, want CodeExtensionExamState", err)
	}
}

func TestRevokeBeforeStartAllowedAfterStartFrozen(t *testing.T) {
	now := time.Now()
	// 尚未开考。
	d := setupExtensionDeps(t, now.Add(time.Hour), now.Add(3*time.Hour))
	ext, err := d.svc.Grant(context.Background(), d.exam.ID, d.student.ID, 30, "医疗原因", "t@example.com")
	if err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	revoked, err := d.svc.Revoke(context.Background(), ext.ID, "t@example.com")
	if err != nil {
		t.Fatalf("Revoke() before start error = %v", err)
	}
	if revoked.Status != constants.ExtensionStatusRevoked {
		t.Fatalf("status = %s", revoked.Status)
	}
	// 撤销后可重新登记。
	if _, err := d.svc.Grant(context.Background(), d.exam.ID, d.student.ID, 15, "重新登记", "t@example.com"); err != nil {
		t.Fatalf("re-grant after revoke error = %v", err)
	}

	// 开考后的补时记录冻结不可撤销。
	d2 := setupExtensionDeps(t, now.Add(-time.Hour), now.Add(time.Hour))
	ext2, _ := d2.svc.Grant(context.Background(), d2.exam.ID, d2.student.ID, 30, "医疗原因", "t@example.com")
	_, err = d2.svc.Revoke(context.Background(), ext2.ID, "t@example.com")
	if appErr, ok := err.(*util.AppError); !ok || appErr.Code != constants.CodeExtensionFrozen {
		t.Fatalf("revoke after start err = %v, want CodeExtensionFrozen", err)
	}
}

func TestPersonalDeadlineAndCountdown(t *testing.T) {
	now := time.Now()
	d := setupExtensionDeps(t, now.Add(-time.Hour), now.Add(time.Hour))

	// 先登记补时，再开考：快照含 20 分钟补时。
	if _, err := d.svc.Grant(context.Background(), d.exam.ID, d.student.ID, 20, "故障", "t@example.com"); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	rec, err := d.recordSvc.StartExam(context.Background(), d.exam.ID, d.student.ID, "李同学")
	if err != nil {
		t.Fatalf("StartExam() error = %v", err)
	}
	if rec.ExtraMinutes != 20 {
		t.Fatalf("snapshot extra = %d, want 20", rec.ExtraMinutes)
	}
	wantDeadline := rec.StartedAt.Add(50 * time.Minute)
	if !rec.DeadlineAt.Equal(wantDeadline) {
		t.Fatalf("deadline = %v, want %v", rec.DeadlineAt, wantDeadline)
	}

	resp := d.recordSvc.BuildResponse(context.Background(), rec)
	if resp.DurationMin != 50 || !resp.DeadlineAt.Equal(wantDeadline) {
		t.Fatalf("resp duration=%d deadline=%v", resp.DurationMin, resp.DeadlineAt)
	}
}

func TestOtherStudentUnaffectedAndGradingKept(t *testing.T) {
	now := time.Now()
	d := setupExtensionDeps(t, now.Add(-time.Hour), now.Add(time.Hour))

	// 给李同学补时 20 分钟。
	if _, err := d.svc.Grant(context.Background(), d.exam.ID, d.student.ID, 20, "故障", "t@example.com"); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	// 另一名学生不受影响：无补时记录，开考快照 ExtraMinutes=0、截止时间按原时长。
	other, err := d.userSvc.Create(context.Background(), "王同学", "other@example.com", "password123", constants.RoleStudent, "admin")
	if err != nil {
		t.Fatalf("create other student: %v", err)
	}
	mineOther, err := d.svc.GetMine(context.Background(), d.exam.ID, other.ID)
	if err != nil {
		t.Fatalf("GetMine() error = %v", err)
	}
	if mineOther != nil {
		t.Fatalf("其他考生不应有补时记录, got %+v", mineOther)
	}
	recOther, err := d.recordSvc.StartExam(context.Background(), d.exam.ID, other.ID, "王同学")
	if err != nil {
		t.Fatalf("other StartExam() error = %v", err)
	}
	if recOther.ExtraMinutes != 0 {
		t.Fatalf("其他考生补时快照 = %d, want 0", recOther.ExtraMinutes)
	}
	if want := recOther.StartedAt.Add(30 * time.Minute); !recOther.DeadlineAt.Equal(want) {
		t.Fatalf("其他考生截止时间 = %v, want %v", recOther.DeadlineAt, want)
	}

	// 获得补时的李同学开考快照为 50 分钟。
	recLi, err := d.recordSvc.StartExam(context.Background(), d.exam.ID, d.student.ID, "李同学")
	if err != nil {
		t.Fatalf("li StartExam() error = %v", err)
	}
	if recLi.ExtraMinutes != 20 {
		t.Fatalf("李同学补时快照 = %d, want 20", recLi.ExtraMinutes)
	}

	// 原有阅卷流程保留：正常提交与自动判分不受补时影响。
	submitted, err := d.recordSvc.Submit(context.Background(), recLi.ID,
		[]dto.AnswerInput{{QuestionID: recLi.Questions[0].QuestionID.Hex(), Answer: "B"}}, 0, nil, false)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if submitted.ObjectiveScore != 5 {
		t.Fatalf("objective score = %f, want 5（标准答案与总分不变）", submitted.ObjectiveScore)
	}
}

func TestGrantInvalidStudentRejected(t *testing.T) {
	now := time.Now()
	d := setupExtensionDeps(t, now.Add(-time.Hour), now.Add(time.Hour))
	if _, err := d.svc.Grant(context.Background(), d.exam.ID, primitive.NewObjectID(), 20, "x", "t@example.com"); err == nil {
		t.Fatal("对不存在学生登记补时应失败")
	}
}

func TestListStudentsOnlyActiveStudents(t *testing.T) {
	now := time.Now()
	d := setupExtensionDeps(t, now.Add(time.Hour), now.Add(3*time.Hour))
	// 另一名正常学生。
	if _, err := d.userSvc.Create(context.Background(), "王同学", "wang@example.com", "password123", constants.RoleStudent, "admin"); err != nil {
		t.Fatalf("create wang: %v", err)
	}
	// 一名教师不应出现在考生选项中。
	if _, err := d.userSvc.Create(context.Background(), "陈老师", "chen@example.com", "password123", constants.RoleTeacher, "admin"); err != nil {
		t.Fatalf("create teacher: %v", err)
	}
	all, err := d.svc.ListStudents(context.Background(), "")
	if err != nil {
		t.Fatalf("ListStudents() error = %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("active students = %d, want 2", len(all))
	}
	filtered, err := d.svc.ListStudents(context.Background(), "wang")
	if err != nil || len(filtered) != 1 || filtered[0].Email != "wang@example.com" {
		t.Fatalf("keyword filter = %+v err=%v", filtered, err)
	}
}
