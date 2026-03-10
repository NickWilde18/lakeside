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

// NewTicketCreateAgent 构造业务状态机。
// 这里不直接依赖 controller/service，而是只关心“抽取器 + 下游 ITSM 客户端 + 幂等存储”。
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
	startedAt := time.Now()
	// user_code 由 service 层在 Query/Resume 时写入 session values，这里直接读取即可。
	userCode := strings.TrimSpace(sessionString(ctx, "user_code"))
	if userCode == "" {
		return singleEventIter(&adk.AgentEvent{Err: fmt.Errorf("missing X-User-ID")})
	}

	// ADK 输入里可能包含多轮消息，实际只取最后一条 user message 作为本轮抽取材料。
	message := latestUserMessage(input)
	if strings.TrimSpace(message) == "" {
		return singleEventIter(&adk.AgentEvent{Err: fmt.Errorf("empty user message")})
	}

	// 固定提示文案目前按“中文 / 非中文”两类切换。
	lang := detectUserLanguage(message)
	draft := TicketDraft{UserCode: userCode}

	var err error
	var clarify string
	assistantContext := sessionString(ctx, "assistant_context")
	// 第一步先让 extractor 尝试从用户原话中补齐草稿。
	draft, clarify, err = a.extractor.FillDraft(ctx, draft, message, assistantContext)
	if err != nil {
		return singleEventIter(&adk.AgentEvent{Err: err})
	}
	a.debugLog(ctx, "itsm agent run extracted draft in %dms subject=%q serviceLevel=%q priority=%q", time.Since(startedAt).Milliseconds(), draft.Subject, draft.ServiceLevel, draft.Priority)

	// 第二步由服务端规则决定当前草稿是否已经足够完整，而不是直接信任模型判断。
	if info, incomplete := a.needInfoInterrupt(lang, draft, clarify); incomplete {
		// 信息不全时中断，等待前端引导用户补充后再 Resume。
		st := &TicketAgentState{Stage: stageCollect, Language: lang, Draft: draft, Pending: *info}
		a.debugLog(ctx, "itsm agent run interrupted need_info in %dms", time.Since(startedAt).Milliseconds())
		return singleEventIter(adk.StatefulInterrupt(ctx, info, st))
	}

	// 槽位齐全后先二次确认，避免直接提交导致误建单。
	confirm := a.buildConfirmInterrupt(lang, draft)
	st := &TicketAgentState{Stage: stageConfirm, Language: lang, Draft: draft, Pending: *confirm}
	a.debugLog(ctx, "itsm agent run interrupted need_confirm in %dms", time.Since(startedAt).Milliseconds())
	return singleEventIter(adk.StatefulInterrupt(ctx, confirm, st))
}

