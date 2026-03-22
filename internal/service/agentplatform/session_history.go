package agentplatform

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gcode"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/google/uuid"
)

func (s *Service) CreateSession(ctx context.Context, req *CreateSessionRequest) (*CreateSessionResult, error) {
	if req == nil || strings.TrimSpace(req.AssistantKey) == "" || strings.TrimSpace(req.UserUPN) == "" {
		return nil, gerror.NewCode(gcode.CodeMissingParameter, "assistant_key and X-User-ID are required")
	}
	assistantKey := strings.TrimSpace(req.AssistantKey)
	userUPN := strings.TrimSpace(req.UserUPN)
	now := time.Now()
	sessionID := "sess-" + uuid.NewString()
	language := chooseLanguage(strings.TrimSpace(req.Language), "zh")
	if err := s.repo.SaveSession(ctx, SessionRecord{
		AssistantKey:     assistantKey,
		SessionID:        sessionID,
		UserUPN:          userUPN,
		ActivePathJSON:   marshalPath([]string{assistantKey}),
		ActiveCheckpoint: "",
		Status:           statusActive,
		Language:         language,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		return nil, err
	}
	return &CreateSessionResult{
		AssistantKey: assistantKey,
		SessionID:    sessionID,
	}, nil
}

func (s *Service) ListSessions(ctx context.Context, req *ListSessionsRequest) ([]SessionSummary, error) {
	if req == nil || strings.TrimSpace(req.AssistantKey) == "" || strings.TrimSpace(req.UserUPN) == "" {
		return nil, gerror.NewCode(gcode.CodeMissingParameter, "assistant_key and X-User-ID are required")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	sessions, err := s.repo.ListSessions(ctx, strings.TrimSpace(req.AssistantKey), strings.TrimSpace(req.UserUPN), limit)
	if err != nil {
		return nil, err
	}

	result := make([]SessionSummary, 0, len(sessions))
	for _, session := range sessions {
		messages, msgErr := s.repo.ListRecentMessages(ctx, session.SessionID, 10)
		if msgErr != nil {
			return nil, msgErr
		}
		runs, runErr := s.repo.ListRunsBySession(ctx, session.SessionID)
		if runErr != nil {
			return nil, runErr
		}
		result = append(result, buildSessionSummary(session, messages, runs))
	}
	return result, nil
}

func (s *Service) GetSessionDetail(ctx context.Context, req *GetSessionRequest) (*SessionDetail, error) {
	if req == nil || strings.TrimSpace(req.AssistantKey) == "" || strings.TrimSpace(req.SessionID) == "" || strings.TrimSpace(req.UserUPN) == "" {
		return nil, gerror.NewCode(gcode.CodeMissingParameter, "assistant_key, session_id and X-User-ID are required")
	}

	session, err := s.repo.GetSession(ctx, strings.TrimSpace(req.SessionID))
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, gerror.NewCodef(gcode.CodeNotFound, "session not found: %s", req.SessionID)
	}
	if strings.EqualFold(strings.TrimSpace(session.Status), statusDeleted) {
		return nil, gerror.NewCodef(gcode.CodeNotFound, "session not found: %s", req.SessionID)
	}
	if session.AssistantKey != strings.TrimSpace(req.AssistantKey) || session.UserUPN != strings.TrimSpace(req.UserUPN) {
		return nil, gerror.NewCode(gcode.CodeNotAuthorized, "session does not belong to current assistant/user")
	}

	messages, err := s.repo.ListMessages(ctx, session.SessionID)
	if err != nil {
		return nil, err
	}
	runs, err := s.repo.ListRunsBySession(ctx, session.SessionID)
	if err != nil {
		return nil, err
	}

	detail := &SessionDetail{
		Session:  buildSessionSummary(*session, messages, runs),
		Messages: buildSessionMessages(messages),
		Runs:     make([]RunTrace, 0, len(runs)),
	}
	for _, run := range runs {
		snapshot, snapErr := buildRunSnapshot(&run)
		if snapErr != nil {
			return nil, snapErr
		}
		events, eventErr := s.repo.ListRunEventsAfter(ctx, run.RunID, 0)
		if eventErr != nil {
			return nil, eventErr
		}
		detail.Runs = append(detail.Runs, RunTrace{
			Snapshot: snapshot,
			Events:   events,
		})
	}
	return detail, nil
}

func (s *Service) DeleteSession(ctx context.Context, req *DeleteSessionRequest) error {
	if req == nil || strings.TrimSpace(req.AssistantKey) == "" || strings.TrimSpace(req.SessionID) == "" || strings.TrimSpace(req.UserUPN) == "" {
		return gerror.NewCode(gcode.CodeMissingParameter, "assistant_key, session_id and X-User-ID are required")
	}

	session, err := s.repo.GetSession(ctx, strings.TrimSpace(req.SessionID))
	if err != nil {
		return err
	}
	if session == nil || strings.EqualFold(strings.TrimSpace(session.Status), statusDeleted) {
		return gerror.NewCodef(gcode.CodeNotFound, "session not found: %s", req.SessionID)
	}
	if session.AssistantKey != strings.TrimSpace(req.AssistantKey) || session.UserUPN != strings.TrimSpace(req.UserUPN) {
		return gerror.NewCode(gcode.CodeNotAuthorized, "session does not belong to current assistant/user")
	}

	runs, err := s.repo.ListRunsBySession(ctx, session.SessionID)
	if err != nil {
		return err
	}
	if len(runs) > 0 {
		lastRun := runs[len(runs)-1]
		switch strings.TrimSpace(lastRun.Status) {
		case runStatusQueued, runStatusRunning:
			return gerror.NewCode(gcode.CodeOperationFailed, "cannot delete a session with an active run")
		case runStatusWaitingInput:
			if err := s.abandonWaitingInputRun(ctx, session, &lastRun); err != nil {
				return err
			}
		}
	}

	return s.repo.DeleteSession(ctx, strings.TrimSpace(req.AssistantKey), strings.TrimSpace(req.SessionID), strings.TrimSpace(req.UserUPN), time.Now())
}

func (s *Service) abandonWaitingInputRun(ctx context.Context, session *SessionRecord, run *RunRecord) error {
	if s == nil || session == nil || run == nil {
		return nil
	}
	errorMessage := localizeText(session.Language, "会话已删除，当前流程已放弃。", "Session deleted; pending flow abandoned.")
	activePath := unmarshalPath(session.ActivePathJSON)
	resp := s.errorResponse(run.AssistantKey, run.SessionID, pathOrDefault(activePath, run.AssistantKey), errorMessage)
	resp.CheckpointID = ""
	if err := s.repo.FinishRun(ctx, run.RunID, runStatusCancelled, toJSONString(resp), "", errorMessage, time.Now()); err != nil {
		return err
	}
	s.invalidateCheckpoint(ctx, run.AssistantKey, run.CheckpointID)
	runCtx := s.withRunContext(ctx, run.RunID, run.AssistantKey, run.SessionID)
	s.emitFinalRunEvent(runCtx, runStatusCancelled, resp, errorMessage)
	return nil
}

func buildSessionSummary(session SessionRecord, messages []MessageRecord, runs []RunRecord) SessionSummary {
	summary := SessionSummary{
		AssistantKey: session.AssistantKey,
		SessionID:    session.SessionID,
		Title:        buildSessionTitle(messages),
		Status:       session.Status,
		ActivePath:   unmarshalPath(session.ActivePathJSON),
		CreatedAt:    session.CreatedAt,
		UpdatedAt:    session.UpdatedAt,
	}
	if len(runs) > 0 {
		lastRun := runs[len(runs)-1]
		summary.LastRunID = lastRun.RunID
		summary.LastRunStatus = lastRun.Status
	}
	return summary
}

func buildSessionMessages(messages []MessageRecord) []SessionMessage {
	if len(messages) == 0 {
		return nil
	}
	result := make([]SessionMessage, 0, len(messages))
	for _, message := range messages {
		result = append(result, SessionMessage{
			ID:           message.ID,
			Role:         strings.TrimSpace(message.Role),
			Content:      strings.TrimSpace(message.Content),
			ActivePath:   unmarshalPath(message.ActivePathJSON),
			CheckpointID: strings.TrimSpace(message.CheckpointID),
			CreatedAt:    message.CreatedAt,
		})
	}
	return result
}

func buildSessionTitle(messages []MessageRecord) string {
	for _, message := range messages {
		if strings.TrimSpace(message.Role) != "user" {
			continue
		}
		return shortenTitle(message.Content)
	}
	return "新对话"
}

func shortenTitle(content string) string {
	content = strings.TrimSpace(strings.ReplaceAll(content, "\n", " "))
	if content == "" {
		return "新对话"
	}
	runes := []rune(content)
	if len(runes) <= 28 {
		return content
	}
	return string(runes[:28]) + "..."
}

func formatSessionTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}
