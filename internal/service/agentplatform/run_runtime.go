package agentplatform

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/gogf/gf/v2/frame/g"

	itsmv1 "lakeside/api/itsm/v1"
	"lakeside/internal/service/agentplatform/eventctx"
)

func (s *Service) launchQueryRun(baseCtx context.Context, req *executeQueryRequest) {
	if s == nil || req == nil {
		return
	}
	runCtx, cancel := context.WithCancel(baseCtx)
	runCtx = s.withRunContext(runCtx, req.RunID, req.AssistantKey, req.SessionID)
	s.setLiveRun(req.RunID, cancel)
	go func() {
		defer s.clearLiveRun(req.RunID)
		if err := s.repo.UpdateRunStatus(withoutCancel(runCtx), req.RunID, runStatusRunning); err != nil {
			g.Log().Errorf(runCtx, "agent run update status failed, run_id=%s err=%v", req.RunID, err)
		}
		s.emitRunEvent(runCtx, eventTypeRunStarted, []string{req.AssistantKey}, "run started", g.Map{
			"run_kind":      runKindQuery,
			"assistant_key": req.AssistantKey,
			"session_id":    req.SessionID,
		})
		resp, err := s.executeQuery(runCtx, req)
		s.finishRun(runCtx, runKindQuery, req.AssistantKey, req.SessionID, req.UserUPN, req.Language, req.CreatedAt, req.CheckpointID, resp, err)
	}()
}

func (s *Service) launchResumeRun(baseCtx context.Context, req *executeResumeRequest) {
	if s == nil || req == nil {
		return
	}
	runCtx, cancel := context.WithCancel(baseCtx)
	runCtx = s.withRunContext(runCtx, req.RunID, req.AssistantKey, req.SessionID)
	s.setLiveRun(req.RunID, cancel)
	go func() {
		defer s.clearLiveRun(req.RunID)
		if err := s.repo.UpdateRunStatus(withoutCancel(runCtx), req.RunID, runStatusRunning); err != nil {
			g.Log().Errorf(runCtx, "agent run update status failed, run_id=%s err=%v", req.RunID, err)
		}
		s.emitRunEvent(runCtx, eventTypeRunStarted, []string{req.AssistantKey}, "run started", g.Map{
			"run_kind":      runKindResume,
			"assistant_key": req.AssistantKey,
			"session_id":    req.SessionID,
		})
		resp, err := s.executeResume(runCtx, req)
		s.finishRun(runCtx, runKindResume, req.AssistantKey, req.SessionID, req.UserUPN, req.Language, req.CreatedAt, req.CheckpointID, resp, err)
	}()
}

func (s *Service) executeQuery(ctx context.Context, req *executeQueryRequest) (*Response, error) {
	assistantContext, err := s.buildAssistantContext(withoutCancel(ctx), req.AssistantKey, req.UserUPN)
	if err != nil {
		return nil, err
	}
	bundle, ok := s.registry.bundles[req.AssistantKey]
	if !ok || bundle == nil || bundle.Runner == nil {
		return nil, errors.New("assistant runner not found")
	}
	g.Log().Infof(ctx, "agent run query started, assistant_key=%s run_id=%s session_id=%s checkpoint_id=%s", req.AssistantKey, req.RunID, req.SessionID, req.CheckpointID)
	iter := bundle.Runner.Query(ctx, req.Message,
		adk.WithCheckPointID(req.CheckpointID),
		adk.WithSessionValues(g.Map{
			"assistant_key":       req.AssistantKey,
			"session_id":          req.SessionID,
			"checkpoint_id":       req.CheckpointID,
			"user_upn":            req.UserUPN,
			"user_code":           req.UserUPN,
			"assistant_context":   assistantContext,
			"latest_user_message": req.Message,
			"preferred_language":  req.Language,
		}),
		s.callbackOpt,
	)
	return s.consumeIterator(ctx, req.AssistantKey, iter, req.SessionID, req.CheckpointID, req.Language), nil
}

