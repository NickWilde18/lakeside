package agentplatform

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/gogf/gf/v2/errors/gcode"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"

	itsmv1 "lakeside/api/itsm/v1"
	legacyitsm "lakeside/internal/service/itsmagent"
)

type liveRun struct {
	cancel          context.CancelFunc
	cancelRequested bool
}

type agentEventDedupState struct {
	lastSeenAt time.Time
}

type Service struct {
	repo            Repository
	redisRuntime    *redisRuntime
	cfg             *config
	registry        *runtimeRegistry
	memoryExtractor *MemoryExtractor
	memoryJobs      chan MemoryJob
	memoryLimit     int
	callbackOpt     adk.AgentRunOption

	liveMu      sync.Mutex
	liveRuns    map[string]*liveRun
	subscribers map[string]map[chan RunEventRecord]struct{}
	eventDedup  map[string]map[string]agentEventDedupState

	workerOnce sync.Once
	workerErr  error
}

var (
	serviceOnce sync.Once
	serviceIns  *Service
)

func GetService(ctx context.Context) *Service {
	serviceOnce.Do(func() {
		cfg, err := loadConfig(ctx)
		if err != nil {
			panic(err)
		}
		repo, err := newRepository(ctx, cfg)
		if err != nil {
			panic(err)
		}
		registry, err := newRuntimeRegistry(ctx, cfg)
		if err != nil {
			panic(err)
		}
		names := append([]string(nil), registry.names...)
		sort.Strings(names)
		callbackOpt := adk.WithCallbacks(newAgentCallbackHandler(names...)).DesignateAgent(names...)
		serviceIns = &Service{
			repo:            repo,
			redisRuntime:    newRedisRuntime(ctx, cfg),
			cfg:             cfg,
			registry:        registry,
			memoryExtractor: NewMemoryExtractor(),
			memoryJobs:      make(chan MemoryJob, cfg.Memory.QueueSize),
			memoryLimit:     cfg.Memory.MaxItems,
			callbackOpt:     callbackOpt,
			liveRuns:        make(map[string]*liveRun),
			subscribers:     make(map[string]map[chan RunEventRecord]struct{}),
			eventDedup:      make(map[string]map[string]agentEventDedupState),
		}
		workers := cfg.Memory.Workers
		if workers <= 0 {
			workers = 1
		}
		for i := 0; i < workers; i++ {
			go runMemoryWorker(context.Background(), repo, serviceIns.memoryExtractor, serviceIns.memoryJobs)
		}
	})
	return serviceIns
}

func (s *Service) StartWorkers(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("agentplatform service is nil")
	}
	s.workerOnce.Do(func() {
		if s.redisRuntime == nil {
			s.workerErr = fmt.Errorf("redis runtime not initialized")
			return
		}
		if err := s.redisRuntime.ensureConsumerGroup(ctx); err != nil {
			s.workerErr = err
			return
		}
		go func() {
			_ = s.redisRuntime.consumeCancelRequests(context.Background(), func(runID string) {
				_ = s.requestRunCancel(runID)
			})
		}()
		workerCount := s.cfg.Runtime.WorkerCount
		if workerCount <= 0 {
			workerCount = 1
		}
		for i := 0; i < workerCount; i++ {
			consumerName := fmt.Sprintf("%s-%d", chooseMessage(s.cfg.Runtime.ConsumerPrefix, "agent-worker"), i+1)
			go func(name string) {
				_ = s.redisRuntime.consumeRuns(context.Background(), name, 16, s.processQueuedRun)
			}(consumerName)
		}
	})
	return s.workerErr
}

