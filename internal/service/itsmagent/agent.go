package itsmagent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"

	"lakeside/internal/service/itsmclient"
)

type TicketCreateAgent struct {
	name              string
	description       string
	extractor         *Extractor
	itsmClient        *itsmclient.Client
	idempotencyStore  idempotencyStore
	enumThreshold     float64
	idempotencyKeyPre string
	idempotencyTTL    time.Duration
}

func NewTicketCreateAgent(extractor *Extractor, itsmClient *itsmclient.Client, idemStore idempotencyStore, cfg serviceConfig) *TicketCreateAgent {
	threshold := cfg.EnumConfidenceThreshold
	if threshold <= 0 {
		threshold = 0.75
	}
	return &TicketCreateAgent{
		name:              "itsm_ticket_create_agent",
		description:       "Extract ITSM fields, interrupt for missing info and confirmation, then create ticket",
		extractor:         extractor,
		itsmClient:        itsmClient,
		idempotencyStore:  idemStore,
		enumThreshold:     threshold,
		idempotencyKeyPre: cfg.IdempotencyKeyPrefix,
		idempotencyTTL:    cfg.IdempotencyTTL,
	}
}

func (a *TicketCreateAgent) Name(_ context.Context) string {
	return a.name
}

func (a *TicketCreateAgent) Description(_ context.Context) string {
	return a.description
}

func (a *TicketCreateAgent) Run(ctx context.Context, input *adk.AgentInput, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	userCode := strings.TrimSpace(sessionString(ctx, "user_code"))
	if userCode == "" {
		return singleEventIter(&adk.AgentEvent{Err: fmt.Errorf("missing X-User-ID")})
	}

	message := latestUserMessage(input)
	if strings.TrimSpace(message) == "" {
		return singleEventIter(&adk.AgentEvent{Err: fmt.Errorf("empty user message")})
	}

	draft := TicketDraft{UserCode: userCode}

	var err error
	var clarify string
	draft, clarify, err = a.extractor.FillDraft(ctx, draft, message)
	if err != nil {
		return singleEventIter(&adk.AgentEvent{Err: err})
	}

	if info, incomplete := a.needInfoInterrupt(draft, clarify); incomplete {
		// 信息不全时中断，等待前端引导用户补充后再 Resume。
		st := &TicketAgentState{Stage: stageCollect, Draft: draft, Pending: *info}
		return singleEventIter(adk.StatefulInterrupt(ctx, info, st))
	}

	// 槽位齐全后先二次确认，避免直接提交导致误建单。
	confirm := a.buildConfirmInterrupt(draft)
	st := &TicketAgentState{Stage: stageConfirm, Draft: draft, Pending: *confirm}
	return singleEventIter(adk.StatefulInterrupt(ctx, confirm, st))
}

