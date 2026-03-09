package itsmagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"lakeside/api/itsm/v1"
	"lakeside/internal/service/itsmclient"
)

type Service struct {
	runner          *adk.Runner
	cfg             serviceConfig
	checkpointStore checkpointStore
}

var (
	serviceOnce sync.Once
	serviceIns  *Service
)

func GetService(ctx context.Context) *Service {
	serviceOnce.Do(func() {
		serviceIns = newService(ctx)
	})
	return serviceIns
}

func newService(ctx context.Context) *Service {
	cfg := serviceConfig{
		EnumConfidenceThreshold: g.Cfg().MustGet(ctx, "agent.enumConfidenceThreshold", 0.75).Float64(),
		CheckpointTTL:           time.Duration(g.Cfg().MustGet(ctx, "agent.checkpoint.ttlHours", 24).Int()) * time.Hour,
		IdempotencyTTL:          time.Duration(g.Cfg().MustGet(ctx, "agent.idempotency.ttlHours", 24).Int()) * time.Hour,
		CheckpointKeyPrefix:     g.Cfg().MustGet(ctx, "agent.checkpoint.keyPrefix", "itsm:adk:checkpoint:").String(),
		IdempotencyKeyPrefix:    g.Cfg().MustGet(ctx, "agent.idempotency.keyPrefix", "itsm:adk:idempotency:").String(),
	}

	// 优先使用 Redis 持久化 checkpoint/幂等状态，失败时降级为内存实现保证可用性。
	checkpointStore, idemStore := initStores(ctx, cfg)
	itsm := itsmclient.NewClient(itsmclient.Config{
		BaseURL:     g.Cfg().MustGet(ctx, "itsm.baseURL").String(),
		AppSecret:   g.Cfg().MustGet(ctx, "itsm.appSecret").String(),
		Timeout:     time.Duration(g.Cfg().MustGet(ctx, "itsm.timeoutMs", 15000).Int()) * time.Millisecond,
		MaxAttempts: g.Cfg().MustGet(ctx, "itsm.retry.maxAttempts", 3).Int(),
		Backoffs:    parseBackoffConfig(g.Cfg().MustGet(ctx, "itsm.retry.backoffMs", []int{1000, 2000, 4000}).Ints()),
	})

	agent := NewTicketCreateAgent(NewExtractor(), itsm, idemStore, cfg)
	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: false,
		CheckPointStore: checkpointStore,
	})

	return &Service{runner: runner, cfg: cfg, checkpointStore: checkpointStore}
}

func initStores(ctx context.Context, cfg serviceConfig) (checkpointStore, idempotencyStore) {
	redisAddr := strings.TrimSpace(g.Cfg().MustGet(ctx, "agent.redis.addr").String())
	if redisAddr == "" {
		g.Log().Warning(ctx, "agent.redis.addr is empty, fallback to in-memory checkpoint/idempotency store")
		return newInMemoryCheckpointStore(), newInMemoryIdempotencyStore()
	}

	client := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: g.Cfg().MustGet(ctx, "agent.redis.password").String(),
		DB:       g.Cfg().MustGet(ctx, "agent.redis.db", 0).Int(),
	})
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		g.Log().Warningf(ctx, "redis unavailable, fallback to in-memory store: %v", err)
		return newInMemoryCheckpointStore(), newInMemoryIdempotencyStore()
	}

	ckpt := &redisCheckpointStore{
		client:    client,
		keyPrefix: cfg.CheckpointKeyPrefix,
		ttl:       cfg.CheckpointTTL,
	}
	idem := &redisIdempotencyStore{client: client}
	return ckpt, idem
}

func parseBackoffConfig(ms []int) []time.Duration {
	if len(ms) == 0 {
		return []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
	}
	out := make([]time.Duration, 0, len(ms))
	for _, item := range ms {
		if item <= 0 {
			continue
		}
		out = append(out, time.Duration(item)*time.Millisecond)
	}
	if len(out) == 0 {
		return []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
	}
	return out
}

func (s *Service) Query(ctx context.Context, req *QueryRequest) (*v1.AgentResponse, error) {
	// Query 负责开启一轮新的 agent 执行，并生成新的 checkpoint 用于后续 resume。
	if strings.TrimSpace(req.UserCode) == "" {
		g.Log().Warning(ctx, "itsm query rejected: missing X-User-ID")
		return errorResponse("", "missing header X-User-ID"), nil
	}
	if strings.TrimSpace(req.Message) == "" {
		g.Log().Warningf(ctx, "itsm query rejected: empty message, user_code=%s", req.UserCode)
		return errorResponse("", "message is empty"), nil
	}

	sessionID := "sess-" + uuid.NewString()
	// checkpoint_id 每轮 Query 唯一，用于后续 Resume 精准接续到中断点。
	checkpointID := genCheckpointID()
	g.Log().Infof(ctx, "itsm query started, user_code=%s session_id=%s checkpoint_id=%s", req.UserCode, sessionID, checkpointID)

	iter := s.runner.Query(ctx, req.Message,
		adk.WithCheckPointID(checkpointID),
		adk.WithSessionValues(g.Map{
			"session_id":    sessionID,
			"checkpoint_id": checkpointID,
			"user_code":     strings.TrimSpace(req.UserCode),
		}),
	)

	return s.consumeIterator(ctx, iter, sessionID, checkpointID), nil
}