func (s *Service) executeResume(ctx context.Context, req *executeResumeRequest) (*Response, error) {
	assistantContext, err := s.buildAssistantContext(withoutCancel(ctx), req.AssistantKey, req.UserUPN)
	if err != nil {
		return nil, err
	}
	bundle, ok := s.registry.bundles[req.AssistantKey]
	if !ok || bundle == nil || bundle.Runner == nil {
		return nil, errors.New("assistant runner not found")
	}
	resumeTargets, err := s.buildResumeTargets(req.Targets)
	if err != nil {
		return nil, err
	}
	latestMessage := summarizeTargets(req.Targets, req.Language)
	g.Log().Infof(ctx, "agent run resume started, assistant_key=%s run_id=%s session_id=%s checkpoint_id=%s target_count=%d", req.AssistantKey, req.RunID, req.SessionID, req.CheckpointID, len(resumeTargets))
	iter, err := bundle.Runner.ResumeWithParams(ctx, req.CheckpointID, &adk.ResumeParams{Targets: resumeTargets},
		adk.WithSessionValues(g.Map{
			"assistant_key":       req.AssistantKey,
			"session_id":          req.SessionID,
			"checkpoint_id":       req.CheckpointID,
			"user_upn":            req.UserUPN,
			"user_code":           req.UserUPN,
			"assistant_context":   assistantContext,
			"latest_user_message": latestMessage,
			"preferred_language":  req.Language,
		}),
		s.callbackOpt,
	)
	if err != nil {
		return nil, err
	}
	return s.consumeIterator(ctx, req.AssistantKey, iter, req.SessionID, req.CheckpointID, req.Language), nil
}

func (s *Service) finishRun(ctx context.Context, kind, assistantKey, sessionID, userUPN, language string, createdAt time.Time, originalCheckpointID string, resp *Response, execErr error) {
	persistCtx := withoutCancel(ctx)
	runID := eventctx.RunID(ctx)
	cancelRequested := s.isRunCancelRequested(runID)
	finishedAt := time.Now()
	status := deriveRunStatus(resp, execErr, cancelRequested)

	if resp == nil {
		resp = s.errorResponse(assistantKey, sessionID, []string{assistantKey}, formatError(execErr))
	}
	if cancelRequested {
		resp = s.errorResponse(assistantKey, sessionID, resp.ActivePath, localizeText(language, "运行已取消。", "Run canceled."))
		resp.CheckpointID = ""
	}
	if status == runStatusFailed {
		resp.Status = "error"
		if resp.Result == nil {
			resp.Result = &Result{Success: false, Message: formatError(execErr)}
		}
	}

	sessionUpdated := false
	switch status {
	case runStatusWaitingInput:
		if err := s.persistSessionAndResponse(persistCtx, assistantKey, userUPN, language, createdAt, resp); err != nil {
			g.Log().Errorf(ctx, "persist waiting_input response failed, run_id=%s err=%v", runID, err)
		}
		s.enqueueMemoryJob(assistantKey, sessionID, userUPN, language)
		sessionUpdated = true
	case runStatusDone:
		if err := s.persistSessionAndResponse(persistCtx, assistantKey, userUPN, language, createdAt, resp); err != nil {
			g.Log().Errorf(ctx, "persist completed response failed, run_id=%s err=%v", runID, err)
		}
		s.enqueueMemoryJob(assistantKey, sessionID, userUPN, language)
		sessionUpdated = true
	case runStatusFailed:
		if kind == runKindQuery {
			resp.CheckpointID = ""
			s.invalidateCheckpoint(persistCtx, assistantKey, originalCheckpointID)
			if err := s.saveStoppedSession(persistCtx, assistantKey, sessionID, userUPN, language, createdAt, resp.ActivePath, false); err != nil {
				g.Log().Errorf(ctx, "save failed session state failed, run_id=%s err=%v", runID, err)
			} else if err := s.persistAssistantResponse(persistCtx, assistantKey, userUPN, language, resp); err != nil {
				g.Log().Errorf(ctx, "persist failed assistant response failed, run_id=%s err=%v", runID, err)
			}
			sessionUpdated = true
		}
	case runStatusCancelled:
		if kind == runKindQuery {
			s.invalidateCheckpoint(persistCtx, assistantKey, originalCheckpointID)
			if err := s.saveStoppedSession(persistCtx, assistantKey, sessionID, userUPN, language, createdAt, resp.ActivePath, true); err != nil {
				g.Log().Errorf(ctx, "save cancelled session state failed, run_id=%s err=%v", runID, err)
			}
			sessionUpdated = true
		}
	}
	if !sessionUpdated && status != runStatusWaitingInput {
		g.Log().Debugf(ctx, "run finished without mutating session state, run_id=%s run_status=%s session_id=%s", runID, status, sessionID)
	}

	checkpointID := resp.CheckpointID
	if status != runStatusWaitingInput {
		checkpointID = ""
	}
	responseJSON := toJSONString(resp)
	errorMessage := ""
	if status == runStatusFailed {
		errorMessage = chooseMessage(resp.Result.Message, formatError(execErr))
	}
	if status == runStatusCancelled {
		errorMessage = localizeText(language, "运行已取消。", "Run canceled.")
	}
	if err := s.repo.FinishRun(persistCtx, runID, status, responseJSON, checkpointID, errorMessage, finishedAt); err != nil {
		g.Log().Errorf(ctx, "finish run failed, run_id=%s err=%v", runID, err)
	}

	s.emitFinalRunEvent(ctx, status, resp, errorMessage)
	s.logRunTimingSummary(ctx, runID, assistantKey, sessionID, status, createdAt, finishedAt)
	g.Log().Infof(ctx, "agent run finished, assistant_key=%s run_id=%s session_id=%s run_status=%s flow_status=%s", assistantKey, runID, sessionID, status, resp.Status)
}

