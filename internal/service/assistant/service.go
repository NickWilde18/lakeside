package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	itsmv1 "lakeside/api/itsm/v1"
	"lakeside/internal/service/itsmagent"

	"github.com/cloudwego/eino/adk"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"
)

type Service struct {
	repo             Repository
	runner           *adk.Runner
	checkpointStore  checkpointStore
	memoryExtractor  *MemoryExtractor
	memoryJobs       chan MemoryJob
	memoryLimit      int
	assistantAgent   string
	itsmAgent        string
	callbackHandlers []adk.AgentRunOption
}

var (
	serviceOnce sync.Once
	serviceIns  *Service
)

func GetService(ctx context.Context) *Service {
	serviceOnce.Do(func() {
		repo, err := newRepository(ctx)
		if err != nil {
			panic(err)
		}
		checkpointStore := newCheckpointStore(ctx)
		runner, assistantAgentName, itsmAgentName, err := newRunner(ctx, checkpointStore)
		if err != nil {
			panic(err)
		}
		callbackOpt := adk.WithCallbacks(newAgentCallbackHandler()).DesignateAgent(assistantAgentName, itsmAgentName)
		serviceIns = &Service{
			repo:             repo,
			runner:           runner,
			checkpointStore:  checkpointStore,
			memoryExtractor:  NewMemoryExtractor(),
			memoryJobs:       make(chan MemoryJob, g.Cfg().MustGet(ctx, "assistant.memory.queueSize", 32).Int()),
			memoryLimit:      g.Cfg().MustGet(ctx, "assistant.memory.maxItems", 8).Int(),
			assistantAgent:   assistantAgentName,
			itsmAgent:        itsmAgentName,
			callbackHandlers: []adk.AgentRunOption{callbackOpt},
		}
		workers := g.Cfg().MustGet(ctx, "assistant.memory.workers", 1).Int()
		if workers <= 0 {
			workers = 1
		}
		for i := 0; i < workers; i++ {
			go runMemoryWorker(context.Background(), repo, serviceIns.memoryExtractor, serviceIns.memoryJobs)
		}
	})
	return serviceIns
}

func (s *Service) Query(ctx context.Context, req *QueryRequest) (*Response, error) {
	startedAt := time.Now()
	persistCtx := withoutCancel(ctx)
	if strings.TrimSpace(req.UserCode) == "" {
		return s.errorResponse("", activeAgentAssistant, "missing header X-User-ID"), nil
	}
	if strings.TrimSpace(req.Message) == "" {
		return s.errorResponse("", activeAgentAssistant, "message is empty"), nil
	}

	language := detectLanguage(req.Message)
	sessionID := "sess-" + uuid.NewString()
	checkpointID := genCheckpointID()
	createdAt := time.Now()
	assistantContext, err := s.buildAssistantContext(persistCtx, req.UserCode)
	if err != nil {
		return nil, err
	}

	if err := s.repo.SaveSession(persistCtx, SessionRecord{
		SessionID:          sessionID,
		UserCode:           strings.TrimSpace(req.UserCode),
		ActiveAgent:        activeAgentAssistant,
		ActiveCheckpointID: checkpointID,
		Status:             statusActive,
		Language:           language,
		CreatedAt:          createdAt,
		UpdatedAt:          createdAt,
	}); err != nil {
		return nil, err
	}
	if _, err := s.repo.AppendMessage(persistCtx, MessageRecord{
		SessionID:    sessionID,
		UserCode:     req.UserCode,
		Role:         "user",
		Content:      req.Message,
		ActiveAgent:  activeAgentAssistant,
		CheckpointID: checkpointID,
		Language:     language,
		CreatedAt:    time.Now(),
	}); err != nil {
		return nil, err
	}

	g.Log().Infof(ctx, "assistant query started, session_id=%s checkpoint_id=%s", sessionID, checkpointID)
	iter := s.runner.Query(ctx, req.Message, append([]adk.AgentRunOption{
		adk.WithCheckPointID(checkpointID),
		adk.WithSessionValues(g.Map{
			"session_id":         sessionID,
			"checkpoint_id":      checkpointID,
			"user_code":          strings.TrimSpace(req.UserCode),
			"assistant_context":  assistantContext,
			"preferred_language": language,
		}),
	}, s.callbackHandlers...)...)

	resp := s.consumeIterator(ctx, iter, sessionID, checkpointID, language, activeAgentAssistant)
	if err := s.persistSessionAndResponse(persistCtx, req.UserCode, language, createdAt, resp); err != nil {
		return nil, err
	}
	s.enqueueMemoryJob(sessionID, req.UserCode, language)
	g.Log().Infof(ctx, "assistant query completed, session_id=%s active_agent=%s status=%s duration_ms=%d", sessionID, resp.ActiveAgent, resp.Status, time.Since(startedAt).Milliseconds())
	return resp, nil
}

