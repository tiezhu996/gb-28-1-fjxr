package model

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TimeExtension 个别考生考试补时登记实体，集合 time_extensions。
// 状态枚举：active（有效）/ revoked（开考前撤销，状态机见 constants/enums.go）。
// 业务规则：一名考生在一场考试中只能有一条有效（active）记录（部分唯一索引兜底并发），
// 补时分钟数限定 5~60；开考前可撤销，开考后冻结。
// 枚举/字段出现位置：constants/enums.go、dto/time_extension.go、service/time_extension_service.go、
// repository/time_extension_repository.go、util/formatters.go、handler、前端 constants/types/页面。
type TimeExtension struct {
	ID            primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ExamID        primitive.ObjectID `bson:"exam_id" json:"exam_id"`
	ExamTitle     string             `bson:"exam_title" json:"exam_title"`
	StudentID     primitive.ObjectID `bson:"student_id" json:"student_id"`
	StudentName   string             `bson:"student_name" json:"student_name"`
	ExtraMinutes  int                `bson:"extra_minutes" json:"extra_minutes"` // 补时分钟（5~60）
	Reason        string             `bson:"reason" json:"reason"`               // 登记原因
	Status        string             `bson:"status" json:"status"`               // active / revoked
	GrantedBy     primitive.ObjectID `bson:"granted_by" json:"granted_by"`
	GrantedByName string             `bson:"granted_by_name" json:"granted_by_name"`
	RevokedAt     *time.Time         `bson:"revoked_at,omitempty" json:"revoked_at"`
	CreatedAt     time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt     time.Time          `bson:"updated_at" json:"updated_at"`
}
