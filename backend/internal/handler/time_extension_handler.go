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

// TimeExtensionHandler 个别考生补时 HTTP 处理器（教师/管理员登记与撤销）。
type TimeExtensionHandler struct {
	svc    *service.TimeExtensionService
	logger *slog.Logger
}

// NewTimeExtensionHandler 构造补时处理器。
func NewTimeExtensionHandler(svc *service.TimeExtensionService, logger *slog.Logger) *TimeExtensionHandler {
	return &TimeExtensionHandler{svc: svc, logger: logger}
}

// Grant 教师为个别考生登记补时（分钟数 + 原因）。
func (h *TimeExtensionHandler) Grant(c *gin.Context) {
	examID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		Error(c, util.NewAppError(constants.CodeBadRequest, "补时模块：examId 参数非法"))
		return
	}
	var req dto.GrantTimeExtensionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, util.WrapAppError(constants.CodeValidationFailed, fmt.Sprintf("补时模块：登记补时参数校验失败（字段 student_id/extra_minutes/reason，补时限定 %d~%d 分钟）（%s）", constants.TimeExtensionMinMinutes, constants.TimeExtensionMaxMinutes, err.Error()), err))
		return
	}
	te, err := h.svc.Grant(c.Request.Context(), examID, &req, middleware.GetUserID(c), middleware.GetEmail(c))
	if err != nil {
		Error(c, err)
		return
	}
	SuccessMessage(c, constants.MsgExtensionGrantSuccess, dto.ToTimeExtensionResponse(te))
}

// Revoke 开考前撤销补时记录（开考后冻结）。
func (h *TimeExtensionHandler) Revoke(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("extId"))
	if err != nil {
		Error(c, util.NewAppError(constants.CodeBadRequest, "补时模块：补时记录 id 参数非法"))
		return
	}
	te, err := h.svc.Revoke(c.Request.Context(), id, middleware.GetEmail(c))
	if err != nil {
		Error(c, err)
		return
	}
	SuccessMessage(c, constants.MsgExtensionRevokeSuccess, dto.ToTimeExtensionResponse(te))
}