func (a *TicketCreateAgent) Resume(ctx context.Context, info *adk.ResumeInfo, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	if info == nil || !info.WasInterrupted {
		return singleEventIter(&adk.AgentEvent{Err: fmt.Errorf("invalid resume context")})
	}

	state, ok := info.InterruptState.(*TicketAgentState)
	if !ok || state == nil {
		return singleEventIter(&adk.AgentEvent{Err: fmt.Errorf("invalid interrupt state type: %T", info.InterruptState)})
	}

	if !info.IsResumeTarget {
		return singleEventIter(adk.StatefulInterrupt(ctx, &state.Pending, state))
	}

	switch state.Stage {
	case stageCollect:
		resume, ok := info.ResumeData.(*ResumeCollectData)
		if !ok || strings.TrimSpace(resume.Answer) == "" {
			pending := state.Pending
			if pending.Prompt == "" {
				pending.Prompt = "请补充缺失信息。"
			}
			state.Pending = pending
			return singleEventIter(adk.StatefulInterrupt(ctx, &state.Pending, state))
		}

		draft, clarify, err := a.extractor.FillDraft(ctx, state.Draft, resume.Answer)
		if err != nil {
			return singleEventIter(&adk.AgentEvent{Err: err})
		}
		state.Draft = draft

		if need, incomplete := a.needInfoInterrupt(state.Draft, clarify); incomplete {
			state.Pending = *need
			state.Stage = stageCollect
			return singleEventIter(adk.StatefulInterrupt(ctx, need, state))
		}

		confirm := a.buildConfirmInterrupt(state.Draft)
		state.Pending = *confirm
		state.Stage = stageConfirm
		return singleEventIter(adk.StatefulInterrupt(ctx, confirm, state))

	case stageConfirm:
		resume, ok := info.ResumeData.(*ResumeConfirmData)
		if !ok {
			return singleEventIter(adk.StatefulInterrupt(ctx, &state.Pending, state))
		}

		if !resume.Confirmed {
			result := &TicketExecutionResult{Success: false, Message: "用户取消提交工单"}
			return singleEventIter(finalAssistantEvent("已取消创建工单。", result))
		}

		if s := strings.TrimSpace(resume.Subject); s != "" {
			state.Draft.Subject = s
		}
		if s := strings.TrimSpace(resume.OthersDesc); s != "" {
			state.Draft.OthersDesc = s
		}

		execResult := a.submitTicket(ctx, state.Draft)
		text := execResult.Message
		if execResult.Success {
			if execResult.TicketNo != "" {
				text = fmt.Sprintf("工单创建成功，单号：%s", execResult.TicketNo)
			} else {
				text = "工单创建成功。"
			}
		}
		return singleEventIter(finalAssistantEvent(text, execResult))
	default:
		return singleEventIter(&adk.AgentEvent{Err: fmt.Errorf("unknown stage: %s", state.Stage)})
	}
}

func (a *TicketCreateAgent) submitTicket(ctx context.Context, draft TicketDraft) *TicketExecutionResult {
	// 先查幂等缓存，避免同一 checkpoint 在重复确认时创建多张工单。
	if key := a.idempotencyKey(ctx, draft); key != "" {
		if val, ok, err := a.idempotencyStore.Get(ctx, key); err == nil && ok {
			var cached TicketExecutionResult
			if uErr := json.Unmarshal([]byte(val), &cached); uErr == nil {
				cached.Message = chooseMessage(cached.Message, "命中幂等结果，返回已创建工单结果")
				return &cached
			}
		}
	}

	result, err := a.itsmClient.CreateTicket(ctx, itsmclient.TicketPayload{
		UserCode:     draft.UserCode,
		Subject:      draft.Subject,
		ServiceLevel: draft.ServiceLevel,
		Priority:     draft.Priority,
		OthersDesc:   draft.OthersDesc,
	})
	if err != nil {
		return &TicketExecutionResult{Success: false, Message: fmt.Sprintf("调用工单系统失败：%v", err)}
	}

	exec := &TicketExecutionResult{
		Success:  result.Success,
		TicketNo: result.TicketNo,
		Message:  chooseMessage(result.Message, "工单创建成功"),
		Code:     result.Code,
	}

	if exec.Success {
		// 仅缓存成功结果，失败结果让后续重试仍有机会成功。
		if key := a.idempotencyKey(ctx, draft); key != "" {
			if payload, mErr := json.Marshal(exec); mErr == nil {
				_, _ = a.idempotencyStore.SetNX(ctx, key, string(payload), a.idempotencyTTL)
			}
		}
	}
	return exec
}