func (s *Service) Resume(ctx context.Context, req *ResumeRequest) (*Response, error) {
	startedAt := time.Now()
	persistCtx := withoutCancel(ctx)
	if strings.TrimSpace(req.UserCode) == "" {
		return s.errorResponse(req.SessionID, activeAgentAssistant, "missing header X-User-ID"), nil
	}
	if strings.TrimSpace(req.SessionID) == "" {
		return s.errorResponse("", activeAgentAssistant, "session_id is required"), nil
	}
	if strings.TrimSpace(req.CheckpointID) == "" {
		return s.errorResponse(req.SessionID, activeAgentAssistant, "checkpoint_id is required"), nil
	}
	if len(req.Targets) == 0 {
		return s.errorResponse(req.SessionID, activeAgentAssistant, "targets is required"), nil
	}

	session, err := s.repo.GetSession(persistCtx, req.SessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return s.errorResponse(req.SessionID, activeAgentAssistant, "session not found"), nil
	}
	if session.UserCode != strings.TrimSpace(req.UserCode) {
		return s.errorResponse(req.SessionID, activeAgentAssistant, "session does not belong to current user"), nil
	}
	if session.ActiveCheckpointID != strings.TrimSpace(req.CheckpointID) {
		return s.errorResponse(req.SessionID, session.ActiveAgent, "checkpoint_id does not match active session state"), nil
	}
	if session.ActiveAgent != activeAgentITSM {
		return s.errorResponse(req.SessionID, session.ActiveAgent, "active agent cannot resume"), nil
	}

	language := chooseLanguage(session.Language, targetsLanguage(req.Targets))
	assistantContext, err := s.buildAssistantContext(persistCtx, req.UserCode)
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.AppendMessage(persistCtx, MessageRecord{
		SessionID:    req.SessionID,
		UserCode:     req.UserCode,
		Role:         "user",
		Content:      summarizeTargets(req.Targets, language),
		PayloadJSON:  toJSONString(req.Targets),
		ActiveAgent:  session.ActiveAgent,
		CheckpointID: req.CheckpointID,
		Language:     language,
		CreatedAt:    time.Now(),
	}); err != nil {
		return nil, err
	}

	resumeTargets, err := s.buildResumeTargets(req.Targets)
	if err != nil {
		return s.errorResponse(req.SessionID, activeAgentITSM, err.Error()), nil
	}

	g.Log().Infof(ctx, "assistant resume started, session_id=%s checkpoint_id=%s target_count=%d", req.SessionID, req.CheckpointID, len(resumeTargets))
	iter, err := s.runner.ResumeWithParams(ctx, req.CheckpointID, &adk.ResumeParams{Targets: resumeTargets}, append([]adk.AgentRunOption{
		adk.WithSessionValues(g.Map{
			"session_id":         req.SessionID,
			"checkpoint_id":      req.CheckpointID,
			"user_code":          strings.TrimSpace(req.UserCode),
			"assistant_context":  assistantContext,
			"preferred_language": language,
		}),
	}, s.callbackHandlers...)...)
	if err != nil {
		return s.errorResponse(req.SessionID, session.ActiveAgent, err.Error()), nil
	}

	resp := s.consumeIterator(ctx, iter, req.SessionID, req.CheckpointID, language, session.ActiveAgent)
	if err := s.persistSessionAndResponse(persistCtx, req.UserCode, language, session.CreatedAt, resp); err != nil {
		return nil, err
	}
	s.enqueueMemoryJob(req.SessionID, req.UserCode, language)
	g.Log().Infof(ctx, "assistant resume completed, session_id=%s active_agent=%s status=%s duration_ms=%d", req.SessionID, resp.ActiveAgent, resp.Status, time.Since(startedAt).Milliseconds())
	return resp, nil
}

