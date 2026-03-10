package entity

import "time"

// AssistantSessions 是 assistant_sessions 表的实体定义。
type AssistantSessions struct {
	SessionId          string    `json:"sessionId" orm:"session_id"`
	UserCode           string    `json:"userCode" orm:"user_code"`
	ActiveAgent        string    `json:"activeAgent" orm:"active_agent"`
	ActiveCheckpointId string    `json:"activeCheckpointId" orm:"active_checkpoint_id"`
	Status             string    `json:"status" orm:"status"`
	Language           string    `json:"language" orm:"language"`
	CreatedAt          time.Time `json:"createdAt" orm:"created_at"`
	UpdatedAt          time.Time `json:"updatedAt" orm:"updated_at"`
}