func (a *TicketCreateAgent) Resume(ctx context.Context, info *adk.ResumeInfo, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	startedAt := time.Now()
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
		// collect 阶段只接受用户补充信息，目标是把草稿推进到“可确认”状态。
		resume, ok := info.ResumeData.(*ResumeCollectData)
		if !ok || strings.TrimSpace(resume.Answer) == "" {
			pending := state.Pending
			if pending.Prompt == "" {
				pending.Prompt = localizeText(state.Language, "请补充缺失信息。", "Please provide the missing information.")
			}
			state.Pending = pending
			return singleEventIter(adk.StatefulInterrupt(ctx, &state.Pending, state))
		}

		// 如果用户补充信息的语言发生切换，后续固定提示也跟着切换。
		state.Language = detectUserLanguage(resume.Answer)
		assistantContext := sessionString(ctx, "assistant_context")
		draft, clarify, err := a.extractor.FillDraft(ctx, state.Draft, resume.Answer, assistantContext)
		if err != nil {
			return singleEventIter(&adk.AgentEvent{Err: err})
		}
		state.Draft = draft
		a.debugLog(ctx, "itsm agent resume collect extracted draft in %dms subject=%q serviceLevel=%q priority=%q", time.Since(startedAt).Milliseconds(), draft.Subject, draft.ServiceLevel, draft.Priority)

		if need, incomplete := a.needInfoInterrupt(state.Language, state.Draft, clarify); incomplete {
			state.Pending = *need
			state.Stage = stageCollect
			a.debugLog(ctx, "itsm agent resume collect still need_info in %dms", time.Since(startedAt).Milliseconds())
			return singleEventIter(adk.StatefulInterrupt(ctx, need, state))
		}

		confirm := a.buildConfirmInterrupt(state.Language, state.Draft)
		state.Pending = *confirm
		state.Stage = stageConfirm
		a.debugLog(ctx, "itsm agent resume collect entered need_confirm in %dms", time.Since(startedAt).Milliseconds())
		return singleEventIter(adk.StatefulInterrupt(ctx, confirm, state))

	case stageConfirm:
		// confirm 阶段不再做字段抽取，只处理“确认 / 取消 / 微调文案”。
		resume, ok := info.ResumeData.(*ResumeConfirmData)
		if !ok {
			return singleEventIter(adk.StatefulInterrupt(ctx, &state.Pending, state))
		}

		if !resume.Confirmed {
			result := &TicketExecutionResult{
				Success: false,
				Message: localizeText(state.Language, "用户取消提交工单", "Ticket submission was canceled by the user"),
			}
			return singleEventIter(finalAssistantEvent(
				localizeText(state.Language, "已取消创建工单。", "Ticket creation has been canceled."),
				result,
			))
		}

		// 仅允许前端在确认阶段覆写白名单字段，避免关键枚举被随意改乱。
		if s := strings.TrimSpace(resume.Subject); s != "" {
			state.Draft.Subject = s
		}
		if s := strings.TrimSpace(resume.OthersDesc); s != "" {
			state.Draft.OthersDesc = s
		}

		execResult := a.submitTicket(ctx, state.Language, state.Draft)
		text := execResult.Message
		if execResult.Success {
			text = localizedTicketCreatedText(state.Language, execResult.TicketNo)
		}
		a.debugLog(ctx, "itsm agent resume confirm finished in %dms success=%v", time.Since(startedAt).Milliseconds(), execResult.Success)
		return singleEventIter(finalAssistantEvent(text, execResult))
	default:
		return singleEventIter(&adk.AgentEvent{Err: fmt.Errorf("unknown stage: %s", state.Stage)})
	}
}

