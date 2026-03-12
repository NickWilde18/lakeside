package agentplatform

import (
	"context"
	"time"

	"github.com/cloudwego/eino/adk"

	itsmv1 "lakeside/api/itsm/v1"
)

const (
	statusActive  = "active"
	statusDone    = "done"
	statusDeleted = "deleted"
)

const (
	runKindQuery  = "query"
	runKindResume = "resume"
)

const (
	runStatusQueued       = "queued"
	runStatusRunning      = "running"
	runStatusWaitingInput = "waiting_input"
	runStatusDone         = "done"
	runStatusFailed       = "failed"
	runStatusCancelled    = "cancelled"
)

const (
	eventTypeRunStarted             = "run_started"
	eventTypeRunWaitingInput        = "run_waiting_input"
	eventTypeRunCompleted           = "run_completed"
	eventTypeRunFailed              = "run_failed"
	eventTypeRunCancelled           = "run_cancelled"
	eventTypeAgentEntered           = "agent_entered"
	eventTypeAgentCompleted         = "agent_completed"
	eventTypeKnowledgeRetrieveStart = "knowledge_retrieve_started"
	eventTypeKnowledgeRetrieveEnd   = "knowledge_retrieve_finished"
	eventTypeKnowledgeAnswerReady   = "knowledge_answer_ready"
	eventTypeITSMInterruptEmitted   = "itsm_interrupt_emitted"
	eventTypeITSMDone               = "itsm_done"
)

const (
	stepKindKnowledge       = "knowledge"
	stepKindITSMDone        = "itsm_done"
	stepKindITSMInterrupt   = "itsm_interrupt"
	stepKindAssistantAnswer = "assistant_message"
)

type CreateRunRequest struct {
	AssistantKey string
	UserUPN      string
	SessionID    string
	Message      string
}

type CreateRunResult struct {
	AssistantKey string
	RunID        string
	SessionID    string
	RunStatus    string
}

type ResumeRunRequest struct {
	AssistantKey string
	RunID        string
	UserUPN      string
	Targets      map[string]*itsmv1.ResumeTarget
}

type ResumeRunResult struct {
	AssistantKey string
	RunID        string
	SessionID    string
	RunStatus    string
}

type GetRunRequest struct {
	AssistantKey string
	RunID        string
	UserUPN      string
}

type ListSessionsRequest struct {
	AssistantKey string
	UserUPN      string
	Limit        int
}

type GetSessionRequest struct {
	AssistantKey string
	SessionID    string
	UserUPN      string
}

type DeleteSessionRequest struct {
	AssistantKey string
	SessionID    string
	UserUPN      string
}

type ListRunEventsRequest struct {
	AssistantKey string
	RunID        string
	UserUPN      string
	AfterID      int64
}

type CancelRunRequest struct {
	AssistantKey string
	RunID        string
	UserUPN      string
}

type executeQueryRequest struct {
	AssistantKey string
	RunID        string
	SessionID    string
	UserUPN      string
	Message      string
	CheckpointID string
	Language     string
	CreatedAt    time.Time
}

type executeResumeRequest struct {
	AssistantKey string
	RunID        string
	SessionID    string
	CheckpointID string
	UserUPN      string
	Targets      map[string]*itsmv1.ResumeTarget
	Language     string
	CreatedAt    time.Time
}

type ListMemoriesRequest struct {
	AssistantKey string
	UserUPN      string
	Limit        int
}

type ClearMemoriesRequest struct {
	AssistantKey string
	UserUPN      string
	Category     string
	CanonicalKey string
}

type Source struct {
	KBID     string
	DocID    string
	NodeID   string
	Filename string
	Snippet  string
	Score    float64
}

type StepResult struct {
	Path       []string
	Kind       string
	Message    string
	Sources    []Source
	Interrupts []itsmv1.AgentInterrupt
}

type Result struct {
	Success  bool
	TicketNo string
	Message  string
	Code     int
	Sources  []Source
}

