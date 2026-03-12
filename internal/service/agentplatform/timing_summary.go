package agentplatform

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

type knowledgeTiming struct {
	path            []string
	firstRetrieveAt time.Time
	lastRetrieveAt  time.Time
	answerReadyAt   time.Time
	retrieveCount   int
	retrieveTotalMs int64
	retrieveMaxMs   int64
}

type agentTiming struct {
	path       []string
	agentName  string
	agentType  string
	durationMs int64
	createdAt  time.Time
}

// logRunTimingSummary 输出单次 run 的阶段耗时汇总日志。
// 汇总基于已经落库的 run events 自动聚合，便于后续新增 subagent 后复用同一套统计逻辑。
func (s *Service) logRunTimingSummary(ctx context.Context, runID, assistantKey, sessionID, runStatus string, startedAt, finishedAt time.Time) {
	if s == nil || s.repo == nil || strings.TrimSpace(runID) == "" {
		return
	}
	events, err := s.repo.ListRunEventsAfter(withoutCancel(ctx), runID, 0)
	if err != nil {
		g.Log().Warningf(ctx, "build run timing summary failed, run_id=%s err=%v", runID, err)
		return
	}
	summary := summarizeRunTiming(events, startedAt, finishedAt)
	g.Log().Infof(ctx, "agent run timing summary, assistant_key=%s run_id=%s session_id=%s run_status=%s total_ms=%d stages=%s", assistantKey, runID, sessionID, runStatus, durationBetweenMs(startedAt, finishedAt), summary)
}

func summarizeRunTiming(events []RunEventRecord, startedAt, finishedAt time.Time) string {
	if len(events) == 0 {
		return "-"
	}

	knowledgeByPath := make(map[string]*knowledgeTiming)
	agentItems := make([]agentTiming, 0, len(events))

	for _, event := range events {
		path := sanitizePath(decodeJSONOrZero[[]string](event.PathJSON))
		payload := decodeJSONOrZero[map[string]any](event.PayloadJSON)
		pathKey := strings.Join(path, ">")

		switch strings.TrimSpace(event.EventType) {
		case eventTypeKnowledgeRetrieveStart:
			item := ensureKnowledgeTiming(knowledgeByPath, pathKey, path)
			if item.firstRetrieveAt.IsZero() || event.CreatedAt.Before(item.firstRetrieveAt) {
				item.firstRetrieveAt = event.CreatedAt
			}
		case eventTypeKnowledgeRetrieveEnd:
			item := ensureKnowledgeTiming(knowledgeByPath, pathKey, path)
			if item.firstRetrieveAt.IsZero() || event.CreatedAt.Before(item.firstRetrieveAt) {
				item.firstRetrieveAt = event.CreatedAt
			}
			if event.CreatedAt.After(item.lastRetrieveAt) {
				item.lastRetrieveAt = event.CreatedAt
			}
			item.retrieveCount++
			dur := payloadInt64(payload["duration_ms"])
			if dur > 0 {
				item.retrieveTotalMs += dur
				if dur > item.retrieveMaxMs {
					item.retrieveMaxMs = dur
				}
			}
		case eventTypeKnowledgeAnswerReady:
			item := ensureKnowledgeTiming(knowledgeByPath, pathKey, path)
			if item.firstRetrieveAt.IsZero() {
				item.firstRetrieveAt = event.CreatedAt
			}
			if event.CreatedAt.After(item.answerReadyAt) {
				item.answerReadyAt = event.CreatedAt
			}
		case eventTypeAgentCompleted:
			dur := payloadInt64(payload["duration_ms"])
			agentName := strings.TrimSpace(toString(payload["agent_name"]))
			agentType := strings.TrimSpace(toString(payload["agent_type"]))
			if dur <= 0 {
				continue
			}
			agentItems = append(agentItems, agentTiming{
				path:       path,
				agentName:  agentName,
				agentType:  agentType,
				durationMs: dur,
				createdAt:  event.CreatedAt,
			})
		}
	}

	stageItems := make([]string, 0, len(knowledgeByPath)+len(agentItems))
	knowledgeItems := make([]*knowledgeTiming, 0, len(knowledgeByPath))
	for _, item := range knowledgeByPath {
		knowledgeItems = append(knowledgeItems, item)
	}
	slices.SortFunc(knowledgeItems, func(a, b *knowledgeTiming) int {
		return a.firstRetrieveAt.Compare(b.firstRetrieveAt)
	})
	for _, item := range knowledgeItems {
		totalMs := durationBetweenMs(item.firstRetrieveAt, chooseLaterTime(item.answerReadyAt, item.lastRetrieveAt))
		stageItems = append(stageItems, fmt.Sprintf("knowledge[%s]{queries=%d,retrieve_total_ms=%d,retrieve_max_ms=%d,total_ms=%d}", formatTimingPath(item.path), item.retrieveCount, item.retrieveTotalMs, item.retrieveMaxMs, totalMs))
	}

	slices.SortFunc(agentItems, func(a, b agentTiming) int {
		return a.createdAt.Compare(b.createdAt)
	})
	for _, item := range agentItems {
		stageItems = append(stageItems, fmt.Sprintf("agent[%s|%s|%s]=%dms", formatTimingPath(item.path), item.agentName, item.agentType, item.durationMs))
	}

	if len(stageItems) == 0 {
		return "-"
	}
	return strings.Join(stageItems, "; ")
}

func ensureKnowledgeTiming(items map[string]*knowledgeTiming, key string, path []string) *knowledgeTiming {
	if item, ok := items[key]; ok {
		return item
	}
	item := &knowledgeTiming{path: append([]string(nil), path...)}
	items[key] = item
	return item
}

func payloadInt64(value any) int64 {
	switch v := value.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	default:
		return 0
	}
}

func chooseLaterTime(left, right time.Time) time.Time {
	if right.After(left) {
		return right
	}
	return left
}

func durationBetweenMs(startedAt, finishedAt time.Time) int64 {
	if startedAt.IsZero() || finishedAt.IsZero() || finishedAt.Before(startedAt) {
		return 0
	}
	return finishedAt.Sub(startedAt).Milliseconds()
}

func formatTimingPath(path []string) string {
	path = sanitizePath(path)
	if len(path) == 0 {
		return "-"
	}
	return strings.Join(path, " / ")
}
