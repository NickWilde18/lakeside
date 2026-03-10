package entity

import "time"

// AssistantMessages 是 assistant_messages 表的实体定义。
type AssistantMessages struct {
	Id           int64     `json:"id" orm:"id"`
	SessionId    string    `json:"sessionId" orm:"session_id"`
	UserCode     string    `json:"userCode" orm:"user_code"`
	Role         string    `json:"role" orm:"role"`
	Content      string    `json:"content" orm:"content"`
	PayloadJson  string    `json:"payloadJson" orm:"payload_json"`
	ActiveAgent  string    `json:"activeAgent" orm:"active_agent"`
	CheckpointId string    `json:"checkpointId" orm:"checkpoint_id"`
	Language     string    `json:"language" orm:"language"`
	CreatedAt    time.Time `json:"createdAt" orm:"created_at"`
}
