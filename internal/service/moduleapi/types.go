package moduleapi

import (
	"context"

	"github.com/cloudwego/eino/adk"
)

type AssessmentStatus string

const (
	AssessmentReady       AssessmentStatus = "ready"
	AssessmentNeedClarify AssessmentStatus = "need_clarify"
	AssessmentReject      AssessmentStatus = "reject"
)

type ExecutionPhase string

const (
	PhaseRead  ExecutionPhase = "read"
	PhaseWrite ExecutionPhase = "write"
)

type Assessment struct {
	ModuleKey      string
	Status         AssessmentStatus
	Phase          ExecutionPhase
	Score          float64
	Reason         string
	FollowUpPrompt string
}

type Module interface {
	adk.ResumableAgent
	Assess(ctx context.Context, userMessage string) (Assessment, error)
}