func (s *Service) emitFinalRunEvent(ctx context.Context, runStatus string, resp *Response, errorMessage string) {
	payload := g.Map{
		"run_status":    runStatus,
		"session_id":    resp.SessionID,
		"checkpoint_id": resp.CheckpointID,
		"active_path":   resp.ActivePath,
	}
	if resp.Result != nil {
		payload["result"] = resp.Result
	}
	switch runStatus {
	case runStatusWaitingInput:
		s.emitRunEvent(ctx, eventTypeRunWaitingInput, pathOrDefault(resp.ActivePath, eventctx.AssistantKey(ctx)), waitingInputMessage(resp), payload)
	case runStatusDone:
		s.emitRunEvent(ctx, eventTypeRunCompleted, pathOrDefault(resp.ActivePath, eventctx.AssistantKey(ctx)), chooseMessage(resp.ResultMessage(), resp.Status), payload)
	case runStatusCancelled:
		s.emitRunEvent(ctx, eventTypeRunCancelled, pathOrDefault(resp.ActivePath, eventctx.AssistantKey(ctx)), errorMessage, payload)
	default:
		payload["error_message"] = errorMessage
		s.emitRunEvent(ctx, eventTypeRunFailed, pathOrDefault(resp.ActivePath, eventctx.AssistantKey(ctx)), errorMessage, payload)
	}
}

func waitingInputMessage(resp *Response) string {
	if resp == nil {
		return "waiting_input"
	}
	for i := len(resp.Interrupts) - 1; i >= 0; i-- {
		if prompt := strings.TrimSpace(resp.Interrupts[i].Prompt); prompt != "" {
			return prompt
		}
	}
	for i := len(resp.Steps) - 1; i >= 0; i-- {
		step := resp.Steps[i]
		if step.Kind != stepKindITSMInterrupt {
			continue
		}
		if msg := strings.TrimSpace(step.Message); msg != "" {
			return msg
		}
	}
	return chooseMessage(resp.Status, "waiting_input")
}

