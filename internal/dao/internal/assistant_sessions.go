package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

type AssistantSessionsDao struct {
	table    string
	group    string
	columns  AssistantSessionsColumns
	handlers []gdb.ModelHandler
}

type AssistantSessionsColumns struct {
	SessionId          string
	UserCode           string
	ActiveAgent        string
	ActiveCheckpointId string
	Status             string
	Language           string
	CreatedAt          string
	UpdatedAt          string
}

var assistantSessionsColumns = AssistantSessionsColumns{
	SessionId:          "session_id",
	UserCode:           "user_code",
	ActiveAgent:        "active_agent",
	ActiveCheckpointId: "active_checkpoint_id",
	Status:             "status",
	Language:           "language",
	CreatedAt:          "created_at",
	UpdatedAt:          "updated_at",
}

func NewAssistantSessionsDao(handlers ...gdb.ModelHandler) *AssistantSessionsDao {
	return &AssistantSessionsDao{
		group:    "assistant",
		table:    "assistant_sessions",
		columns:  assistantSessionsColumns,
		handlers: handlers,
	}
}

func (dao *AssistantSessionsDao) DB() gdb.DB {
	return g.DB(dao.group)
}

func (dao *AssistantSessionsDao) Table() string {
	return dao.table
}

func (dao *AssistantSessionsDao) Group() string {
	return dao.group
}

func (dao *AssistantSessionsDao) Columns() AssistantSessionsColumns {
	return dao.columns
}

func (dao *AssistantSessionsDao) Ctx(ctx context.Context) *gdb.Model {
	model := dao.DB().Model(dao.table).Safe().Ctx(ctx)
	for _, handler := range dao.handlers {
		model = handler(model)
	}
	return model
}

func (dao *AssistantSessionsDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) error {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