func (s *Service) processQueuedRun(ctx context.Context, runID string) error {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil
	}
	run, err := s.repo.GetRun(ctx, runID)
	if err != nil || run == nil {
		return nil
	}
	started, err := s.repo.TryStartRun(ctx, runID)
	if err != nil || !started {
		return err
	}
	session, err := s.repo.GetSession(ctx, run.SessionID)
	if err != nil || session == nil {
		finishErr := fmt.Sprintf("session not found: %s", run.SessionID)
		if err != nil {
			finishErr = err.Error()
		}
		_ = s.repo.FinishRun(ctx, run.RunID, runStatusFailed, "{}", "", finishErr, time.Now())
		return nil
	}
	switch strings.TrimSpace(run.Kind) {
	case runKindQuery:
		payload, payloadErr := decodeJSON[struct {
			Message string `json:"message"`
		}](run.RequestJSON)
		if payloadErr != nil {
			g.Log().Warningf(ctx, "decode query run payload failed, run_id=%s err=%v", run.RunID, payloadErr)
		}
		execReq := &executeQueryRequest{
			AssistantKey: run.AssistantKey,
			RunID:        run.RunID,
			SessionID:    run.SessionID,
			UserUPN:      run.UserUPN,
			Message:      strings.TrimSpace(payload.Message),
			CheckpointID: run.CheckpointID,
			Language:     chooseLanguage(session.Language, detectLanguage(payload.Message)),
			CreatedAt:    run.StartedAt,
		}
		runCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		runCtx = s.withRunContext(runCtx, run.RunID, run.AssistantKey, run.SessionID)
		s.setLiveRun(run.RunID, cancel)
		defer s.clearLiveRun(run.RunID)
		s.emitRunEvent(runCtx, eventTypeRunStarted, []string{run.AssistantKey}, "run started", g.Map{
			"run_kind":      runKindQuery,
			"assistant_key": run.AssistantKey,
			"session_id":    run.SessionID,
		})
		resp, execErr := s.executeQuery(runCtx, execReq)
		s.finishRun(runCtx, runKindQuery, run.AssistantKey, run.SessionID, run.UserUPN, execReq.Language, run.StartedAt, run.CheckpointID, resp, execErr)
	case runKindResume:
		payload, payloadErr := decodeJSON[struct {
			Targets map[string]*itsmv1.ResumeTarget `json:"targets"`
		}](run.RequestJSON)
		if payloadErr != nil {
			g.Log().Warningf(ctx, "decode resume run payload failed, run_id=%s err=%v", run.RunID, payloadErr)
		}
		execReq := &executeResumeRequest{
			AssistantKey: run.AssistantKey,
			RunID:        run.RunID,
			SessionID:    run.SessionID,
			CheckpointID: run.CheckpointID,
			UserUPN:      run.UserUPN,
			Targets:      payload.Targets,
			Language:     chooseLanguage(session.Language, targetsLanguage(payload.Targets)),
			CreatedAt:    run.StartedAt,
		}
		runCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		runCtx = s.withRunContext(runCtx, run.RunID, run.AssistantKey, run.SessionID)
		s.setLiveRun(run.RunID, cancel)
		defer s.clearLiveRun(run.RunID)
		s.emitRunEvent(runCtx, eventTypeRunStarted, []string{run.AssistantKey}, "run started", g.Map{
			"run_kind":      runKindResume,
			"assistant_key": run.AssistantKey,
			"session_id":    run.SessionID,
		})
		resp, execErr := s.executeResume(runCtx, execReq)
		s.finishRun(runCtx, runKindResume, run.AssistantKey, run.SessionID, run.UserUPN, execReq.Language, run.StartedAt, run.CheckpointID, resp, execErr)
	default:
		_ = s.repo.FinishRun(ctx, run.RunID, runStatusFailed, "{}", "", "unsupported run kind", time.Now())
	}
	return nil
}

