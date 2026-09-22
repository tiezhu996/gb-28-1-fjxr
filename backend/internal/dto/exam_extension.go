package dto

import (
	"time"

	"github.com/onlineexam/onlineexam/internal/model"
)

// GrantExtensionRequest 教师给个别考生登记补时请求。
// 业务规则：补时限于 5~60 分钟；须在交卷前登记分钟数与原因。
type GrantExtensionRequest struct {
	StudentID    string `json:"student_id" binding:"required"`
	ExtraMinutes int    `json:"extra_minutes" binding:"required,min=5,max=60"`
	Reason       string `json:"reason" binding:"required,min=1,max=500"`
}

// ExtensionResponse 补时记录响应（教师考试详情、撤销返回）。
type ExtensionResponse struct {
	ID           string     `json:"id"`
	ExamID       string     `json:"exam_id"`
	ExamTitle    string     `json:"exam_title"`
	StudentID    string     `json:"student_id"`
	StudentName  string     `json:"student_name"`
	ExtraMinutes int        `json:"extra_minutes"`
	Reason       string     `json:"reason"`
	Status       string     `json:"status"`
	StatusText   string     `json:"status_text"`
	GrantedBy    string     `json:"granted_by"`
	RevokedBy    string     `json:"revoked_by,omitempty"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// ToExtensionResponse 模型转响应。
func ToExtensionResponse(e *model.ExamExtension) ExtensionResponse {
	return ExtensionResponse{
		ID:           e.ID.Hex(),
		ExamID:       e.ExamID.Hex(),
		ExamTitle:    e.ExamTitle,
		StudentID:    e.StudentID.Hex(),
		StudentName:  e.StudentName,
		ExtraMinutes: e.ExtraMinutes,
		Reason:       e.Reason,
		Status:       e.Status,
		StatusText:   extensionStatusText(e.Status),
		GrantedBy:    e.GrantedBy,
		RevokedBy:    e.RevokedBy,
		RevokedAt:    e.RevokedAt,
		CreatedAt:    e.CreatedAt,
	}
}

// ToExtensionResponseList 批量转换。
func ToExtensionResponseList(list []*model.ExamExtension) []ExtensionResponse {
	out := make([]ExtensionResponse, 0, len(list))
	for _, e := range list {
		out = append(out, ToExtensionResponse(e))
	}
	return out
}

// MyExtensionResponse 学生查询本人补时记录响应（含按个人截止时间计算的字段）。
type MyExtensionResponse struct {
	ExamID        string    `json:"exam_id"`
	ExamTitle     string    `json:"exam_title"`
	ExtraMinutes  int       `json:"extra_minutes"`
	Status        string    `json:"status"`
	StatusText    string    `json:"status_text"`
	DurationMin   int       `json:"duration_min"`    // 含补时的个人总时长（原时长 + 补时）
	PersonalEndAt time.Time `json:"personal_end_at"` // 个人截止时间（原结束时间 + 补时，未开考前口径）
	HasInProgress bool      `json:"has_in_progress"` // 是否已开考（有进行中答卷）
	StartedAt     time.Time `json:"started_at,omitempty"`
	DeadlineAt    time.Time `json:"deadline_at,omitempty"` // 开考后个人截止时间（开考时间 + 个人总时长）
}

// extensionStatusText 避免 dto 直接循环依赖 util 的轻量映射（与 util.ExtensionStatusText 保持一致）。
func extensionStatusText(s string) string {
	switch s {
	case model.ExtensionStatusActive:
		return "有效"
	case model.ExtensionStatusRevoked:
		return "已撤销"
	default:
		return s
	}
}

// StudentOption 补时登记时的考生下拉选项（仅暴露 id/姓名/邮箱，不返回敏感信息）。
type StudentOption struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Status string `json:"status"`
}
