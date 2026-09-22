package dto

import (
	"time"

	"github.com/onlineexam/onlineexam/internal/model"
)

// GrantTimeExtensionRequest 教师为个别考生登记补时请求。
// 补时分钟限定 5~60（binding + service 双重校验），原因为必填（交卷前须登记原因）。
type GrantTimeExtensionRequest struct {
	StudentID    string `json:"student_id" binding:"required"`
	ExtraMinutes int    `json:"extra_minutes" binding:"required,min=5,max=60"`
	Reason       string `json:"reason" binding:"required,min=2,max=500"`
}

// TimeExtensionResponse 补时记录响应。
// DeadlineAt 为该考生的个人截止时间（含补时）：开考后取答卷截止时间快照；
// 未开考时为计划截止时间（统一考试结束时间 + 补时）。
type TimeExtensionResponse struct {
	ID            string     `json:"id"`
	ExamID        string     `json:"exam_id"`
	ExamTitle     string     `json:"exam_title"`
	StudentID     string     `json:"student_id"`
	StudentName   string     `json:"student_name"`
	ExtraMinutes  int        `json:"extra_minutes"`
	Reason        string     `json:"reason"`
	Status        string     `json:"status"`
	DeadlineAt    *time.Time `json:"deadline_at"`
	GrantedByName string     `json:"granted_by_name"`
	RevokedAt     *time.Time `json:"revoked_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

// ToTimeExtensionResponse 模型转响应（不含个人截止时间，截止时间由 service 结合试卷/答卷填充）。
func ToTimeExtensionResponse(te *model.TimeExtension) TimeExtensionResponse {
	return TimeExtensionResponse{
		ID:            te.ID.Hex(),
		ExamID:        te.ExamID.Hex(),
		ExamTitle:     te.ExamTitle,
		StudentID:     te.StudentID.Hex(),
		StudentName:   te.StudentName,
		ExtraMinutes:  te.ExtraMinutes,
		Reason:        te.Reason,
		Status:        te.Status,
		GrantedByName: te.GrantedByName,
		RevokedAt:     te.RevokedAt,
		CreatedAt:     te.CreatedAt,
	}
}