type Response struct {
	AssistantKey string
	Status       string
	SessionID    string
	CheckpointID string
	ActivePath   []string
	Steps        []StepResult
	Interrupts   []itsmv1.AgentInterrupt
	Result       *Result
}

type RunSnapshot struct {
	RunID        string
	AssistantKey string
	SessionID    string
	RunStatus    string
	Status       string
	CheckpointID string
	ActivePath   []string
	Steps        []StepResult
	Interrupts   []itsmv1.AgentInterrupt
	Result       *Result
	ErrorMessage string
	StartedAt    time.Time
	FinishedAt   time.Time
}

type SessionSummary struct {
	AssistantKey  string
	SessionID     string
	Title         string
	Status        string
	ActivePath    []string
	LastRunID     string
	LastRunStatus string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type SessionMessage struct {
	ID           int64
	Role         string
	Content      string
	ActivePath   []string
	CheckpointID string
	CreatedAt    time.Time
}

type RunTrace struct {
	Snapshot *RunSnapshot
	Events   []RunEventRecord
}

type SessionDetail struct {
	Session  SessionSummary
	Messages []SessionMessage
	Runs     []RunTrace
}

type SessionRecord struct {
	AssistantKey     string    `orm:"assistant_key"`
	SessionID        string    `orm:"session_id"`
	UserUPN          string    `orm:"user_upn"`
	ActivePathJSON   string    `orm:"active_path_json"`
	ActiveCheckpoint string    `orm:"active_checkpoint_id"`
	Status           string    `orm:"status"`
	Language         string    `orm:"language"`
	CreatedAt        time.Time `orm:"created_at"`
	UpdatedAt        time.Time `orm:"updated_at"`
}

type MessageRecord struct {
	ID             int64     `orm:"id"`
	AssistantKey   string    `orm:"assistant_key"`
	SessionID      string    `orm:"session_id"`
	UserUPN        string    `orm:"user_upn"`
	Role           string    `orm:"role"`
	Content        string    `orm:"content"`
	PayloadJSON    string    `orm:"payload_json"`
	ActivePathJSON string    `orm:"active_path_json"`
	CheckpointID   string    `orm:"checkpoint_id"`
	Language       string    `orm:"language"`
	CreatedAt      time.Time `orm:"created_at"`
}

type RunRecord struct {
	RunID        string    `orm:"run_id"`
	AssistantKey string    `orm:"assistant_key"`
	SessionID    string    `orm:"session_id"`
	UserUPN      string    `orm:"user_upn"`
	Kind         string    `orm:"kind"`
	Status       string    `orm:"status"`
	ParentRunID  string    `orm:"parent_run_id"`
	RequestJSON  string    `orm:"request_json"`
	ResponseJSON string    `orm:"response_json"`
	CheckpointID string    `orm:"checkpoint_id"`
	ErrorMessage string    `orm:"error_message"`
	StartedAt    time.Time `orm:"started_at"`
	FinishedAt   time.Time `orm:"finished_at"`
	LastEventID  int64     `orm:"last_event_id"`
}

type RunEventRecord struct {
	ID           int64     `orm:"id"`
	RunID        string    `orm:"run_id"`
	AssistantKey string    `orm:"assistant_key"`
	SessionID    string    `orm:"session_id"`
	EventType    string    `orm:"event_type"`
	PathJSON     string    `orm:"path_json"`
	Message      string    `orm:"message"`
	PayloadJSON  string    `orm:"payload_json"`
	CreatedAt    time.Time `orm:"created_at"`
}

type EventPayload map[string]any

type MemoryRecord struct {
	ID              int64     `orm:"id"`
	AssistantKey    string    `orm:"assistant_key"`
	UserUPN         string    `orm:"user_upn"`
	Category        string    `orm:"category"`
	CanonicalKey    string    `orm:"canonical_key"`
	Content         string    `orm:"content"`
	ValueJSON       string    `orm:"value_json"`
	Confidence      float64   `orm:"confidence"`
	SourceSessionID string    `orm:"source_session_id"`
	SourceMessageID int64     `orm:"source_message_id"`
	Status          string    `orm:"status"`
	CreatedAt       time.Time `orm:"created_at"`
	UpdatedAt       time.Time `orm:"updated_at"`
}

type MemoryItem struct {
	Category     string  `json:"category"`
	CanonicalKey string  `json:"canonical_key"`
	Content      string  `json:"content"`
	ValueJSON    string  `json:"value_json"`
	Confidence   float64 `json:"confidence"`
}

type MemoryJob struct {
	AssistantKey string
	SessionID    string
	UserUPN      string
	Language     string
}

type Repository interface {
	SaveSession(ctx context.Context, session SessionRecord) error
	GetSession(ctx context.Context, sessionID string) (*SessionRecord, error)
	ListSessions(ctx context.Context, assistantKey, userUPN string, limit int) ([]SessionRecord, error)
	DeleteSession(ctx context.Context, assistantKey, sessionID, userUPN string, deletedAt time.Time) error
	AppendMessage(ctx context.Context, message MessageRecord) (int64, error)
	ListRecentMessages(ctx context.Context, sessionID string, limit int) ([]MessageRecord, error)
	ListMessages(ctx context.Context, sessionID string) ([]MessageRecord, error)
	CreateRun(ctx context.Context, run RunRecord) error
	TryStartRun(ctx context.Context, runID string) (bool, error)
	TryCancelQueuedRun(ctx context.Context, runID, responseJSON, errorMessage string, finishedAt time.Time) (bool, error)
	UpdateRunStatus(ctx context.Context, runID, status string) error
	FinishRun(ctx context.Context, runID, status, responseJSON, checkpointID, errorMessage string, finishedAt time.Time) error
	GetRun(ctx context.Context, runID string) (*RunRecord, error)
	ListRunsBySession(ctx context.Context, sessionID string) ([]RunRecord, error)
	AppendRunEvent(ctx context.Context, event RunEventRecord) (int64, error)
	ListRunEventsAfter(ctx context.Context, runID string, afterID int64) ([]RunEventRecord, error)
	MarkStaleRunsFailed(ctx context.Context, errorMessage string, finishedAt time.Time) error
	UpsertMemories(ctx context.Context, assistantKey, userUPN, sessionID string, sourceMessageID int64, memories []MemoryItem) error
	ListMemories(ctx context.Context, assistantKey, userUPN string, limit int) ([]MemoryRecord, error)
	DeleteMemories(ctx context.Context, assistantKey, userUPN, category, canonicalKey string) (int64, error)
}

type config struct {
	Storage struct {
		Provider   string
		SQLitePath string
		MSSQL      struct {
			Host   string
			Port   int
			User   string
			Pass   string
			DBName string
		}
	}
	Checkpoint struct {
		TTLHours int
	}
	Summarization struct {
		ContextTokens             int
		PreserveUserMessageTokens int
	}
	Memory struct {
		MaxItems  int
		Workers   int
		QueueSize int
	}
	RAG struct {
		BaseURL     string
		TimeoutMs   int
		DefaultTopK int
	}
	Runtime struct {
		WorkerCount    int
		StreamKey      string
		StreamGroup    string
		ConsumerPrefix string
		ReadBlockMs    int
		PubSubChannel  string
	}
	Roots   []supervisorNodeConfig
	Domains []supervisorNodeConfig
	Leaves  []leafNodeConfig
}

type supervisorNodeConfig struct {
	Key                 string
	Description         string
	InstructionTemplate string
	Children            []string
	MaxIterations       int
}

type leafNodeConfig struct {
	Key            string
	Type           string
	Description    string
	KBIDs          []string
	TopK           int
	RewriteQueries int
	MaxContextDocs int
	SourceLimit    int
}

type nodeInfo struct {
	Key         string
	Description string
	Kind        string
}

type runnerBundle struct {
	RootKey         string
	Runner          *adk.Runner
	CheckpointStore checkpointStore
}

type nodePathIndex map[string]map[string][]string
