package dao

import "lakeside/internal/dao/internal"

type assistantMemoriesDao struct {
	*internal.AssistantMemoriesDao
}

var AssistantMemories = assistantMemoriesDao{
	AssistantMemoriesDao: internal.NewAssistantMemoriesDao(),
}
