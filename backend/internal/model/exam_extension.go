package model

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// 补时记录状态枚举（TimeExtensionStatus）。
// 出现位置：constants/enums.go、model/exam_extension.go、dto/exam_extension.go、
// service/exam_extension_service.go、handler/exam_extension_handler.go、
// constants/error_codes.go、constants/log_templates.go、util/formatters.go、
// migrations/indexes.go、前端 src/constants/index.ts、src/utils/format.ts。
const (
	ExtensionStatusActive  = "active"  // 有效（开考前可撤销，开考后冻结）
	ExtensionStatusRevoked = "revoked" // 已撤销（开考前教师主动撤销）
)

// ExamExtension 个别考生补时记录，集合 exam_extensions。
// 业务规则：已发布试卷可给个别考生延时；一名考生一场考试只能有一条有效（active）记录；
// 补时限于 5~60 分钟；开考前教师可撤销，开考后冻结；重复/并发/越界请求不能新增。
type ExamExtension struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ExamID       primitive.ObjectID `bson:"exam_id" json:"exam_id"`
	ExamTitle    string             `bson:"exam_title" json:"exam_title"`
	StudentID    primitive.ObjectID `bson:"student_id" json:"student_id"`
	StudentName  string             `bson:"student_name" json:"student_name"`
	ExtraMinutes int                `bson:"extra_minutes" json:"extra_minutes"`     // 补时分钟数（5~60）
	Reason       string             `bson:"reason" json:"reason"`                   // 补时原因（交卷前登记）
	Status       string             `bson:"status" json:"status"`                   // active / revoked
	GrantedBy    string             `bson:"granted_by" json:"granted_by"`           // 登记教师邮箱
	RevokedBy    string             `bson:"revoked_by,omitempty" json:"revoked_by"` // 撤销教师邮箱
	RevokedAt    *time.Time         `bson:"revoked_at,omitempty" json:"revoked_at"`
	CreatedAt    time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt    time.Time          `bson:"updated_at" json:"updated_at"`
}
