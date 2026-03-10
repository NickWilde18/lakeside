package dao

import "lakeside/internal/dao/internal"

type assistantSessionsDao struct {
	*internal.AssistantSessionsDao
}

var AssistantSessions = assistantSessionsDao{
	AssistantSessionsDao: internal.NewAssistantSessionsDao(),
}
