package service

import (
	"context"
	"sort"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/onlineexam/onlineexam/internal/constants"
	"github.com/onlineexam/onlineexam/internal/model"
	"github.com/onlineexam/onlineexam/internal/repository"
)

// fakeExtensionRepo 内存版补时记录仓储（模拟部分唯一索引：同场同考生仅一条 active）。
type fakeExtensionRepo struct {
	items map[string]*model.TimeExtension
}

func newFakeExtensionRepo() *fakeExtensionRepo {
	return &fakeExtensionRepo{items: make(map[string]*model.TimeExtension)}
}

func (f *fakeExtensionRepo) Create(_ context.Context, te *model.TimeExtension) error {
	for _, ex := range f.items {
		if ex.ExamID == te.ExamID && ex.StudentID == te.StudentID && ex.Status == constants.TimeExtensionStatusActive {
			return repository.ErrConflict
		}
	}
	f.items[te.ID.Hex()] = te
	return nil
}

func (f *fakeExtensionRepo) Update(_ context.Context, te *model.TimeExtension) error {
	if _, ok := f.items[te.ID.Hex()]; !ok {
		return repository.ErrNotFound
	}
	f.items[te.ID.Hex()] = te
	return nil
}

func (f *fakeExtensionRepo) FindByID(_ context.Context, id primitive.ObjectID) (*model.TimeExtension, error) {
	if te, ok := f.items[id.Hex()]; ok {
		return te, nil
	}
	return nil, repository.ErrNotFound
}

func (f *fakeExtensionRepo) FindActiveByExamAndStudent(_ context.Context, examID, studentID primitive.ObjectID) (*model.TimeExtension, error) {
	for _, te := range f.items {
		if te.ExamID == examID && te.StudentID == studentID && te.Status == constants.TimeExtensionStatusActive {
			return te, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (f *fakeExtensionRepo) ListByExam(_ context.Context, examID primitive.ObjectID) ([]*model.TimeExtension, error) {
	var out []*model.TimeExtension
	for _, te := range f.items {
		if te.ExamID == examID {
			out = append(out, te)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (f *fakeExtensionRepo) ListActiveByExam(_ context.Context, examID primitive.ObjectID) ([]*model.TimeExtension, error) {
	var out []*model.TimeExtension
	for _, te := range f.items {
		if te.ExamID == examID && te.Status == constants.TimeExtensionStatusActive {
			out = append(out, te)
		}
	}
	return out, nil
}
