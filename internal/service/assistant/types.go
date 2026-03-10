package assistant

import (
	"context"
	"time"

	itsmv1 "lakeside/api/itsm/v1"
)

const (
	activeAgentAssistant = "assistant"
	activeAgentITSM      = "itsm"
)

const (
	statusActive = "active"
	statusDone   = "done"
)

type QueryRequest struct {
	UserCode string
	Message  string
}

type ResumeRequest struct {
	SessionID    string
	CheckpointID string
	UserCode     string
	Targets      map[string]*itsmv1.ResumeTarget
}

type ListMemoriesRequest struct {
	UserCode string
	Limit    int
}

type ClearMemoriesRequest struct {
	UserCode     string
	Category     string
	CanonicalKey string
}

type Response struct {
	Status       string
	SessionID    string
	CheckpointID string
	ActiveAgent  string
	Interrupts   []itsmv1.AgentInterrupt
	Result       *itsmv1.AgentResult
}

type SessionRecord struct {
	SessionID          string
	UserCode           string
	ActiveAgent        string
	ActiveCheckpointID string
	Status             string
	Language           string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type MessageRecord struct {
	ID           int64
	SessionID    string
	UserCode     string
	Role         string
	Content      string
	PayloadJSON  string
	ActiveAgent  string
	CheckpointID string
	Language     string
	CreatedAt    time.Time
}

type SummaryRecord struct {
	SessionID     string
	SummaryText   string
	LastMessageID int64
	UpdatedAt     time.Time
}

type MemoryRecord struct {
	ID              int64
	UserCode        string
	Category        string
	CanonicalKey    string
	Content         string
	ValueJSON       string
	Confidence      float64
	SourceSessionID string
	SourceMessageID int64
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type MemoryItem struct {
	Category     string  `json:"category"`
	CanonicalKey string  `json:"canonical_key"`
	Content      string  `json:"content"`
	ValueJSON    string  `json:"value_json"`
	Confidence   float64 `json:"confidence"`
}

type MemoryJob struct {
	SessionID string
	UserCode  string
	Language  string
}

// Repository 负责 assistant 的持久化读写，service 只依赖业务需要的存储能力，
// 不直接关心底层是 SQLite、MSSQL 还是别的实现。
type Repository interface {
	SaveSession(ctx context.Context, session SessionRecord) error
	GetSession(ctx context.Context, sessionID string) (*SessionRecord, error)
	AppendMessage(ctx context.Context, message MessageRecord) (int64, error)
	ListMessagesAfter(ctx context.Context, sessionID string, lastMessageID int64, limit int) ([]MessageRecord, error)
	ListRecentMessages(ctx context.Context, sessionID string, limit int) ([]MessageRecord, error)
	UpsertSummary(ctx context.Context, summary SummaryRecord) error
	GetSummary(ctx context.Context, sessionID string) (*SummaryRecord, error)
	UpsertMemories(ctx context.Context, userCode string, sessionID string, sourceMessageID int64, memories []MemoryItem) error
	ListMemories(ctx context.Context, userCode string, limit int) ([]MemoryRecord, error)
	DeleteMemories(ctx context.Context, userCode, category, canonicalKey string) (int64, error)
}
