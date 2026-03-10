package dao

import "lakeside/internal/dao/internal"

type assistantMessagesDao struct {
	*internal.AssistantMessagesDao
}

var AssistantMessages = assistantMessagesDao{
	AssistantMessagesDao: internal.NewAssistantMessagesDao(),
}
