package entity

import "time"

// AssistantMemories 是 assistant_memories 表的实体定义。
type AssistantMemories struct {
	Id              int64     `json:"id" orm:"id"`
	UserCode        string    `json:"userCode" orm:"user_code"`
	Category        string    `json:"category" orm:"category"`
	CanonicalKey    string    `json:"canonicalKey" orm:"canonical_key"`
	Content         string    `json:"content" orm:"content"`
	ValueJson       string    `json:"valueJson" orm:"value_json"`
	Confidence      float64   `json:"confidence" orm:"confidence"`
	SourceSessionId string    `json:"sourceSessionId" orm:"source_session_id"`
	SourceMessageId int64     `json:"sourceMessageId" orm:"source_message_id"`
	Status          string    `json:"status" orm:"status"`
	CreatedAt       time.Time `json:"createdAt" orm:"created_at"`
	UpdatedAt       time.Time `json:"updatedAt" orm:"updated_at"`
}