func (s *Service) ListMemories(ctx context.Context, req *ListMemoriesRequest) ([]MemoryRecord, error) {
	if req == nil || strings.TrimSpace(req.UserCode) == "" {
		return nil, fmt.Errorf("user_code is required")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	return s.repo.ListMemories(ctx, strings.TrimSpace(req.UserCode), limit)
}

func (s *Service) ClearMemories(ctx context.Context, req *ClearMemoriesRequest) (int64, error) {
	if req == nil || strings.TrimSpace(req.UserCode) == "" {
		return 0, fmt.Errorf("user_code is required")
	}
	deleted, err := s.repo.DeleteMemories(
		ctx,
		strings.TrimSpace(req.UserCode),
		strings.TrimSpace(req.Category),
		strings.TrimSpace(req.CanonicalKey),
	)
	if err != nil {
		return 0, err
	}
	g.Log().Infof(ctx, "assistant memories cleared, user_code=%s category=%s canonical_key=%s deleted_count=%d", strings.TrimSpace(req.UserCode), strings.TrimSpace(req.Category), strings.TrimSpace(req.CanonicalKey), deleted)
	return deleted, nil
}

func (s *Service) consumeIterator(ctx context.Context, iter *adk.AsyncIterator[*adk.AgentEvent], sessionID, checkpointID, language, fallbackActiveAgent string) *Response {
	resp := &Response{
		Status:      itsmagentStatusDone(),
		SessionID:   sessionID,
		ActiveAgent: activeAgentAssistant,
	}
	var (
		lastMessage string
		execResult  *itsmagent.TicketExecutionResult
		sawITSM     bool
	)

	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		if event.AgentName == s.itsmAgent {
			sawITSM = true
		}
		if event.Err != nil {
			g.Log().Errorf(ctx, "assistant agent event failed, session_id=%s checkpoint_id=%s err=%v", sessionID, checkpointID, event.Err)
			return s.errorResponse(sessionID, responseActiveAgent(sawITSM, fallbackActiveAgent, event.Err), event.Err.Error())
		}
		if event.Output != nil {
			if event.Output.CustomizedOutput != nil {
				if result := itsmagent.ExecutionResultFromAny(event.Output.CustomizedOutput); result != nil {
					execResult = result
					sawITSM = true
				}
			}
			msg, _, err := adk.GetMessage(event)
			if err == nil && msg != nil && strings.TrimSpace(msg.Content) != "" {
				lastMessage = strings.TrimSpace(msg.Content)
			}
		}
		if event.Action != nil && event.Action.Interrupted != nil {
			interrupts, status := itsmagent.APIInterruptsFromContexts(event.Action.Interrupted.InterruptContexts)
			resp.Status = status
			resp.CheckpointID = checkpointID
			resp.ActiveAgent = activeAgentITSM
			resp.Interrupts = interrupts
			return resp
		}
	}

	resp.Status = itsmagentStatusDone()
	resp.CheckpointID = ""
	resp.ActiveAgent = responseActiveAgent(sawITSM, fallbackActiveAgent, nil)
	if execResult != nil {
		resp.Result = &itsmv1.AgentResult{
			Success:  execResult.Success,
			TicketNo: execResult.TicketNo,
			Message:  chooseMessage(execResult.Message, lastMessage),
			Code:     execResult.Code,
		}
	} else {
		resp.Result = &itsmv1.AgentResult{
			Success: true,
			Message: chooseMessage(lastMessage, localizeText(language, "操作完成。", "Done.")),
		}
	}
	s.invalidateCheckpoint(ctx, checkpointID)
	return resp
}

func (s *Service) persistSessionAndResponse(ctx context.Context, userCode, language string, createdAt time.Time, resp *Response) error {
	if resp == nil {
		return nil
	}
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	if err := s.repo.SaveSession(ctx, SessionRecord{
		SessionID:          resp.SessionID,
		UserCode:           strings.TrimSpace(userCode),
		ActiveAgent:        resp.ActiveAgent,
		ActiveCheckpointID: resp.CheckpointID,
		Status:             sessionStatusFromResponse(resp),
		Language:           language,
		CreatedAt:          createdAt,
		UpdatedAt:          time.Now(),
	}); err != nil {
		return err
	}
	return s.persistAssistantResponse(ctx, resp.SessionID, userCode, language, resp)
}

func (s *Service) buildAssistantContext(ctx context.Context, userCode string) (string, error) {
	memories, err := s.repo.ListMemories(ctx, userCode, s.memoryLimit)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(fmt.Sprintf("当前用户工号：%s\n长期记忆：%s", strings.TrimSpace(userCode), joinMemories(memories))), nil
}