func (s *Service) CreateRun(ctx context.Context, req *CreateRunRequest) (*CreateRunResult, error) {
	if err := s.validateCreateRunRequest(req); err != nil {
		return nil, err
	}
	assistantKey := strings.TrimSpace(req.AssistantKey)
	userUPN := strings.TrimSpace(req.UserUPN)
	sessionID := strings.TrimSpace(req.SessionID)
	message := strings.TrimSpace(req.Message)
	bundle, ok := s.registry.bundles[assistantKey]
	if !ok || bundle == nil || bundle.Runner == nil {
		return nil, gerror.NewCodef(gcode.CodeNotFound, "assistant_key not found: %s", assistantKey)
	}

	startedAt := time.Now()
	runID := "run-" + uuid.NewString()
	checkpointID := genCheckpointID()
	language := detectLanguage(message)
	persistCtx := withoutCancel(ctx)

	if sessionID == "" {
		sessionID = "sess-" + uuid.NewString()
	} else {
		session, err := s.repo.GetSession(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		if session == nil || strings.TrimSpace(session.Status) == statusDeleted {
			return nil, gerror.NewCodef(gcode.CodeNotFound, "session not found: %s", sessionID)
		}
		if session.AssistantKey != assistantKey || session.UserUPN != userUPN {
			return nil, gerror.NewCode(gcode.CodeNotAuthorized, "session does not belong to current assistant/user")
		}
		runs, err := s.repo.ListRunsBySession(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		if len(runs) > 0 {
			lastRun := runs[len(runs)-1]
			switch strings.TrimSpace(lastRun.Status) {
			case runStatusQueued, runStatusRunning, runStatusWaitingInput:
				return nil, gerror.NewCodef(gcode.CodeInvalidOperation, "session %s still has unfinished run %s", sessionID, lastRun.RunID)
			}
		}
		language = chooseLanguage(session.Language, language)
	}

	if err := s.repo.SaveSession(persistCtx, SessionRecord{
		AssistantKey:     assistantKey,
		SessionID:        sessionID,
		UserUPN:          userUPN,
		ActivePathJSON:   marshalPath([]string{assistantKey}),
		ActiveCheckpoint: checkpointID,
		Status:           statusActive,
		Language:         language,
		CreatedAt:        startedAt,
		UpdatedAt:        startedAt,
	}); err != nil {
		return nil, err
	}
	if _, err := s.repo.AppendMessage(persistCtx, MessageRecord{
		AssistantKey:   assistantKey,
		SessionID:      sessionID,
		UserUPN:        userUPN,
		Role:           "user",
		Content:        message,
		ActivePathJSON: marshalPath([]string{assistantKey}),
		CheckpointID:   checkpointID,
		Language:       language,
		CreatedAt:      startedAt,
	}); err != nil {
		return nil, err
	}
	if err := s.repo.CreateRun(persistCtx, RunRecord{
		RunID:        runID,
		AssistantKey: assistantKey,
		SessionID:    sessionID,
		UserUPN:      userUPN,
		Kind:         runKindQuery,
		Status:       runStatusQueued,
		RequestJSON:  toJSONString(g.Map{"message": message}),
		CheckpointID: checkpointID,
		StartedAt:    startedAt,
		FinishedAt:   startedAt,
	}); err != nil {
		return nil, err
	}

	if s.redisRuntime == nil {
		return nil, gerror.NewCode(gcode.CodeInternalError, "redis runtime not initialized")
	}
	if err := s.redisRuntime.enqueueRun(persistCtx, runID); err != nil {
		return nil, err
	}

	return &CreateRunResult{
		AssistantKey: assistantKey,
		RunID:        runID,
		SessionID:    sessionID,
		RunStatus:    runStatusQueued,
	}, nil
}

func (s *Service) ResumeRun(ctx context.Context, req *ResumeRunRequest) (*ResumeRunResult, error) {
	if err := s.validateResumeRunRequest(req); err != nil {
		return nil, err
	}
	assistantKey := strings.TrimSpace(req.AssistantKey)
	userUPN := strings.TrimSpace(req.UserUPN)
	runID := strings.TrimSpace(req.RunID)
	bundle, ok := s.registry.bundles[assistantKey]
	if !ok || bundle == nil || bundle.Runner == nil {
		return nil, gerror.NewCodef(gcode.CodeNotFound, "assistant_key not found: %s", assistantKey)
	}
	currentRun, err := s.repo.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if currentRun == nil {
		return nil, gerror.NewCodef(gcode.CodeNotFound, "run not found: %s", runID)
	}
	if currentRun.AssistantKey != assistantKey || currentRun.UserUPN != userUPN {
		return nil, gerror.NewCode(gcode.CodeNotAuthorized, "run does not belong to current assistant/user")
	}
	if currentRun.Status != runStatusWaitingInput {
		return nil, gerror.NewCodef(gcode.CodeInvalidOperation, "run status %s cannot be resumed", currentRun.Status)
	}
	session, err := s.repo.GetSession(ctx, currentRun.SessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, gerror.NewCode(gcode.CodeNotFound, "session not found")
	}
	if session.AssistantKey != assistantKey || session.UserUPN != userUPN {
		return nil, gerror.NewCode(gcode.CodeNotAuthorized, "session does not belong to current assistant/user")
	}
	if strings.TrimSpace(session.ActiveCheckpoint) == "" {
		return nil, gerror.NewCode(gcode.CodeInvalidOperation, "session has no active checkpoint")
	}
	resumeTargets, err := s.buildResumeTargets(req.Targets)
	if err != nil {
		return nil, gerror.NewCode(gcode.CodeInvalidParameter, err.Error())
	}

	startedAt := time.Now()
	newRunID := "run-" + uuid.NewString()
	language := chooseLanguage(session.Language, targetsLanguage(req.Targets))
	message := summarizeTargets(req.Targets, language)
	persistCtx := withoutCancel(ctx)
	if _, err := s.repo.AppendMessage(persistCtx, MessageRecord{
		AssistantKey:   assistantKey,
		SessionID:      session.SessionID,
		UserUPN:        userUPN,
		Role:           "user",
		Content:        message,
		PayloadJSON:    toJSONString(req.Targets),
		ActivePathJSON: session.ActivePathJSON,
		CheckpointID:   session.ActiveCheckpoint,
		Language:       language,
		CreatedAt:      startedAt,
	}); err != nil {
		return nil, err
	}
	if err := s.repo.CreateRun(persistCtx, RunRecord{
		RunID:        newRunID,
		AssistantKey: assistantKey,
		SessionID:    session.SessionID,
		UserUPN:      userUPN,
		Kind:         runKindResume,
		Status:       runStatusQueued,
		ParentRunID:  currentRun.RunID,
		RequestJSON:  toJSONString(g.Map{"targets": req.Targets}),
		CheckpointID: session.ActiveCheckpoint,
		StartedAt:    startedAt,
		FinishedAt:   startedAt,
	}); err != nil {
		return nil, err
	}

	_ = resumeTargets
	if s.redisRuntime == nil {
		return nil, gerror.NewCode(gcode.CodeInternalError, "redis runtime not initialized")
	}
	if err := s.redisRuntime.enqueueRun(persistCtx, newRunID); err != nil {
		return nil, err
	}

	return &ResumeRunResult{
		AssistantKey: assistantKey,
		RunID:        newRunID,
		SessionID:    session.SessionID,
		RunStatus:    runStatusQueued,
	}, nil
}

func (s *Service) GetRun(ctx context.Context, req *GetRunRequest) (*RunSnapshot, error) {
	if req == nil || strings.TrimSpace(req.AssistantKey) == "" || strings.TrimSpace(req.RunID) == "" || strings.TrimSpace(req.UserUPN) == "" {
		return nil, gerror.NewCode(gcode.CodeMissingParameter, "assistant_key, run_id and X-User-ID are required")
	}
	run, err := s.repo.GetRun(ctx, strings.TrimSpace(req.RunID))
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, gerror.NewCodef(gcode.CodeNotFound, "run not found: %s", req.RunID)
	}
	if run.AssistantKey != strings.TrimSpace(req.AssistantKey) || run.UserUPN != strings.TrimSpace(req.UserUPN) {
		return nil, gerror.NewCode(gcode.CodeNotAuthorized, "run does not belong to current assistant/user")
	}
	return buildRunSnapshot(run)
}

func (s *Service) ListRunEvents(ctx context.Context, req *ListRunEventsRequest) ([]RunEventRecord, error) {
	if req == nil || strings.TrimSpace(req.AssistantKey) == "" || strings.TrimSpace(req.RunID) == "" || strings.TrimSpace(req.UserUPN) == "" {
		return nil, gerror.NewCode(gcode.CodeMissingParameter, "assistant_key, run_id and X-User-ID are required")
	}
	run, err := s.repo.GetRun(ctx, strings.TrimSpace(req.RunID))
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, gerror.NewCodef(gcode.CodeNotFound, "run not found: %s", req.RunID)
	}
	if run.AssistantKey != strings.TrimSpace(req.AssistantKey) || run.UserUPN != strings.TrimSpace(req.UserUPN) {
		return nil, gerror.NewCode(gcode.CodeNotAuthorized, "run does not belong to current assistant/user")
	}
	return s.repo.ListRunEventsAfter(ctx, run.RunID, req.AfterID)
}

