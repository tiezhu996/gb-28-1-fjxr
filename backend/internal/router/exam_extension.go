package router

import (
	"github.com/gin-gonic/gin"

	"github.com/onlineexam/onlineexam/internal/constants"
	"github.com/onlineexam/onlineexam/internal/handler"
	"github.com/onlineexam/onlineexam/internal/middleware"
)

// RegisterExamExtensionRoutes 个别考生补时模块路由。
// 教师/管理员：登记、撤销、按考试查询、考生选项；学生：查询本人补时记录与个人截止时间。
func RegisterExamExtensionRoutes(g *gin.RouterGroup, h *handler.ExamExtensionHandler) {
	teacher := g.Group("", middleware.RequireRoles(constants.RoleTeacher, constants.RoleAdmin))
	{
		teacher.GET("/exam-extension-students", h.ListStudents)
		teacher.GET("/exams/:id/extensions", h.ListByExam)
		teacher.POST("/exams/:id/extensions", h.Grant)
		teacher.DELETE("/exams/extensions/:extId", h.Revoke)
	}

	student := g.Group("/exams", middleware.RequireRoles(constants.RoleStudent))
	{
		student.GET("/:id/my-extension", h.Mine)
	}
}
