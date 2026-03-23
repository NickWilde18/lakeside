package deerflow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"lakeside/internal/service/deerflowclient"
	"lakeside/internal/service/agentplatform/eventctx"
)

type Result struct {
	AgentName string   `json:"agent_name"`
	Success   bool     `json:"success"`
	Message   string   `json:"message"`
	ThreadID  string   `json:"thread_id,omitempty"`
	Artifacts []string `json:"artifacts,omitempty"`
	Trace     *deerflowclient.Trace `json:"trace,omitempty"`
}

func ResultFromAny(value any) *Result {
	switch item := value.(type) {
	case nil:
		return nil
	case *Result:
		return item
	case Result:
		copied := item
		return &copied
	default:
		return nil
	}
}

type Agent struct {
	key         string
	description string
	client      *deerflowclient.Client
}

func New(key, description string, client *deerflowclient.Client) adk.Agent {
	return &Agent{
		key:         strings.TrimSpace(key),
		description: strings.TrimSpace(description),
		client:      client,
	}
}

func (a *Agent) Name(_ context.Context) string {
	return a.key
}

func (a *Agent) Description(_ context.Context) string {
	return a.description
}

func (a *Agent) GetType() string {
	return "DeerFlowAgent"
}

func (a *Agent) Run(ctx context.Context, input *adk.AgentInput, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		defer gen.Close()
		if a == nil || a.client == nil {
			gen.Send(&adk.AgentEvent{Err: fmt.Errorf("deerflow client is nil")})
			return
		}
		message := latestUserMessage(ctx, input)
		if message == "" {
			gen.Send(&adk.AgentEvent{Err: fmt.Errorf("empty user message")})
			return
		}
		result, err := a.client.RunWait(ctx, deerflowclient.RunWaitRequest{
			ThreadID:          sessionString(ctx, "deerflow_thread_id"),
			Message:           message,
			SessionID:         sessionString(ctx, "session_id"),
			UserUPN:           sessionString(ctx, "user_upn"),
			PreferredLanguage: sessionString(ctx, "preferred_language"),
			OnTraceUpdate: func(trace *deerflowclient.Trace) {
				if trace == nil {
					return
				}
				eventctx.EmitForNode(ctx, "deerflow_trace_updated", a.key, "deerflow trace updated", map[string]any{
					"trace": trace,
				})
			},
		})
		if err != nil {
			gen.Send(&adk.AgentEvent{Err: err})
			return
		}
		text := strings.TrimSpace(result.Message)
		if text == "" {
			details := make([]string, 0, 3)
			if status := strings.TrimSpace(result.RunStatus); status != "" {
				details = append(details, fmt.Sprintf("status=%s", status))
			}
			if threadID := strings.TrimSpace(result.ThreadID); threadID != "" {
				details = append(details, fmt.Sprintf("thread_id=%s", threadID))
			}
			if runID := strings.TrimSpace(result.RunID); runID != "" {
				details = append(details, fmt.Sprintf("run_id=%s", runID))
			}
			message := "deerflow completed without visible text"
			if len(details) > 0 {
				message += " (" + strings.Join(details, ", ") + ")"
			}
			gen.Send(&adk.AgentEvent{Err: errors.New(message)})
			return
		}
		gen.Send(&adk.AgentEvent{
			AgentName: a.key,
			Output: &adk.AgentOutput{
				MessageOutput: &adk.MessageVariant{
					IsStreaming: false,
					Message:     schema.AssistantMessage(text, nil),
					Role:        schema.Assistant,
				},
				CustomizedOutput: &Result{
					AgentName: a.key,
					Success:   true,
					Message:   text,
					ThreadID:  strings.TrimSpace(result.ThreadID),
					Artifacts: append([]string(nil), result.Artifacts...),
					Trace:     result.Trace,
				},
			},
		})
	}()
	return iter
}

func latestUserMessage(ctx context.Context, input *adk.AgentInput) string {
	if value, ok := adk.GetSessionValue(ctx, "latest_user_message"); ok {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	if input == nil || len(input.Messages) == 0 {
		return ""
	}
	for i := len(input.Messages) - 1; i >= 0; i-- {
		msg := input.Messages[i]
		if msg == nil || msg.Role != schema.User {
			continue
		}
		if strings.TrimSpace(msg.Content) != "" {
			return strings.TrimSpace(msg.Content)
		}
	}
	return ""
}

func sessionString(ctx context.Context, key string) string {
	value, ok := adk.GetSessionValue(ctx, key)
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}
