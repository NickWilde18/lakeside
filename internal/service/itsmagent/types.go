package itsmagent

import (
	"time"

	"lakeside/api/itsm/v1"
)

const (
	statusNeedInfo    = "need_info"
	statusNeedConfirm = "need_confirm"
	statusDone        = "done"
	statusError       = "error"
)

const (
	stageCollect = "collect"
	stageConfirm = "confirm"
)

type QueryRequest struct {
	UserCode         string
	Message          string
	AssistantContext string
}

type ResumeRequest struct {
	CheckpointID     string
	UserCode         string
	Targets          map[string]*v1.ResumeTarget
	AssistantContext string
}

type TicketDraft struct {
	UserCode     string `json:"userCode"`
	Subject      string `json:"subject"`
	ServiceLevel string `json:"serviceLevel"`
	Priority     string `json:"priority"`
	OthersDesc   string `json:"othersDesc"`

	ServiceLevelConfidence float64 `json:"-"`
	PriorityConfidence     float64 `json:"-"`
}

type TicketInterruptInfo struct {
	Type           string      `json:"type"`
	Prompt         string      `json:"prompt"`
	Language       string      `json:"language,omitempty"`
	MissingFields  []string    `json:"missing_fields,omitempty"`
	EditableFields []string    `json:"editable_fields,omitempty"`
	ReadonlyFields []string    `json:"readonly_fields,omitempty"`
	Draft          TicketDraft `json:"draft"`
}

type TicketAgentState struct {
	Stage    string              `json:"stage"`
	Language string              `json:"language,omitempty"`
	Draft    TicketDraft         `json:"draft"`
	Pending  TicketInterruptInfo `json:"pending"`
}

type ResumeCollectData struct {
	Answer string `json:"answer"`
}

type ResumeConfirmData struct {
	Confirmed  bool   `json:"confirmed"`
	Subject    string `json:"subject,omitempty"`
	OthersDesc string `json:"othersDesc,omitempty"`
}

type TicketExecutionResult struct {
	Success  bool   `json:"success"`
	TicketNo string `json:"ticket_no,omitempty"`
	Message  string `json:"message"`
	Code     int    `json:"code,omitempty"`
}

type ExtractResult struct {
	Subject                string  `json:"subject"`
	OthersDesc             string  `json:"othersDesc"`
	ServiceLevel           string  `json:"serviceLevel"`
	ServiceLevelConfidence float64 `json:"serviceLevel_confidence"`
	Priority               string  `json:"priority"`
	PriorityConfidence     float64 `json:"priority_confidence"`
	ClarifyQuestion        string  `json:"clarify_question"`
}

type serviceConfig struct {
	EnumConfidenceThreshold float64
	CheckpointTTL           time.Duration
	IdempotencyTTL          time.Duration
	CheckpointKeyPrefix     string
	IdempotencyKeyPrefix    string
}
