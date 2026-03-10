package entity

import "time"

// AssistantSummaries 是 assistant_summaries 表的实体定义。
type AssistantSummaries struct {
	SessionId     string    `json:"sessionId" orm:"session_id"`
	SummaryText   string    `json:"summaryText" orm:"summary_text"`
	LastMessageId int64     `json:"lastMessageId" orm:"last_message_id"`
	UpdatedAt     time.Time `json:"updatedAt" orm:"updated_at"`
}