func (s *Service) Resume(ctx context.Context, req *ResumeRequest) (*v1.AgentResponse, error) {
	// Resume 使用 query 返回的 checkpoint_id 接续到上一次中断的位置。
	if strings.TrimSpace(req.UserCode) == "" {
		g.Log().Warning(ctx, "itsm resume rejected: missing X-User-ID")
		return errorResponse("", "missing header X-User-ID"), nil
	}
	if strings.TrimSpace(req.CheckpointID) == "" {
		g.Log().Warningf(ctx, "itsm resume rejected: missing checkpoint_id, user_code=%s", req.UserCode)
		return errorResponse("", "checkpoint_id is required"), nil
	}
	if len(req.Targets) == 0 {
		g.Log().Warningf(ctx, "itsm resume rejected: empty targets, user_code=%s checkpoint_id=%s", req.UserCode, req.CheckpointID)
		return errorResponse("", "targets is required"), nil
	}

	sessionID := "sess-" + uuid.NewString()
	g.Log().Infof(ctx, "itsm resume started, user_code=%s session_id=%s checkpoint_id=%s raw_target_count=%d", req.UserCode, sessionID, req.CheckpointID, len(req.Targets))

	targets := make(g.Map, len(req.Targets))
	// 按 interrupt_id 回填对应的恢复负载，让 ADK 只恢复用户确认的中断点。
	for interruptID, target := range req.Targets {
		if strings.TrimSpace(interruptID) == "" || target == nil {
			continue
		}
		if target.Confirmed != nil {
			targets[interruptID] = &ResumeConfirmData{
				Confirmed:  *target.Confirmed,
				Subject:    target.Subject,
				OthersDesc: target.OthersDesc,
			}
			continue
		}
		targets[interruptID] = &ResumeCollectData{Answer: target.Answer}
	}

	if len(targets) == 0 {
		g.Log().Warningf(ctx, "itsm resume rejected: no valid targets, user_code=%s checkpoint_id=%s", req.UserCode, req.CheckpointID)
		return errorResponse(sessionID, "targets has no valid resume payload"), nil
	}
	g.Log().Infof(ctx, "itsm resume targets built, checkpoint_id=%s valid_target_count=%d", req.CheckpointID, len(targets))

	iter, err := s.runner.ResumeWithParams(ctx, req.CheckpointID, &adk.ResumeParams{Targets: targets},
		adk.WithSessionValues(g.Map{
			"session_id":    sessionID,
			"checkpoint_id": req.CheckpointID,
			"user_code":     strings.TrimSpace(req.UserCode),
		}),
	)
	if err != nil {
		g.Log().Errorf(ctx, "itsm resume failed to create iterator, checkpoint_id=%s err=%v", req.CheckpointID, err)
		return errorResponse(sessionID, err.Error()), nil
	}

	return s.consumeIterator(ctx, iter, sessionID, req.CheckpointID), nil
}

func (s *Service) consumeIterator(ctx context.Context, iter *adk.AsyncIterator[*adk.AgentEvent], sessionID, checkpointID string) *v1.AgentResponse {
	resp := &v1.AgentResponse{SessionID: sessionID}
	var lastMessage string
	var exec *TicketExecutionResult

	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}

		if event.Err != nil {
			g.Log().Errorf(ctx, "itsm agent event failed, session_id=%s checkpoint_id=%s err=%v", sessionID, checkpointID, event.Err)
			return errorResponse(sessionID, event.Err.Error())
		}

		if event.Output != nil {
			if event.Output.CustomizedOutput != nil {
				exec = castExecutionResult(event.Output.CustomizedOutput)
			}
			msg, _, err := adk.GetMessage(event)
			if err == nil && msg != nil && strings.TrimSpace(msg.Content) != "" {
				lastMessage = strings.TrimSpace(msg.Content)
			}
		}

		if event.Action != nil && event.Action.Interrupted != nil {
			// 一旦出现中断，立即返回前端所需的提示和草稿，不继续消费后续事件。
			interrupts, status := parseInterrupts(event.Action.Interrupted.InterruptContexts)
			g.Log().Infof(ctx, "itsm agent interrupted, session_id=%s checkpoint_id=%s status=%s interrupt_count=%d", sessionID, checkpointID, status, len(interrupts))
			resp.Status = status
			resp.CheckpointID = checkpointID
			resp.Interrupts = interrupts
			return resp
		}
	}

	if exec == nil {
		exec = &TicketExecutionResult{Success: true, Message: chooseMessage(lastMessage, "操作完成")}
	}
	resp.Status = statusDone
	resp.Result = &v1.AgentResult{
		Success:  exec.Success,
		TicketNo: exec.TicketNo,
		Message:  chooseMessage(exec.Message, lastMessage),
		Code:     exec.Code,
	}
	g.Log().Infof(ctx, "itsm agent completed, session_id=%s checkpoint_id=%s success=%v ticket_no=%s", sessionID, checkpointID, exec.Success, exec.TicketNo)
	// done/cancel 后立即失效 checkpoint，减少重放窗口；TTL 仍作为兜底清理。
	s.invalidateCheckpoint(ctx, checkpointID)
	return resp
}