// submitTicket 是真正触发下游建单的唯一入口。
// 所有确认完成后的请求都会收敛到这里，方便统一做幂等、重试和结果包装。
func (a *TicketCreateAgent) submitTicket(ctx context.Context, lang string, draft TicketDraft) *TicketExecutionResult {
	// 先查幂等缓存，避免同一 checkpoint 在重复确认时创建多张工单。
	if key := a.idempotencyKey(ctx, draft); key != "" {
		if val, ok, err := a.idempotencyStore.Get(ctx, key); err == nil && ok {
			var cached TicketExecutionResult
			if uErr := json.Unmarshal([]byte(val), &cached); uErr == nil {
				cached.Message = localizedResultMessage(lang, cached.Message, localizeText(lang, "命中幂等结果，返回已创建工单结果", "Matched an idempotent result and returned the existing ticket result"))
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
		return &TicketExecutionResult{
			Success: false,
			Message: localizeText(lang, fmt.Sprintf("调用工单系统失败：%v", err), fmt.Sprintf("Failed to call the ticket system: %v", err)),
		}
	}

	exec := &TicketExecutionResult{
		Success:  result.Success,
		TicketNo: result.TicketNo,
		Message:  localizedResultMessage(lang, result.Message, localizeText(lang, "工单创建成功", "Ticket created successfully")),
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

// needInfoInterrupt 根据当前草稿判断是否还需要继续追问用户。
// 这是“工单是否完整”的最终裁决点。
func (a *TicketCreateAgent) needInfoInterrupt(lang string, draft TicketDraft, clarify string) (*TicketInterruptInfo, bool) {
	internalMissing := make([]string, 0, 4)

	// 这里是“完整性”的最终裁决点。模型只能给建议，缺不缺字段由这里决定。
	if strings.TrimSpace(draft.Subject) == "" {
		internalMissing = append(internalMissing, "subject")
	}
	if strings.TrimSpace(draft.OthersDesc) == "" {
		internalMissing = append(internalMissing, "othersDesc")
	}
	if !validEnumValue(draft.ServiceLevel) {
		internalMissing = append(internalMissing, "serviceLevel")
	} else if draft.ServiceLevelConfidence > 0 && draft.ServiceLevelConfidence < a.enumThreshold {
		internalMissing = append(internalMissing, "serviceLevel")
	}
	if !validEnumValue(draft.Priority) {
		internalMissing = append(internalMissing, "priority")
	} else if draft.PriorityConfidence > 0 && draft.PriorityConfidence < a.enumThreshold {
		internalMissing = append(internalMissing, "priority")
	}

	if len(internalMissing) == 0 {
		return nil, false
	}

	internalMissing = uniqueStrings(internalMissing)
	visibleMissing, enumDecisionPending := userVisibleMissingFields(internalMissing)
	prompt := localizedNeedInfoPrompt(lang, visibleMissing, clarify, enumDecisionPending)
	info := &TicketInterruptInfo{
		Type:          statusNeedInfo,
		Prompt:        prompt,
		Language:      lang,
		MissingFields: visibleMissing,
		Draft:         draft,
	}
	return info, true
}

// buildConfirmInterrupt 在信息齐全后生成确认阶段中断。
// 当前只允许前端修改 subject 和 othersDesc，避免改坏关键枚举。
func (a *TicketCreateAgent) buildConfirmInterrupt(lang string, draft TicketDraft) *TicketInterruptInfo {
	return &TicketInterruptInfo{
		Type:           statusNeedConfirm,
		Prompt:         localizeText(lang, "请确认工单信息。你可以编辑 subject 和 othersDesc，确认后将正式提交。", "Please confirm the ticket information. You may edit subject and othersDesc before submission."),
		Language:       lang,
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

func missingFieldLabels(lang string, fields []string) []string {
	labels := make([]string, 0, len(fields))
	for _, field := range fields {
		switch field {
		case "subject":
			labels = append(labels, localizeText(lang, "主题", "subject"))
		case "othersDesc":
			labels = append(labels, localizeText(lang, "问题描述", "description"))
		default:
			labels = append(labels, field)
		}
	}
	return labels
}

func userVisibleMissingFields(internalMissing []string) ([]string, bool) {
	visible := make([]string, 0, len(internalMissing))
	enumDecisionPending := false
	for _, field := range internalMissing {
		switch field {
		case "serviceLevel", "priority":
			enumDecisionPending = true
		default:
			visible = append(visible, field)
		}
	}
	if enumDecisionPending {
		// serviceLevel/priority 由系统决定，不要求用户直接填写。
		// 当这两个枚举不确定时，统一向用户追问更具体的现象描述，便于模型继续判断。
		visible = append(visible, "othersDesc")
	}
	return uniqueStrings(visible), enumDecisionPending
}

func localizedNeedInfoPrompt(lang string, missing []string, clarify string, enumDecisionPending bool) string {
	var prompt string
	if len(missing) > 0 {
		labels := strings.Join(missingFieldLabels(lang, missing), localizeText(lang, "、", ", "))
		prompt = localizeText(lang, "信息还不完整，请补充：", "The information is incomplete. Please provide: ") + labels
	} else {
		prompt = localizeText(lang, "信息还不完整，请补充更多信息。", "The information is incomplete. Please provide more details.")
	}
	if extra := strings.TrimSpace(clarify); extra != "" {
		prompt += localizeText(lang, "。补充说明：", ". Additional details: ") + extra
	} else if enumDecisionPending {
		prompt += localizeText(lang, "。请尽量补充更具体的地点、故障现象和影响范围，系统会据此判断服务级别和工单类型。", ". Please provide more specific details about the location, symptoms, and impact scope so the system can determine the service level and ticket type.")
	}
	return prompt
}

// localizedTicketCreatedText 用于最终 assistant message，和 result.message 分开处理，
// 这样既能保留结构化返回，又能给用户一条简洁直观的自然语言结果。
func localizedTicketCreatedText(lang, ticketNo string) string {
	if strings.TrimSpace(ticketNo) == "" {
		return localizeText(lang, "工单创建成功。", "Ticket created successfully.")
	}
	if isChineseLanguage(lang) {
		return fmt.Sprintf("工单创建成功，单号：%s", ticketNo)
	}
	return fmt.Sprintf("Ticket created successfully. Ticket No: %s", ticketNo)
}

func localizedResultMessage(lang, primary, fallback string) string {
	if isChineseLanguage(lang) {
		return chooseMessage(primary, fallback)
	}
	return fallback
}

func localizeText(lang, zh, en string) string {
	if isChineseLanguage(lang) {
		return zh
	}
	return en
}

func isChineseLanguage(lang string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "zh")
}

// detectUserLanguage 目前只做“中文 / 非中文”两类识别。
// 这是一个工程上的折中：固定文案先保证中文和英文可用，后续再扩到更细粒度多语言。
func detectUserLanguage(text string) string {
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

func chooseMessage(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}

func (a *TicketCreateAgent) debugLog(ctx context.Context, format string, args ...any) {
	g.Log().Infof(ctx, format, args...)
}
