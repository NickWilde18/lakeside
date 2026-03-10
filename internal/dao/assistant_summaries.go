package dao

import "lakeside/internal/dao/internal"

type assistantSummariesDao struct {
	*internal.AssistantSummariesDao
}

var AssistantSummaries = assistantSummariesDao{
	AssistantSummariesDao: internal.NewAssistantSummariesDao(),
}
