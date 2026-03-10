package itsmagent

import (
	"fmt"

	"github.com/cloudwego/eino/adk"

	"lakeside/api/itsm/v1"
)

func APIInterruptsFromContexts(ctxs []*adk.InterruptCtx) ([]v1.AgentInterrupt, string) {
	return parseInterrupts(ctxs)
}

func ExecutionResultFromAny(v any) *TicketExecutionResult {
	return castExecutionResult(v)
}

func ErrorResponse(sessionID, message string) *v1.AgentResponse {
	return errorResponse(sessionID, message)
}

func DefaultResultMessage(lastMessage string) *TicketExecutionResult {
	return &TicketExecutionResult{
		Success: true,
		Message: chooseMessage(lastMessage, "操作完成"),
	}
}

func BuildResumeData(target *v1.ResumeTarget) (any, error) {
	if target == nil {
		return nil, fmt.Errorf("resume target is nil")
	}
	if target.Confirmed != nil {
		return &ResumeConfirmData{
			Confirmed:  *target.Confirmed,
			Subject:    target.Subject,
			OthersDesc: target.OthersDesc,
		}, nil
	}
	return &ResumeCollectData{Answer: target.Answer}, nil
}