func (s *Service) saveStoppedSession(ctx context.Context, assistantKey, sessionID, userUPN, language string, createdAt time.Time, activePath []string, cancelled bool) error {
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	status := statusDone
	if cancelled {
		status = statusDone
	}
	return s.repo.SaveSession(ctx, SessionRecord{
		AssistantKey:     strings.TrimSpace(assistantKey),
		SessionID:        sessionID,
		UserUPN:          strings.TrimSpace(userUPN),
		ActivePathJSON:   marshalPath(pathOrDefault(activePath, assistantKey)),
		ActiveCheckpoint: "",
		Status:           status,
		Language:         language,
		CreatedAt:        createdAt,
		UpdatedAt:        time.Now(),
	})
}

func deriveRunStatus(resp *Response, execErr error, cancelRequested bool) string {
	if cancelRequested {
		return runStatusCancelled
	}
	if execErr != nil {
		return runStatusFailed
	}
	if resp == nil {
		return runStatusFailed
	}
	switch resp.Status {
	case "need_info", "need_confirm":
		return runStatusWaitingInput
	case "done":
		return runStatusDone
	default:
		return runStatusFailed
	}
}

func buildRunSnapshot(run *RunRecord) (*RunSnapshot, error) {
	if run == nil {
		return nil, nil
	}
	snapshot := &RunSnapshot{
		RunID:        run.RunID,
		AssistantKey: run.AssistantKey,
		SessionID:    run.SessionID,
		RunStatus:    run.Status,
		CheckpointID: run.CheckpointID,
		ErrorMessage: run.ErrorMessage,
		StartedAt:    run.StartedAt,
		FinishedAt:   run.FinishedAt,
	}
	if !isRunTerminal(run.Status) {
		snapshot.FinishedAt = time.Time{}
	}
	if strings.TrimSpace(run.ResponseJSON) == "" || strings.TrimSpace(run.ResponseJSON) == "{}" {
		return snapshot, nil
	}
	resp, err := decodeJSON[Response](run.ResponseJSON)
	if err != nil {
		return nil, err
	}
	snapshot.Status = resp.Status
	snapshot.CheckpointID = chooseMessage(resp.CheckpointID, run.CheckpointID)
	snapshot.ActivePath = append([]string(nil), resp.ActivePath...)
	snapshot.Steps = append([]StepResult(nil), resp.Steps...)
	snapshot.Interrupts = append([]itsmv1.AgentInterrupt(nil), resp.Interrupts...)
	snapshot.Result = resp.Result
	if snapshot.AssistantKey == "" {
		snapshot.AssistantKey = resp.AssistantKey
	}
	if snapshot.SessionID == "" {
		snapshot.SessionID = resp.SessionID
	}
	return snapshot, nil
}

func (s *Service) withRunContext(ctx context.Context, runID, assistantKey, sessionID string) context.Context {
	return eventctx.WithRun(ctx, runID, assistantKey, sessionID, func(nodeName string) []string {
		if strings.TrimSpace(nodeName) == "" {
			return []string{assistantKey}
		}
		if path := s.registry.pathForNode(assistantKey, nodeName); len(path) > 0 {
			return path
		}
		return []string{assistantKey}
	}, s.emitRunEvent)
}

func (s *Service) emitRunEvent(ctx context.Context, eventType string, path []string, message string, payload any) {
	if s == nil {
		return
	}
	runID := eventctx.RunID(ctx)
	if strings.TrimSpace(runID) == "" {
		return
	}
	now := time.Now()
	if s.shouldDropAgentLifecycleEvent(runID, eventType, path, payload, now) {
		return
	}
	record := RunEventRecord{
		RunID:        runID,
		AssistantKey: eventctx.AssistantKey(ctx),
		SessionID:    eventctx.SessionID(ctx),
		EventType:    strings.TrimSpace(eventType),
		PathJSON:     marshalPath(path),
		Message:      strings.TrimSpace(message),
		PayloadJSON:  toJSONString(payload),
		CreatedAt:    now,
	}
	id, err := s.repo.AppendRunEvent(withoutCancel(ctx), record)
	if err != nil {
		g.Log().Errorf(ctx, "append run event failed, run_id=%s event_type=%s err=%v", runID, eventType, err)
		return
	}
	record.ID = id
	s.broadcastRunEvent(record)
	if s.redisRuntime != nil {
		s.redisRuntime.publishRunEvent(withoutCancel(ctx), runID)
	}
}