func (s *Service) buildResumeTargets(targets map[string]*itsmv1.ResumeTarget) (map[string]any, error) {
	resumeTargets := make(map[string]any, len(targets))
	for interruptID, target := range targets {
		if strings.TrimSpace(interruptID) == "" || target == nil {
			continue
		}
		data, err := itsmagent.BuildResumeData(target)
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

func (s *Service) errorResponse(sessionID, activeAgent, message string) *Response {
	return &Response{
		Status:      "error",
		SessionID:   sessionID,
		ActiveAgent: activeAgent,
		Result: &itsmv1.AgentResult{
			Success: false,
			Message: message,
		},
	}
}

func (s *Service) persistAssistantResponse(ctx context.Context, sessionID, userCode, language string, resp *Response) error {
	content, payload := responseToVisibleMessage(resp)
	_, err := s.repo.AppendMessage(ctx, MessageRecord{
		SessionID:    sessionID,
		UserCode:     userCode,
		Role:         "assistant",
		Content:      content,
		PayloadJSON:  payload,
		ActiveAgent:  resp.ActiveAgent,
		CheckpointID: resp.CheckpointID,
		Language:     language,
		CreatedAt:    time.Now(),
	})
	return err
}

func (s *Service) enqueueMemoryJob(sessionID, userCode, language string) {
	select {
	case s.memoryJobs <- MemoryJob{SessionID: sessionID, UserCode: userCode, Language: language}:
	default:
		g.Log().Warningf(context.Background(), "assistant memory queue full, skip job, session_id=%s", sessionID)
	}
}

func (s *Service) invalidateCheckpoint(ctx context.Context, checkpointID string) {
	if s == nil || s.checkpointStore == nil || strings.TrimSpace(checkpointID) == "" {
		return
	}
	if err := s.checkpointStore.Delete(ctx, checkpointID); err != nil {
		g.Log().Warningf(ctx, "assistant delete checkpoint failed, checkpoint_id=%s err=%v", checkpointID, err)
	}
}

func responseToVisibleMessage(resp *Response) (string, string) {
	if resp == nil {
		return "", "{}"
	}
	if len(resp.Interrupts) > 0 {
		return resp.Interrupts[0].Prompt, toJSONString(resp)
	}
	if resp.Result != nil {
		return resp.Result.Message, toJSONString(resp)
	}
	return resp.Status, toJSONString(resp)
}

func responseActiveAgent(sawITSM bool, fallback string, err error) string {
	if sawITSM {
		return activeAgentITSM
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "itsm_ticket_create_agent") {
		return activeAgentITSM
	}
	return activeAgentAssistant
}

func sessionStatusFromResponse(resp *Response) string {
	if resp == nil || resp.Status == itsmagentStatusDone() {
		return statusDone
	}
	return statusActive
}

func summarizeTargets(targets map[string]*itsmv1.ResumeTarget, language string) string {
	parts := make([]string, 0, len(targets))
	for _, target := range targets {
		if target == nil {
			continue
		}
		if strings.TrimSpace(target.Answer) != "" {
			parts = append(parts, target.Answer)
			continue
		}
		if target.Confirmed != nil {
			if *target.Confirmed {
				parts = append(parts, localizeText(language, "用户确认继续提交", "User confirmed ticket submission"))
			} else {
				parts = append(parts, localizeText(language, "用户取消提交", "User canceled ticket submission"))
			}
		}
		if strings.TrimSpace(target.Subject) != "" {
			parts = append(parts, target.Subject)
		}
		if strings.TrimSpace(target.OthersDesc) != "" {
			parts = append(parts, target.OthersDesc)
		}
	}
	return strings.Join(parts, " | ")
}

func detectLanguage(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "zh"
	}
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FFF {
			return "zh"
		}
	}
	return "en"
}

func chooseLanguage(current, candidate string) string {
	if strings.TrimSpace(candidate) != "" {
		return candidate
	}
	if strings.TrimSpace(current) != "" {
		return current
	}
	return "zh"
}

func targetsLanguage(targets map[string]*itsmv1.ResumeTarget) string {
	for _, target := range targets {
		if target == nil {
			continue
		}
		if strings.TrimSpace(target.Answer) != "" {
			return detectLanguage(target.Answer)
		}
		if strings.TrimSpace(target.Subject) != "" {
			return detectLanguage(target.Subject)
		}
		if strings.TrimSpace(target.OthersDesc) != "" {
			return detectLanguage(target.OthersDesc)
		}
	}
	return ""
}

func localizeText(language, zh, en string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "zh") {
		return zh
	}
	return en
}

func toJSONString(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func itsmagentStatusDone() string {
	return "done"
}

func genCheckpointID() string {
	return "ckpt-" + uuid.NewString()
}

func chooseMessage(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}

func withoutCancel(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return context.WithoutCancel(ctx)
}
