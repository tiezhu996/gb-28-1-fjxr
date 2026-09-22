package handler

import (
	"fmt"
	"log/slog"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/onlineexam/onlineexam/internal/constants"
	"github.com/onlineexam/onlineexam/internal/dto"
	"github.com/onlineexam/onlineexam/internal/middleware"
	"github.com/onlineexam/onlineexam/internal/service"
	"github.com/onlineexam/onlineexam/internal/util"
)

// ExamExtensionHandler 个别考生补时 HTTP 处理器。
type ExamExtensionHandler struct {
	svc    *service.ExamExtensionService
	logger *slog.Logger
}

// NewExamExtensionHandler 构造补时处理器。
func NewExamExtensionHandler(svc *service.ExamExtensionService, logger *slog.Logger) *ExamExtensionHandler {
	return &ExamExtensionHandler{svc: svc, logger: logger}
}

// Grant 教师给个别考生登记补时（须在交卷前登记分钟数与原因）。
func (h *ExamExtensionHandler) Grant(c *gin.Context) {
	examID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		Error(c, util.NewAppError(constants.CodeBadRequest, "补时模块：examId 参数非法"))
		return
	}
	var req dto.GrantExtensionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, util.WrapAppError(constants.CodeValidationFailed, fmt.Sprintf("补时模块：登记补时参数校验失败（字段 student_id/extra_minutes(5~60)/reason）（%s）", err.Error()), err))
		return
	}
	studentID, err := primitive.ObjectIDFromHex(req.StudentID)
	if err != nil {
		Error(c, util.NewAppError(constants.CodeBadRequest, "补时模块：student_id 参数非法"))
		return
	}
	ext, err := h.svc.Grant(c.Request.Context(), examID, studentID, req.ExtraMinutes, req.Reason, middleware.GetEmail(c))
	if err != nil {
		Error(c, err)
		return
	}
	SuccessMessage(c, constants.MsgExtensionGrantSuccess, dto.ToExtensionResponse(ext))
}

// Revoke 开考前撤销补时（开考后冻结）。
func (h *ExamExtensionHandler) Revoke(c *gin.Context) {
	extensionID, err := primitive.ObjectIDFromHex(c.Param("extId"))
	if err != nil {
		Error(c, util.NewAppError(constants.CodeBadRequest, "补时模块：补时记录 id 参数非法"))
		return
	}
	ext, err := h.svc.Revoke(c.Request.Context(), extensionID, middleware.GetEmail(c))
	if err != nil {
		Error(c, err)
		return
	}
	SuccessMessage(c, constants.MsgExtensionRevokeSuccess, dto.ToExtensionResponse(ext))
}

// ListByExam 教师查询某场考试全部补时记录。
func (h *ExamExtensionHandler) ListByExam(c *gin.Context) {
	examID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		Error(c, util.NewAppError(constants.CodeBadRequest, "补时模块：examId 参数非法"))
		return
	}
	list, err := h.svc.ListByExam(c.Request.Context(), examID)
	if err != nil {
		Error(c, err)
		return
	}
	Success(c, dto.ToExtensionResponseList(list))
}

// Mine 学生查询本人在某场考试的有效补时记录（含个人截止时间）。
func (h *ExamExtensionHandler) Mine(c *gin.Context) {
	examID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		Error(c, util.NewAppError(constants.CodeBadRequest, "补时模块：examId 参数非法"))
		return
	}
	resp, err := h.svc.GetMine(c.Request.Context(), examID, middleware.GetUserID(c))
	if err != nil {
		Error(c, err)
		return
	}
	Success(c, resp) // 无有效补时时 data 为 null
}

// ListStudents 教师登记补时时选择考生（仅返回启用状态学生，避免教师依赖管理员用户接口）。
func (h *ExamExtensionHandler) ListStudents(c *gin.Context) {
	list, err := h.svc.ListStudents(c.Request.Context(), c.Query("keyword"))
	if err != nil {
		Error(c, err)
		return
	}
	Success(c, list)
}
