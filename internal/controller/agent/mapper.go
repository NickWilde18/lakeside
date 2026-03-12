package agent

import (
	"encoding/json"
	"strings"
	"time"

	v1 "lakeside/api/agent/v1"
	itsmv1 "lakeside/api/itsm/v1"
	"lakeside/internal/service/agentplatform"
)

func buildAgentRunSnapshot(snapshot *agentplatform.RunSnapshot) v1.AgentRunSnapshot {
	if snapshot == nil {
		return v1.AgentRunSnapshot{}
	}
	return v1.AgentRunSnapshot{
		RunID:        snapshot.RunID,
		AssistantKey: snapshot.AssistantKey,
		RunStatus:    snapshot.RunStatus,
		Status:       snapshot.Status,
		SessionID:    snapshot.SessionID,
		CheckpointID: snapshot.CheckpointID,
		ActivePath:   append([]string(nil), snapshot.ActivePath...),
		Steps:        buildAgentSteps(snapshot.Steps),
		Interrupts:   append([]itsmv1.AgentInterrupt(nil), snapshot.Interrupts...),
		Result:       buildAgentResult(snapshot.Result),
		ErrorMessage: snapshot.ErrorMessage,
		StartedAt:    formatTime(snapshot.StartedAt),
		FinishedAt:   formatTime(snapshot.FinishedAt),
	}
}

func buildAgentSteps(steps []agentplatform.StepResult) []v1.AgentStep {
	if len(steps) == 0 {
		return nil
	}
	return mapSlice(steps, func(step agentplatform.StepResult) v1.AgentStep {
		return v1.AgentStep{
			Path:       append([]string(nil), step.Path...),
			Kind:       step.Kind,
			Message:    step.Message,
			Sources:    buildAgentSources(step.Sources),
			Interrupts: step.Interrupts,
		}
	})
}

func buildAgentResult(result *agentplatform.Result) *v1.AgentResult {
	if result == nil {
		return nil
	}
	return &v1.AgentResult{
		Success:  result.Success,
		TicketNo: result.TicketNo,
		Message:  result.Message,
		Code:     result.Code,
		Sources:  buildAgentSources(result.Sources),
	}
}

func buildAgentSources(sources []agentplatform.Source) []v1.AgentSource {
	if len(sources) == 0 {
		return nil
	}
	return mapSlice(sources, func(source agentplatform.Source) v1.AgentSource {
		return v1.AgentSource{
			KBID:     source.KBID,
			DocID:    source.DocID,
			NodeID:   source.NodeID,
			Filename: source.Filename,
			Snippet:  source.Snippet,
			Score:    source.Score,
		}
	})
}

func buildAgentSessionSummary(summary agentplatform.SessionSummary) v1.AgentSessionSummary {
	return v1.AgentSessionSummary{
		AssistantKey:  summary.AssistantKey,
		SessionID:     summary.SessionID,
		Title:         summary.Title,
		Status:        summary.Status,
		ActivePath:    append([]string(nil), summary.ActivePath...),
		LastRunID:     summary.LastRunID,
		LastRunStatus: summary.LastRunStatus,
		CreatedAt:     formatTime(summary.CreatedAt),
		UpdatedAt:     formatTime(summary.UpdatedAt),
	}
}

func buildAgentSessionDetail(detail *agentplatform.SessionDetail) v1.AgentSessionDetail {
	if detail == nil {
		return v1.AgentSessionDetail{}
	}
	out := v1.AgentSessionDetail{
		Session:  buildAgentSessionSummary(detail.Session),
		Messages: make([]v1.AgentSessionMessage, 0, len(detail.Messages)),
		Runs:     make([]v1.AgentSessionRunTrace, 0, len(detail.Runs)),
	}
	for _, message := range detail.Messages {
		out.Messages = append(out.Messages, v1.AgentSessionMessage{
			ID:           message.ID,
			Role:         message.Role,
			Content:      message.Content,
			ActivePath:   append([]string(nil), message.ActivePath...),
			CheckpointID: message.CheckpointID,
			CreatedAt:    formatTime(message.CreatedAt),
		})
	}
	for _, trace := range detail.Runs {
		snapshot := buildAgentRunSnapshot(trace.Snapshot)
		runTrace := v1.AgentSessionRunTrace{
			Snapshot: &snapshot,
			Events:   make([]v1.AgentRunEvent, 0, len(trace.Events)),
		}
		for _, event := range trace.Events {
			runTrace.Events = append(runTrace.Events, buildAgentRunEvent(event))
		}
		out.Runs = append(out.Runs, runTrace)
	}
	return out
}

type runEventPayload struct {
	EventID      int64    `json:"event_id"`
	RunID        string   `json:"run_id"`
	AssistantKey string   `json:"assistant_key"`
	SessionID    string   `json:"session_id"`
	Path         []string `json:"path,omitempty"`
	EventType    string   `json:"event_type"`
	Message      string   `json:"message,omitempty"`
	Payload      any      `json:"payload,omitempty"`
	CreatedAt    string   `json:"created_at"`
}

func buildRunEventPayload(event agentplatform.RunEventRecord) runEventPayload {
	return runEventPayload{
		EventID:      event.ID,
		RunID:        event.RunID,
		AssistantKey: event.AssistantKey,
		SessionID:    event.SessionID,
		Path:         parsePath(event.PathJSON),
		EventType:    event.EventType,
		Message:      event.Message,
		Payload:      parsePayload(event.PayloadJSON),
		CreatedAt:    formatTime(event.CreatedAt),
	}
}

func buildAgentRunEvent(event agentplatform.RunEventRecord) v1.AgentRunEvent {
	return v1.AgentRunEvent{
		EventID:      event.ID,
		RunID:        event.RunID,
		AssistantKey: event.AssistantKey,
		SessionID:    event.SessionID,
		Path:         parsePath(event.PathJSON),
		EventType:    event.EventType,
		Message:      event.Message,
		Payload:      parsePayload(event.PayloadJSON),
		CreatedAt:    formatTime(event.CreatedAt),
	}
}

func parsePayload(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return nil
	}
	out, err := parseJSON[any](raw)
	if err != nil {
		return raw
	}
	return out
}

func parsePath(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil
	}
	out, err := parseJSON[[]string](raw)
	if err != nil {
		return nil
	}
	return out
}

func parseJSON[T any](raw string) (T, error) {
	var out T
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return out, err
	}
	return out, nil
}

func mapSlice[T any, R any](items []T, mapper func(T) R) []R {
	if len(items) == 0 {
		return nil
	}
	out := make([]R, 0, len(items))
	for _, item := range items {
		out = append(out, mapper(item))
	}
	return out
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}
