package agent

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/frame/g"

	v1 "lakeside/api/agent/v1"
	itsmv1 "lakeside/api/itsm/v1"
	"lakeside/internal/service/agentplatform"
)

func (c *ControllerV1) AgentActions(ctx context.Context, req *v1.AgentActionsReq) (res *v1.AgentActionsRes, err error) {
	svc := agentplatform.GetService(ctx)
	r := g.RequestFromCtx(ctx)
	r.Response.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	r.Response.Header().Set("Cache-Control", "no-cache")
	if req.UserAction == nil {
		state, buildErr := buildAgentPageState(ctx, svc, req.AssistantKey, req.UserID, "", req.AdvancedTrace, nil)
		if buildErr != nil {
			return nil, buildErr
		}
		if err := writeA2UIRender(r.Response.Writer, state); err != nil {
			return nil, err
		}
		r.Response.Flush()
		return nil, nil
	}
	action := req.UserAction
	sessionID := parseActionString(action.Context, "sessionId")
	switch strings.TrimSpace(action.Name) {
	case "new_session":
		created, createErr := svc.CreateSession(ctx, &agentplatform.CreateSessionRequest{
			AssistantKey: req.AssistantKey,
			UserUPN:      req.UserID,
			Language:     "zh",
		})
		if createErr != nil {
			return nil, createErr
		}
		state, buildErr := buildAgentPageState(ctx, svc, req.AssistantKey, req.UserID, created.SessionID, req.AdvancedTrace, nil)
		if buildErr != nil {
			return nil, buildErr
		}
		if err := writeA2UIRender(r.Response.Writer, state); err != nil {
			return nil, err
		}
		r.Response.Flush()
		return nil, nil
	case "open_session":
		state, buildErr := buildAgentPageState(ctx, svc, req.AssistantKey, req.UserID, sessionID, req.AdvancedTrace, nil)
		if buildErr != nil {
			return nil, buildErr
		}
		if err := writeA2UIRender(r.Response.Writer, state); err != nil {
			return nil, err
		}
		r.Response.Flush()
		return nil, nil
	case "delete_session":
		if sessionID != "" {
			if err := svc.DeleteSession(ctx, &agentplatform.DeleteSessionRequest{
				AssistantKey: req.AssistantKey,
				SessionID:    sessionID,
				UserUPN:      req.UserID,
			}); err != nil {
				return nil, err
			}
		}
		state, buildErr := buildAgentPageState(ctx, svc, req.AssistantKey, req.UserID, "", req.AdvancedTrace, nil)
		if buildErr != nil {
			return nil, buildErr
		}
		if err := writeA2UIRender(r.Response.Writer, state); err != nil {
			return nil, err
		}
		r.Response.Flush()
		return nil, nil
	case "send_message":
		message := parseActionString(action.Context, "message")
		created, runErr := svc.CreateRun(ctx, &agentplatform.CreateRunRequest{
			AssistantKey: req.AssistantKey,
			UserUPN:      req.UserID,
			SessionID:    sessionID,
			Message:      message,
		})
		if runErr != nil {
			return nil, runErr
		}
		overlay := &agentRuntimeOverlay{
			SessionID:    created.SessionID,
			CurrentRunID: created.RunID,
			DraftStatus:  created.RunStatus,
		}
		state, buildErr := buildAgentPageState(ctx, svc, req.AssistantKey, req.UserID, created.SessionID, req.AdvancedTrace, overlay)
		if buildErr != nil {
			return nil, buildErr
		}
		if err := writeA2UIRender(r.Response.Writer, state); err != nil {
			return nil, err
		}
		r.Response.Flush()
		waitErr := waitForRunTerminal(ctx, svc, req.AssistantKey, created.RunID, req.UserID, 0, func(event agentplatform.RunEventRecord) error {
			if err := onRuntimeEvent(overlay, r.Response.Writer, req.AdvancedTrace)(event); err != nil {
				return err
			}
			eventType := strings.TrimSpace(event.EventType)
			if eventType == "knowledge_answer_chunk" {
				r.Response.Flush()
				return nil
			}
			if req.AdvancedTrace || shouldRefreshActionSurface(eventType) {
				updated, err := buildAgentPageState(ctx, svc, req.AssistantKey, req.UserID, overlay.SessionID, req.AdvancedTrace, overlay)
				if err != nil {
					return err
				}
				if err := writeA2UIRender(r.Response.Writer, updated); err != nil {
					return err
				}
				r.Response.Flush()
			}
			return nil
		})
		if waitErr != nil && !strings.Contains(waitErr.Error(), "context canceled") {
			g.Log().Warningf(ctx, "agent action stream failed, assistant_key=%s run_id=%s err=%v", req.AssistantKey, created.RunID, waitErr)
		}
		finalState, buildErr := buildAgentPageState(ctx, svc, req.AssistantKey, req.UserID, created.SessionID, req.AdvancedTrace, nil)
		if buildErr == nil {
			_ = writeA2UIRender(r.Response.Writer, finalState)
			r.Response.Flush()
		}
		return nil, nil
	case "follow_up_submit", "approval_submit":
		runID := parseActionString(action.Context, "runId")
		interruptID := parseActionString(action.Context, "interruptId")
		var target itsmv1.ResumeTarget
		if strings.TrimSpace(action.Name) == "follow_up_submit" {
			answer := parseActionString(action.Context, "answer")
			target.Answer = answer
		} else {
			approved := parseActionBool(action.Context, "approved")
			target.Confirmed = approved
			target.Subject = parseActionString(action.Context, "subject")
			target.OthersDesc = parseActionString(action.Context, "othersDesc")
		}
		created, resumeErr := svc.ResumeRun(ctx, &agentplatform.ResumeRunRequest{
			AssistantKey: req.AssistantKey,
			RunID:        runID,
			UserUPN:      req.UserID,
			Targets:      map[string]*itsmv1.ResumeTarget{interruptID: &target},
		})
		if resumeErr != nil {
			return nil, resumeErr
		}
		overlay := &agentRuntimeOverlay{
			SessionID:    created.SessionID,
			CurrentRunID: created.RunID,
			DraftStatus:  created.RunStatus,
		}
		state, buildErr := buildAgentPageState(ctx, svc, req.AssistantKey, req.UserID, created.SessionID, req.AdvancedTrace, overlay)
		if buildErr != nil {
			return nil, buildErr
		}
		if err := writeA2UIRender(r.Response.Writer, state); err != nil {
			return nil, err
		}
		r.Response.Flush()
		waitErr := waitForRunTerminal(ctx, svc, req.AssistantKey, created.RunID, req.UserID, 0, func(event agentplatform.RunEventRecord) error {
			if err := onRuntimeEvent(overlay, r.Response.Writer, req.AdvancedTrace)(event); err != nil {
				return err
			}
			eventType := strings.TrimSpace(event.EventType)
			if eventType == "knowledge_answer_chunk" {
				r.Response.Flush()
				return nil
			}
			if req.AdvancedTrace || shouldRefreshActionSurface(eventType) {
				updated, err := buildAgentPageState(ctx, svc, req.AssistantKey, req.UserID, overlay.SessionID, req.AdvancedTrace, overlay)
				if err != nil {
					return err
				}
				if err := writeA2UIRender(r.Response.Writer, updated); err != nil {
					return err
				}
				r.Response.Flush()
			}
			return nil
		})
		if waitErr != nil && !strings.Contains(waitErr.Error(), "context canceled") {
			g.Log().Warningf(ctx, "agent resume action stream failed, assistant_key=%s run_id=%s err=%v", req.AssistantKey, created.RunID, waitErr)
		}
		finalState, buildErr := buildAgentPageState(ctx, svc, req.AssistantKey, req.UserID, created.SessionID, req.AdvancedTrace, nil)
		if buildErr == nil {
			_ = writeA2UIRender(r.Response.Writer, finalState)
			r.Response.Flush()
		}
		return nil, nil
	case "cancel_turn":
		runID := parseActionString(action.Context, "runId")
		if err := svc.CancelRun(ctx, &agentplatform.CancelRunRequest{
			AssistantKey: req.AssistantKey,
			RunID:        runID,
			UserUPN:      req.UserID,
		}); err != nil {
			return nil, err
		}
		_ = waitForRunTerminal(ctx, svc, req.AssistantKey, runID, req.UserID, 0, nil)
		state, buildErr := buildAgentPageState(ctx, svc, req.AssistantKey, req.UserID, sessionID, req.AdvancedTrace, nil)
		if buildErr != nil {
			return nil, buildErr
		}
		if err := writeA2UIRender(r.Response.Writer, state); err != nil {
			return nil, err
		}
		r.Response.Flush()
		return nil, nil
	default:
		state, buildErr := buildAgentPageState(ctx, svc, req.AssistantKey, req.UserID, sessionID, req.AdvancedTrace, nil)
		if buildErr != nil {
			return nil, buildErr
		}
		if err := writeA2UIRender(r.Response.Writer, state); err != nil {
			return nil, err
		}
		r.Response.Flush()
		return nil, nil
	}
}

func shouldRefreshActionSurface(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "run_started", "itsm_interrupt_emitted", "run_waiting_input", "run_completed", "run_failed", "run_cancelled":
		return true
	default:
		return false
	}
}
