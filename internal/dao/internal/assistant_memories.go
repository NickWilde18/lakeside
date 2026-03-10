package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

type AssistantMemoriesDao struct {
	table    string
	group    string
	columns  AssistantMemoriesColumns
	handlers []gdb.ModelHandler
}

type AssistantMemoriesColumns struct {
	Id              string
	UserCode        string
	Category        string
	CanonicalKey    string
	Content         string
	ValueJson       string
	Confidence      string
	SourceSessionId string
	SourceMessageId string
	Status          string
	CreatedAt       string
	UpdatedAt       string
}

var assistantMemoriesColumns = AssistantMemoriesColumns{
	Id:              "id",
	UserCode:        "user_code",
	Category:        "category",
	CanonicalKey:    "canonical_key",
	Content:         "content",
	ValueJson:       "value_json",
	Confidence:      "confidence",
	SourceSessionId: "source_session_id",
	SourceMessageId: "source_message_id",
	Status:          "status",
	CreatedAt:       "created_at",
	UpdatedAt:       "updated_at",
}

func NewAssistantMemoriesDao(handlers ...gdb.ModelHandler) *AssistantMemoriesDao {
	return &AssistantMemoriesDao{
		group:    "assistant",
		table:    "assistant_memories",
		columns:  assistantMemoriesColumns,
		handlers: handlers,
	}
}

func (dao *AssistantMemoriesDao) DB() gdb.DB {
	return g.DB(dao.group)
}

func (dao *AssistantMemoriesDao) Table() string {
	return dao.table
}

func (dao *AssistantMemoriesDao) Group() string {
	return dao.group
}

func (dao *AssistantMemoriesDao) Columns() AssistantMemoriesColumns {
	return dao.columns
}

func (dao *AssistantMemoriesDao) Ctx(ctx context.Context) *gdb.Model {
	model := dao.DB().Model(dao.table).Safe().Ctx(ctx)
	for _, handler := range dao.handlers {
		model = handler(model)
	}
	return model
}

func (dao *AssistantMemoriesDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) error {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
