package router

import (
	"github.com/gin-gonic/gin"

	"github.com/onlineexam/onlineexam/internal/constants"
	"github.com/onlineexam/onlineexam/internal/handler"
	"github.com/onlineexam/onlineexam/internal/middleware"
)

// RegisterExamRoutes 试卷模块路由（教师/管理员维护，学生只读列表与详情）。
func RegisterExamRoutes(g *gin.RouterGroup, h *handler.ExamHandler, th *handler.TimeExtensionHandler) {
	g.GET("/exams", h.List)
	g.GET("/exams/:id", h.Get)

	write := g.Group("/exams", middleware.RequireRoles(constants.RoleTeacher, constants.RoleAdmin))
	{
		write.POST("", h.Create)
		write.POST("/auto-generate", h.AutoGenerate)
		write.POST("/:id/publish", h.Publish)
		write.POST("/:id/close", h.Close)
		write.PUT("/:id", h.Update)
		write.DELETE("/:id", h.Delete)
		// 个别考生补时：交卷前登记（分钟数+原因），开考前撤销，开考后冻结
		write.POST("/:id/time-extensions", th.Grant)
	}

	// 撤销补时：独立路径 /exam-time-extensions/:extId（避开 Gin 单段通配与 /exams/:id 冲突）
	g.POST("/exam-time-extensions/:extId/revoke", middleware.RequireRoles(constants.RoleTeacher, constants.RoleAdmin), th.Revoke)
}
