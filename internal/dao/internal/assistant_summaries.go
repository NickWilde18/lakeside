package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

type AssistantSummariesDao struct {
	table    string
	group    string
	columns  AssistantSummariesColumns
	handlers []gdb.ModelHandler
}

type AssistantSummariesColumns struct {
	SessionId     string
	SummaryText   string
	LastMessageId string
	UpdatedAt     string
}

var assistantSummariesColumns = AssistantSummariesColumns{
	SessionId:     "session_id",
	SummaryText:   "summary_text",
	LastMessageId: "last_message_id",
	UpdatedAt:     "updated_at",
}

func NewAssistantSummariesDao(handlers ...gdb.ModelHandler) *AssistantSummariesDao {
	return &AssistantSummariesDao{
		group:    "assistant",
		table:    "assistant_summaries",
		columns:  assistantSummariesColumns,
		handlers: handlers,
	}
}

func (dao *AssistantSummariesDao) DB() gdb.DB {
	return g.DB(dao.group)
}

func (dao *AssistantSummariesDao) Table() string {
	return dao.table
}

func (dao *AssistantSummariesDao) Group() string {
	return dao.group
}

func (dao *AssistantSummariesDao) Columns() AssistantSummariesColumns {
	return dao.columns
}

func (dao *AssistantSummariesDao) Ctx(ctx context.Context) *gdb.Model {
	model := dao.DB().Model(dao.table).Safe().Ctx(ctx)
	for _, handler := range dao.handlers {
		model = handler(model)
	}
	return model
}

func (dao *AssistantSummariesDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) error {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