func (s *Service) CancelRun(ctx context.Context, req *CancelRunRequest) error {
	if req == nil || strings.TrimSpace(req.AssistantKey) == "" || strings.TrimSpace(req.RunID) == "" || strings.TrimSpace(req.UserUPN) == "" {
		return gerror.NewCode(gcode.CodeMissingParameter, "assistant_key, run_id and X-User-ID are required")
	}
	run, err := s.repo.GetRun(ctx, strings.TrimSpace(req.RunID))
	if err != nil {
		return err
	}
	if run == nil {
		return gerror.NewCodef(gcode.CodeNotFound, "run not found: %s", req.RunID)
	}
	if run.AssistantKey != strings.TrimSpace(req.AssistantKey) || run.UserUPN != strings.TrimSpace(req.UserUPN) {
		return gerror.NewCode(gcode.CodeNotAuthorized, "run does not belong to current assistant/user")
	}
	if isRunTerminal(run.Status) {
		return gerror.NewCodef(gcode.CodeInvalidOperation, "run status %s cannot be canceled", run.Status)
	}
	if run.Status == runStatusQueued {
		queuedCancelled, err := s.cancelQueuedRun(withoutCancel(ctx), run)
		if err != nil {
			return err
		}
		if queuedCancelled {
			return nil
		}
		run, err = s.repo.GetRun(ctx, strings.TrimSpace(req.RunID))
		if err != nil {
			return err
		}
		if run == nil {
			return gerror.NewCodef(gcode.CodeNotFound, "run not found: %s", req.RunID)
		}
		if isRunTerminal(run.Status) {
			return gerror.NewCodef(gcode.CodeInvalidOperation, "run status %s cannot be canceled", run.Status)
		}
	}
	_ = s.requestRunCancel(run.RunID)
	if s.redisRuntime != nil {
		if err := s.redisRuntime.publishCancelRequest(withoutCancel(ctx), run.RunID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) cancelQueuedRun(ctx context.Context, run *RunRecord) (bool, error) {
	if s == nil || run == nil {
		return false, nil
	}
	session, err := s.repo.GetSession(ctx, run.SessionID)
	if err != nil {
		return false, err
	}
	language := "zh"
	activePath := []string{run.AssistantKey}
	if session != nil {
		language = chooseLanguage(session.Language, "")
		if path := unmarshalPath(session.ActivePathJSON); len(path) > 0 {
			activePath = path
		}
	}
	errorMessage := localizeText(language, "运行已取消。", "Run canceled.")
	resp := s.errorResponse(run.AssistantKey, run.SessionID, activePath, errorMessage)
	resp.CheckpointID = ""
	cancelled, err := s.repo.TryCancelQueuedRun(ctx, run.RunID, toJSONString(resp), errorMessage, time.Now())
	if err != nil || !cancelled {
		return cancelled, err
	}
	runCtx := s.withRunContext(ctx, run.RunID, run.AssistantKey, run.SessionID)
	if run.Kind == runKindQuery {
		s.invalidateCheckpoint(ctx, run.AssistantKey, run.CheckpointID)
		if session != nil {
			if err := s.saveStoppedSession(ctx, run.AssistantKey, run.SessionID, run.UserUPN, language, run.StartedAt, activePath, true); err != nil {
				g.Log().Warningf(ctx, "save cancelled queued session failed, run_id=%s err=%v", run.RunID, err)
			} else if err := s.persistAssistantResponse(ctx, run.AssistantKey, run.UserUPN, language, resp); err != nil {
				g.Log().Warningf(ctx, "persist cancelled queued assistant response failed, run_id=%s err=%v", run.RunID, err)
			}
		}
	}
	s.emitFinalRunEvent(runCtx, runStatusCancelled, resp, errorMessage)
	return true, nil
}

func (s *Service) SubscribeRun(runID string) (<-chan RunEventRecord, func()) {
	return s.subscribeRun(runID)
}

func (s *Service) SubscribeRunWake(ctx context.Context, runID string) (<-chan struct{}, func()) {
	if s == nil || s.redisRuntime == nil {
		return nil, func() {}
	}
	ch, cancel, err := s.redisRuntime.subscribeRunEvent(ctx, runID)
	if err != nil {
		return nil, func() {}
	}
	return ch, cancel
}

func (s *Service) ListMemories(ctx context.Context, req *ListMemoriesRequest) ([]MemoryRecord, error) {
	if req == nil || strings.TrimSpace(req.AssistantKey) == "" || strings.TrimSpace(req.UserUPN) == "" {
		return nil, fmt.Errorf("assistant_key and user_upn are required")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	return s.repo.ListMemories(ctx, strings.TrimSpace(req.AssistantKey), strings.TrimSpace(req.UserUPN), limit)
}

func (s *Service) ClearMemories(ctx context.Context, req *ClearMemoriesRequest) (int64, error) {
	if req == nil || strings.TrimSpace(req.AssistantKey) == "" || strings.TrimSpace(req.UserUPN) == "" {
		return 0, fmt.Errorf("assistant_key and user_upn are required")
	}
	deleted, err := s.repo.DeleteMemories(ctx, strings.TrimSpace(req.AssistantKey), strings.TrimSpace(req.UserUPN), strings.TrimSpace(req.Category), strings.TrimSpace(req.CanonicalKey))
	if err != nil {
		return 0, err
	}
	g.Log().Infof(ctx, "agent memories cleared, assistant_key=%s user_upn=%s category=%s canonical_key=%s deleted_count=%d", req.AssistantKey, req.UserUPN, req.Category, req.CanonicalKey, deleted)
	return deleted, nil
}

func (s *Service) buildAssistantContext(ctx context.Context, assistantKey, userUPN, sessionID string) (string, error) {
	memories, err := s.repo.ListMemories(ctx, strings.TrimSpace(assistantKey), strings.TrimSpace(userUPN), s.memoryLimit)
	if err != nil {
		return "", err
	}
	recentConversation := "无"
	if strings.TrimSpace(sessionID) != "" {
		messages, msgErr := s.repo.ListRecentMessages(ctx, strings.TrimSpace(sessionID), recentConversationContextLimit)
		if msgErr != nil {
			return "", msgErr
		}
		recentConversation = formatRecentConversation(messages)
	}
	return strings.TrimSpace(fmt.Sprintf(
		"当前登录用户 UPN：%s\n当前顶层助手：%s\n长期记忆：%s\n近期会话：%s",
		strings.TrimSpace(userUPN),
		strings.TrimSpace(assistantKey),
		joinMemories(memories),
		recentConversation,
	)), nil
}

const (
	recentConversationContextLimit    = 12
	recentConversationMessageMaxRunes = 280
)

func formatRecentConversation(messages []MessageRecord) string {
	if len(messages) == 0 {
		return "无"
	}
	lines := make([]string, 0, len(messages))
	for _, message := range messages {
		role := strings.TrimSpace(message.Role)
		if role == "" {
			continue
		}
		content := shortenContextText(message.Content, recentConversationMessageMaxRunes)
		if content == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %s", role, content))
	}
	if len(lines) == 0 {
		return "无"
	}
	return strings.Join(lines, "\n")
}

func shortenContextText(content string, limit int) string {
	content = strings.TrimSpace(strings.ReplaceAll(content, "\n", " "))
	if content == "" {
		return ""
	}
	if limit <= 0 {
		return content
	}
	runes := []rune(content)
	if len(runes) <= limit {
		return content
	}
	return string(runes[:limit]) + "..."
}

func (s *Service) buildResumeTargets(targets map[string]*itsmv1.ResumeTarget) (map[string]any, error) {
	resumeTargets := make(map[string]any, len(targets))
	for interruptID, target := range targets {
		if strings.TrimSpace(interruptID) == "" || target == nil {
			continue
		}
		data, err := legacyitsm.BuildResumeData(target)
		if err != nil {
			return nil, err
		}
		resumeTargets[interruptID] = data
	}
	if len(resumeTargets) == 0 {
		return nil, fmt.Errorf("targets has no valid resume payload")
	}
	return resumeTargets, nil
}

func (s *Service) persistSessionAndResponse(ctx context.Context, assistantKey, userUPN, language string, createdAt time.Time, resp *Response) error {
	if resp == nil {
		return nil
	}
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	if err := s.repo.SaveSession(ctx, SessionRecord{
		AssistantKey:     strings.TrimSpace(assistantKey),
		SessionID:        resp.SessionID,
		UserUPN:          strings.TrimSpace(userUPN),
		ActivePathJSON:   marshalPath(resp.ActivePath),
		ActiveCheckpoint: resp.CheckpointID,
		Status:           sessionStatusFromResponse(resp),
		Language:         language,
		CreatedAt:        createdAt,
		UpdatedAt:        time.Now(),
	}); err != nil {
		return err
	}
	return s.persistAssistantResponse(ctx, assistantKey, userUPN, language, resp)
}

func (s *Service) persistAssistantResponse(ctx context.Context, assistantKey, userUPN, language string, resp *Response) error {
	content, payload := responseToVisibleMessage(resp)
	_, err := s.repo.AppendMessage(ctx, MessageRecord{
		AssistantKey:   strings.TrimSpace(assistantKey),
		SessionID:      resp.SessionID,
		UserUPN:        strings.TrimSpace(userUPN),
		Role:           "assistant",
		Content:        content,
		PayloadJSON:    payload,
		ActivePathJSON: marshalPath(resp.ActivePath),
		CheckpointID:   resp.CheckpointID,
		Language:       language,
		CreatedAt:      time.Now(),
	})
	return err
}

func (s *Service) enqueueMemoryJob(assistantKey, sessionID, userUPN, language string) {
	select {
	case s.memoryJobs <- MemoryJob{AssistantKey: assistantKey, SessionID: sessionID, UserUPN: userUPN, Language: language}:
	default:
		g.Log().Warningf(context.Background(), "agent memory queue full, skip job, assistant_key=%s session_id=%s", assistantKey, sessionID)
	}
}

func (s *Service) invalidateCheckpoint(ctx context.Context, assistantKey, checkpointID string) {
	if s == nil || s.registry == nil || strings.TrimSpace(checkpointID) == "" {
		return
	}
	bundle, ok := s.registry.bundles[strings.TrimSpace(assistantKey)]
	if !ok || bundle.CheckpointStore == nil {
		return
	}
	if err := bundle.CheckpointStore.Delete(ctx, checkpointID); err != nil {
		g.Log().Warningf(ctx, "agent delete checkpoint failed, assistant_key=%s checkpoint_id=%s err=%v", assistantKey, checkpointID, err)
	}
}

func (s *Service) errorResponse(assistantKey, sessionID string, activePath []string, message string) *Response {
	return &Response{
		AssistantKey: strings.TrimSpace(assistantKey),
		Status:       "error",
		SessionID:    sessionID,
		ActivePath:   pathOrDefault(activePath, assistantKey),
		Result: &Result{
			Success: false,
			Message: message,
		},
	}
}

func (s *Service) validateCreateRunRequest(req *CreateRunRequest) error {
	if req == nil {
		return gerror.NewCode(gcode.CodeMissingParameter, "request is required")
	}
	if strings.TrimSpace(req.AssistantKey) == "" {
		return gerror.NewCode(gcode.CodeMissingParameter, "assistant_key is required")
	}
	if strings.TrimSpace(req.UserUPN) == "" {
		return gerror.NewCode(gcode.CodeMissingParameter, "missing header X-User-ID")
	}
	if strings.TrimSpace(req.Message) == "" {
		return gerror.NewCode(gcode.CodeMissingParameter, "message is required")
	}
	return nil
}

func (s *Service) validateResumeRunRequest(req *ResumeRunRequest) error {
	if req == nil {
		return gerror.NewCode(gcode.CodeMissingParameter, "request is required")
	}
	if strings.TrimSpace(req.AssistantKey) == "" {
		return gerror.NewCode(gcode.CodeMissingParameter, "assistant_key is required")
	}
	if strings.TrimSpace(req.RunID) == "" {
		return gerror.NewCode(gcode.CodeMissingParameter, "run_id is required")
	}
	if strings.TrimSpace(req.UserUPN) == "" {
		return gerror.NewCode(gcode.CodeMissingParameter, "missing header X-User-ID")
	}
	if len(req.Targets) == 0 {
		return gerror.NewCode(gcode.CodeMissingParameter, "targets is required")
	}
	return nil
}
