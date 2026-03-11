package agent

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

	v1 "lakeside/api/agent/v1"
	"lakeside/internal/service/agentplatform"
)

func (c *ControllerV1) AgentRunEvents(ctx context.Context, req *v1.AgentRunEventsReq) (res *v1.AgentRunEventsRes, err error) {
	svc := agentplatform.GetService(ctx)
	runReq := &agentplatform.GetRunRequest{
		AssistantKey: req.AssistantKey,
		RunID:        req.RunID,
		UserUPN:      req.UserID,
	}
	snapshot, err := svc.GetRun(ctx, runReq)
	if err != nil {
		return nil, err
	}
	r := g.RequestFromCtx(ctx)
	r.Response.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	r.Response.Header().Set("Cache-Control", "no-cache")
	r.Response.Header().Set("Connection", "keep-alive")
	r.Response.Header().Set("X-Accel-Buffering", "no")
	r.Response.Write(": connected\n\n")
	r.Response.Flush()

	afterID := lastEventIDFromRequest(r)
	flushEvents := func() (bool, error) {
		events, listErr := svc.ListRunEvents(ctx, &agentplatform.ListRunEventsRequest{
			AssistantKey: req.AssistantKey,
			RunID:        req.RunID,
			UserUPN:      req.UserID,
			AfterID:      afterID,
		})
		if listErr != nil {
			return false, listErr
		}
		for _, event := range events {
			if event.ID <= afterID {
				continue
			}
			writeSSEEvent(r, buildRunEventPayload(event))
			afterID = event.ID
			if isTerminalEventType(event.EventType) {
				return true, nil
			}
		}
		return false, nil
	}
	terminal, err := flushEvents()
	if err != nil {
		return nil, err
	}
	if terminal {
		return nil, nil
	}
	if isTerminalRunStatus(snapshot.RunStatus) {
		return nil, nil
	}

	stream, unsubscribeLocal := svc.SubscribeRun(req.RunID)
	defer unsubscribeLocal()
	wakeup, unsubscribeWake := svc.SubscribeRunWake(r.Context(), req.RunID)
	defer unsubscribeWake()
	pollTicker := time.NewTicker(1200 * time.Millisecond)
	defer pollTicker.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, nil
		case <-r.Context().Done():
			return nil, nil
		case event, ok := <-stream:
			if !ok {
				return nil, nil
			}
			if event.ID <= afterID {
				continue
			}
			writeSSEEvent(r, buildRunEventPayload(event))
			afterID = event.ID
			if isTerminalEventType(event.EventType) {
				return nil, nil
			}
		case <-wakeup:
			finished, flushErr := flushEvents()
			if flushErr != nil {
				return nil, flushErr
			}
			if finished {
				return nil, nil
			}
		case <-pollTicker.C:
			finished, flushErr := flushEvents()
			if flushErr != nil {
				return nil, flushErr
			}
			if finished {
				return nil, nil
			}
		case <-heartbeat.C:
			r.Response.Write(": heartbeat\n\n")
			r.Response.Flush()
		}
	}
}

func lastEventIDFromRequest(r *ghttp.Request) int64 {
	if r == nil {
		return 0
	}
	text := strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	if text == "" {
		text = strings.TrimSpace(r.Get("last_event_id").String())
	}
	if text == "" {
		return 0
	}
	id, err := strconv.ParseInt(text, 10, 64)
	if err != nil || id < 0 {
		return 0
	}
	return id
}

func writeSSEEvent(r *ghttp.Request, payload runEventPayload) {
	if r == nil {
		return
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	r.Response.Write("id: ", strconv.FormatInt(payload.EventID, 10), "\n")
	r.Response.Write("event: ", payload.EventType, "\n")
	r.Response.Write("data: ", string(body), "\n\n")
	r.Response.Flush()
}

func isTerminalEventType(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "run_waiting_input", "run_completed", "run_failed", "run_cancelled":
		return true
	default:
		return false
	}
}

func isTerminalRunStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "waiting_input", "done", "failed", "cancelled":
		return true
	default:
		return false
	}
}