func (a *TicketCreateAgent) idempotencyKey(ctx context.Context, draft TicketDraft) string {
	checkpointID := strings.TrimSpace(sessionString(ctx, "checkpoint_id"))
	if checkpointID == "" {
		return ""
	}

	payload := map[string]string{
		"userCode":     draft.UserCode,
		"subject":      draft.Subject,
		"serviceLevel": draft.ServiceLevel,
		"priority":     draft.Priority,
		"othersDesc":   draft.OthersDesc,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(b)
	// checkpoint_id + 请求核心字段摘要，兼顾“同会话幂等”和“不同草稿可重试”。
	return a.idempotencyKeyPre + checkpointID + ":" + hex.EncodeToString(h[:])
}

func (a *TicketCreateAgent) needInfoInterrupt(draft TicketDraft, clarify string) (*TicketInterruptInfo, bool) {
	missing := make([]string, 0, 4)

	if strings.TrimSpace(draft.Subject) == "" {
		missing = append(missing, "subject")
	}
	if strings.TrimSpace(draft.OthersDesc) == "" {
		missing = append(missing, "othersDesc")
	}
	if !validEnumValue(draft.ServiceLevel) {
		missing = append(missing, "serviceLevel")
	} else if draft.ServiceLevelConfidence > 0 && draft.ServiceLevelConfidence < a.enumThreshold {
		missing = append(missing, "serviceLevel")
	}
	if !validEnumValue(draft.Priority) {
		missing = append(missing, "priority")
	} else if draft.PriorityConfidence > 0 && draft.PriorityConfidence < a.enumThreshold {
		missing = append(missing, "priority")
	}

	if len(missing) == 0 {
		return nil, false
	}

	missing = uniqueStrings(missing)
	prompt := "信息还不完整，请补充：" + strings.Join(missingFieldLabels(missing), "、")
	if extra := strings.TrimSpace(clarify); extra != "" {
		prompt = prompt + "。补充说明：" + extra
	}
	info := &TicketInterruptInfo{
		Type:          statusNeedInfo,
		Prompt:        prompt,
		MissingFields: missing,
		Draft:         draft,
	}
	return info, true
}

func (a *TicketCreateAgent) buildConfirmInterrupt(draft TicketDraft) *TicketInterruptInfo {
	return &TicketInterruptInfo{
		Type:           statusNeedConfirm,
		Prompt:         "请确认工单信息。你可以编辑 subject 和 othersDesc，确认后将正式提交。",
		EditableFields: []string{"subject", "othersDesc"},
		ReadonlyFields: []string{"userCode", "serviceLevel", "priority"},
		Draft:          draft,
	}
}

func finalAssistantEvent(text string, result *TicketExecutionResult) *adk.AgentEvent {
	msg := schema.AssistantMessage(text, nil)
	return &adk.AgentEvent{
		Output: &adk.AgentOutput{
			MessageOutput: &adk.MessageVariant{
				IsStreaming: false,
				Message:     msg,
				Role:        schema.Assistant,
			},
			CustomizedOutput: result,
		},
	}
}

func singleEventIter(event *adk.AgentEvent) *adk.AsyncIterator[*adk.AgentEvent] {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		gen.Send(event)
		gen.Close()
	}()
	return iter
}

func latestUserMessage(input *adk.AgentInput) string {
	if input == nil || len(input.Messages) == 0 {
		return ""
	}
	for i := len(input.Messages) - 1; i >= 0; i-- {
		m := input.Messages[i]
		if m == nil {
			continue
		}
		if m.Role == schema.User {
			return strings.TrimSpace(m.Content)
		}
	}
	return strings.TrimSpace(input.Messages[len(input.Messages)-1].Content)
}

func sessionString(ctx context.Context, key string) string {
	v, ok := adk.GetSessionValue(ctx, key)
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func validEnumValue(v string) bool {
	v = strings.TrimSpace(v)
	return v == "1" || v == "2" || v == "3" || v == "4"
}

func uniqueStrings(in []string) []string {
	set := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, item := range in {
		if _, ok := set[item]; ok {
			continue
		}
		set[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func missingFieldLabels(fields []string) []string {
	labels := make([]string, 0, len(fields))
	for _, field := range fields {
		switch field {
		case "subject":
			labels = append(labels, "主题")
		case "othersDesc":
			labels = append(labels, "问题描述")
		case "serviceLevel":
			labels = append(labels, "服务级别")
		case "priority":
			labels = append(labels, "工单类型")
		default:
			labels = append(labels, field)
		}
	}
	return labels
}

func chooseMessage(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}

func logDebug(ctx context.Context, format string, args ...any) {
	g.Log().Debugf(ctx, format, args...)
}