func parseInterrupts(ctxs []*adk.InterruptCtx) ([]v1.AgentInterrupt, string) {
	interrupts := make([]v1.AgentInterrupt, 0, len(ctxs))
	status := statusNeedInfo
	for _, c := range ctxs {
		if c == nil {
			continue
		}
		info := castInterruptInfo(c.Info)
		if info.Type == statusNeedConfirm {
			status = statusNeedConfirm
		}
		interrupts = append(interrupts, v1.AgentInterrupt{
			InterruptID:    c.ID,
			Type:           chooseMessage(info.Type, statusNeedInfo),
			Prompt:         chooseMessage(info.Prompt, fmt.Sprintf("interrupted: %v", c.Info)),
			MissingFields:  info.MissingFields,
			EditableFields: info.EditableFields,
			ReadonlyFields: info.ReadonlyFields,
			Draft: &v1.TicketDraft{
				UserCode:     info.Draft.UserCode,
				Subject:      info.Draft.Subject,
				ServiceLevel: info.Draft.ServiceLevel,
				Priority:     info.Draft.Priority,
				OthersDesc:   info.Draft.OthersDesc,
			},
		})
	}
	if len(interrupts) == 0 {
		interrupts = append(interrupts, v1.AgentInterrupt{
			Type:   statusNeedInfo,
			Prompt: "流程被中断，请继续补充信息。",
		})
	}
	return interrupts, status
}

func castInterruptInfo(v any) *TicketInterruptInfo {
	if v == nil {
		return &TicketInterruptInfo{}
	}
	if info, ok := v.(*TicketInterruptInfo); ok {
		return info
	}
	if info, ok := v.(TicketInterruptInfo); ok {
		return &info
	}
	b, err := json.Marshal(v)
	if err != nil {
		return &TicketInterruptInfo{Prompt: fmt.Sprintf("%v", v)}
	}
	var info TicketInterruptInfo
	if err := json.Unmarshal(b, &info); err != nil {
		return &TicketInterruptInfo{Prompt: fmt.Sprintf("%v", v)}
	}
	return &info
}

func castExecutionResult(v any) *TicketExecutionResult {
	if v == nil {
		return nil
	}
	if res, ok := v.(*TicketExecutionResult); ok {
		return res
	}
	if res, ok := v.(TicketExecutionResult); ok {
		return &res
	}
	b, err := json.Marshal(v)
	if err != nil {
		return &TicketExecutionResult{Success: false, Message: fmt.Sprintf("%v", v)}
	}
	var out TicketExecutionResult
	if err := json.Unmarshal(b, &out); err != nil {
		return &TicketExecutionResult{Success: false, Message: fmt.Sprintf("%v", v)}
	}
	return &out
}

func genCheckpointID() string {
	return "ckpt-" + uuid.NewString()
}

func (s *Service) invalidateCheckpoint(ctx context.Context, checkpointID string) {
	if s == nil || s.checkpointStore == nil || strings.TrimSpace(checkpointID) == "" {
		return
	}
	if err := s.checkpointStore.Delete(ctx, checkpointID); err != nil {
		g.Log().Warningf(ctx, "delete checkpoint failed, checkpoint_id=%s err=%v", checkpointID, err)
	}
}

func errorResponse(sessionID, message string) *v1.AgentResponse {
	if strings.TrimSpace(sessionID) == "" {
		sessionID = "sess-" + uuid.NewString()
	}
	return &v1.AgentResponse{
		Status:    statusError,
		SessionID: sessionID,
		Result: &v1.AgentResult{
			Success: false,
			Message: chooseMessage(message, "internal error"),
		},
	}
}