func (s *Service) shouldDropAgentLifecycleEvent(runID, eventType string, path []string, payload any, now time.Time) bool {
	switch strings.TrimSpace(eventType) {
	case eventTypeAgentEntered, eventTypeAgentCompleted:
	default:
		return false
	}
	agentName, agentType := parseAgentEventIdentity(payload)
	if agentName == "" || agentType == "" {
		// 过滤无法识别身份的生命周期事件，避免前端出现 agent_type="" 的噪声。
		return true
	}
	pathKey := strings.Join(path, ">")
	dedupKey := strings.Join([]string{
		strings.TrimSpace(eventType),
		strings.TrimSpace(pathKey),
		agentName,
		agentType,
	}, "|")
	const dedupWindow = 300 * time.Millisecond

	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	items := s.eventDedup[runID]
	if items == nil {
		items = make(map[string]agentEventDedupState)
		s.eventDedup[runID] = items
	}
	if prev, ok := items[dedupKey]; ok && now.Sub(prev.lastSeenAt) <= dedupWindow {
		return true
	}
	items[dedupKey] = agentEventDedupState{lastSeenAt: now}
	return false
}

func parseAgentEventIdentity(payload any) (name string, agentType string) {
	switch v := payload.(type) {
	case map[string]any:
		name = strings.TrimSpace(toString(v["agent_name"]))
		agentType = strings.TrimSpace(toString(v["agent_type"]))
	}
	return name, agentType
}

func toString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}

func (s *Service) setLiveRun(runID string, cancel context.CancelFunc) {
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	s.liveRuns[runID] = &liveRun{cancel: cancel}
}

func (s *Service) clearLiveRun(runID string) {
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	delete(s.liveRuns, runID)
	delete(s.eventDedup, runID)
}

func (s *Service) requestRunCancel(runID string) bool {
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	item, ok := s.liveRuns[runID]
	if !ok || item == nil || item.cancel == nil {
		return false
	}
	item.cancelRequested = true
	item.cancel()
	return true
}

func (s *Service) isRunCancelRequested(runID string) bool {
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	item, ok := s.liveRuns[runID]
	return ok && item != nil && item.cancelRequested
}

func (s *Service) subscribeRun(runID string) (<-chan RunEventRecord, func()) {
	ch := make(chan RunEventRecord, 64)
	s.liveMu.Lock()
	if _, ok := s.subscribers[runID]; !ok {
		s.subscribers[runID] = make(map[chan RunEventRecord]struct{})
	}
	s.subscribers[runID][ch] = struct{}{}
	s.liveMu.Unlock()
	return ch, func() {
		s.liveMu.Lock()
		defer s.liveMu.Unlock()
		if items, ok := s.subscribers[runID]; ok {
			if _, exists := items[ch]; exists {
				delete(items, ch)
				close(ch)
			}
			if len(items) == 0 {
				delete(s.subscribers, runID)
			}
		}
	}
}

func (s *Service) broadcastRunEvent(event RunEventRecord) {
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	items := s.subscribers[event.RunID]
	for ch := range items {
		select {
		case ch <- event:
		default:
			delete(items, ch)
			close(ch)
		}
	}
	if len(items) == 0 {
		delete(s.subscribers, event.RunID)
	}
}

func isRunTerminal(status string) bool {
	switch strings.TrimSpace(status) {
	case runStatusWaitingInput, runStatusDone, runStatusFailed, runStatusCancelled:
		return true
	default:
		return false
	}
}

func (r *Response) ResultMessage() string {
	if r == nil || r.Result == nil {
		return ""
	}
	return strings.TrimSpace(r.Result.Message)
}
