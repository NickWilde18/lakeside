package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

type AssistantMessagesDao struct {
	table    string
	group    string
	columns  AssistantMessagesColumns
	handlers []gdb.ModelHandler
}

type AssistantMessagesColumns struct {
	Id           string
	SessionId    string
	UserCode     string
	Role         string
	Content      string
	PayloadJson  string
	ActiveAgent  string
	CheckpointId string
	Language     string
	CreatedAt    string
}

var assistantMessagesColumns = AssistantMessagesColumns{
	Id:           "id",
	SessionId:    "session_id",
	UserCode:     "user_code",
	Role:         "role",
	Content:      "content",
	PayloadJson:  "payload_json",
	ActiveAgent:  "active_agent",
	CheckpointId: "checkpoint_id",
	Language:     "language",
	CreatedAt:    "created_at",
}

func NewAssistantMessagesDao(handlers ...gdb.ModelHandler) *AssistantMessagesDao {
	return &AssistantMessagesDao{
		group:    "assistant",
		table:    "assistant_messages",
		columns:  assistantMessagesColumns,
		handlers: handlers,
	}
}

func (dao *AssistantMessagesDao) DB() gdb.DB {
	return g.DB(dao.group)
}

func (dao *AssistantMessagesDao) Table() string {
	return dao.table
}

func (dao *AssistantMessagesDao) Group() string {
	return dao.group
}

func (dao *AssistantMessagesDao) Columns() AssistantMessagesColumns {
	return dao.columns
}

func (dao *AssistantMessagesDao) Ctx(ctx context.Context) *gdb.Model {
	model := dao.DB().Model(dao.table).Safe().Ctx(ctx)
	for _, handler := range dao.handlers {
		model = handler(model)
	}
	return model
}

func (dao *AssistantMessagesDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) error {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
