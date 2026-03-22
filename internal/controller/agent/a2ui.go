package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	itsmv1 "lakeside/api/itsm/v1"
	"lakeside/internal/service/agentplatform"
)

const (
	a2uiSurfaceID = "agent-canvas"
)

type a2uiMessage struct {
	BeginRendering *a2uiBeginRendering `json:"beginRendering,omitempty"`
	SurfaceUpdate  *a2uiSurfaceUpdate  `json:"surfaceUpdate,omitempty"`
	DataModel      *a2uiDataModel      `json:"dataModelUpdate,omitempty"`
	DeleteSurface  *a2uiDeleteSurface  `json:"deleteSurface,omitempty"`
}

type a2uiBeginRendering struct {
	SurfaceID string            `json:"surfaceId"`
	Root      string            `json:"root"`
	Styles    map[string]string `json:"styles,omitempty"`
}

type a2uiSurfaceUpdate struct {
	SurfaceID  string          `json:"surfaceId"`
	Components []a2uiComponent `json:"components"`
}

type a2uiDataModel struct {
	SurfaceID string            `json:"surfaceId"`
	Path      string            `json:"path,omitempty"`
	Contents  []a2uiDataContent `json:"contents"`
}

type a2uiDataContent struct {
	Key          string            `json:"key"`
	ValueString  string            `json:"valueString,omitempty"`
	ValueBoolean *bool             `json:"valueBoolean,omitempty"`
	ValueNumber  *float64          `json:"valueNumber,omitempty"`
	ValueMap     []a2uiDataContent `json:"valueMap,omitempty"`
}

type a2uiDeleteSurface struct {
	SurfaceID string `json:"surfaceId"`
}

type a2uiComponent struct {
	ID        string         `json:"id"`
	Weight    *float64       `json:"weight,omitempty"`
	Component map[string]any `json:"component"`
}

type agentRuntimeOverlay struct {
	SessionID        string
	CurrentRunID     string
	DraftContent     string
	DraftStatus      string
	PendingInterrupt *itsmv1.AgentInterrupt
	TraceEvents      []agentplatform.RunEventRecord
}

type agentPageState struct {
	AssistantKey         string
	AssistantTitle       string
	AssistantDescription string
	BackendEnabled       bool
	SessionID            string
	CurrentRunID         string
	RunStatus            string
	Messages             []agentPageMessage
	PendingInterrupt     *itsmv1.AgentInterrupt
	ComposerEnabled      bool
	EmptyTitle           string
	EmptyBody            string
	DraftContent         string
	DraftSubject         string
	DraftOthersDesc      string
}

type agentPageMessage struct {
	ID      string
	Role    string
	Content string
	Status  string
	Sources []agentplatform.Source
}

type agentAssistantMeta struct {
	Title          string
	Description    string
	BackendEnabled bool
}

func assistantMeta(assistantKey string) agentAssistantMeta {
	switch strings.TrimSpace(assistantKey) {
	case "campus":
		return agentAssistantMeta{
			Title:          "校园助理",
			Description:    "回答校园知识问题，并在需要时协助发起 ITSM 工单流程。",
			BackendEnabled: true,
		}
	case "mail":
		return agentAssistantMeta{
			Title:          "邮件助理",
			Description:    "邮件助理界面已预留，当前后端尚未接入。",
			BackendEnabled: false,
		}
	case "coding":
		return agentAssistantMeta{
			Title:          "编程助理",
			Description:    "编程助理界面已预留，当前后端尚未接入。",
			BackendEnabled: false,
		}
	default:
		return agentAssistantMeta{
			Title:          strings.TrimSpace(assistantKey),
			Description:    "智能体界面",
			BackendEnabled: false,
		}
	}
}

func writeA2UIMessage(w io.Writer, msg a2uiMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

func writeA2UIRender(w io.Writer, state agentPageState) error {
	for _, msg := range buildA2UIRenderMessages(state) {
		if err := writeA2UIMessage(w, msg); err != nil {
			return err
		}
	}
	return nil
}

func buildA2UIRenderMessages(state agentPageState) []a2uiMessage {
	components, data := buildAgentPageSurface(state)
	return []a2uiMessage{
		{DeleteSurface: &a2uiDeleteSurface{SurfaceID: a2uiSurfaceID}},
		{SurfaceUpdate: &a2uiSurfaceUpdate{SurfaceID: a2uiSurfaceID, Components: components}},
		{DataModel: &a2uiDataModel{SurfaceID: a2uiSurfaceID, Contents: data}},
		{BeginRendering: &a2uiBeginRendering{SurfaceID: a2uiSurfaceID, Root: "chat-root"}},
	}
}

func buildAgentPageState(ctx context.Context, svc *agentplatform.Service, assistantKey, userUPN, sessionID string, traceEnabled bool, runtime *agentRuntimeOverlay) (agentPageState, error) {
	meta := assistantMeta(assistantKey)
	state := agentPageState{
		AssistantKey:         strings.TrimSpace(assistantKey),
		AssistantTitle:       meta.Title,
		AssistantDescription: meta.Description,
		BackendEnabled:       meta.BackendEnabled,
		SessionID:            strings.TrimSpace(sessionID),
		ComposerEnabled:      meta.BackendEnabled,
		EmptyTitle:           "开始新对话",
		EmptyBody:            "发送第一条消息后，会自动创建新的会话。",
	}
	if svc == nil {
		return state, nil
	}
	if !state.BackendEnabled {
		state.ComposerEnabled = false
		state.EmptyTitle = meta.Title
		state.EmptyBody = meta.Description
		return state, nil
	}
	if state.SessionID == "" {
		if runtime != nil && strings.TrimSpace(runtime.SessionID) != "" {
			state.SessionID = strings.TrimSpace(runtime.SessionID)
		}
	}
	if state.SessionID == "" {
		if runtime != nil {
			state.CurrentRunID = strings.TrimSpace(runtime.CurrentRunID)
			state.RunStatus = strings.TrimSpace(runtime.DraftStatus)
			state.DraftContent = runtime.DraftContent
			state.PendingInterrupt = runtime.PendingInterrupt
		}
		if isComposerBlockedStatus(state.RunStatus) {
			state.ComposerEnabled = false
		}
		return state, nil
	}
	detail, err := svc.GetSessionDetail(ctx, &agentplatform.GetSessionRequest{
		AssistantKey: assistantKey,
		SessionID:    state.SessionID,
		UserUPN:      userUPN,
	})
	if err != nil {
		return state, err
	}
	messages, latestSnapshot := buildPageMessages(detail)
	state.Messages = messages
	if latestSnapshot != nil {
		state.CurrentRunID = latestSnapshot.RunID
		state.RunStatus = latestSnapshot.RunStatus
		if latestSnapshot.RunStatus == "waiting_input" && len(latestSnapshot.Interrupts) > 0 {
			interrupt := latestSnapshot.Interrupts[0]
			state.PendingInterrupt = &interrupt
			if interrupt.Draft != nil {
				state.DraftSubject = strings.TrimSpace(interrupt.Draft.Subject)
				state.DraftOthersDesc = strings.TrimSpace(interrupt.Draft.OthersDesc)
			}
		}
	}
	if runtime != nil {
		if strings.TrimSpace(runtime.SessionID) != "" {
			state.SessionID = strings.TrimSpace(runtime.SessionID)
		}
		if strings.TrimSpace(runtime.CurrentRunID) != "" {
			state.CurrentRunID = strings.TrimSpace(runtime.CurrentRunID)
		}
		if strings.TrimSpace(runtime.DraftStatus) != "" {
			state.RunStatus = strings.TrimSpace(runtime.DraftStatus)
		}
		if runtime.PendingInterrupt != nil {
			state.PendingInterrupt = runtime.PendingInterrupt
		}
		if runtime.DraftContent != "" || state.RunStatus == "running" || state.RunStatus == "queued" {
			state.DraftContent = runtime.DraftContent
			state.Messages = append(state.Messages, agentPageMessage{
				ID:      "runtime-draft",
				Role:    "assistant",
				Content: runtime.DraftContent,
				Status:  chooseRuntimeStatus(runtime.DraftStatus),
			})
		}
	}
	if state.PendingInterrupt != nil || isComposerBlockedStatus(state.RunStatus) {
		state.ComposerEnabled = false
	}
	return state, nil
}

func chooseRuntimeStatus(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return "running"
	}
	return status
}

func isComposerBlockedStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "queued", "running", "waiting_input":
		return true
	default:
		return false
	}
}

func buildPageMessages(detail *agentplatform.SessionDetail) ([]agentPageMessage, *agentplatform.RunSnapshot) {
	if detail == nil {
		return nil, nil
	}
	assistantRunIndex := 0
	messages := make([]agentPageMessage, 0, len(detail.Messages))
	var latestSnapshot *agentplatform.RunSnapshot
	for _, trace := range detail.Runs {
		if trace.Snapshot != nil {
			latestSnapshot = trace.Snapshot
		}
	}
	for _, message := range detail.Messages {
		item := agentPageMessage{
			ID:      fmt.Sprintf("msg-%d", message.ID),
			Role:    strings.TrimSpace(message.Role),
			Content: strings.TrimSpace(message.Content),
			Status:  "done",
		}
		if item.Role == "assistant" && assistantRunIndex < len(detail.Runs) {
			trace := detail.Runs[assistantRunIndex]
			assistantRunIndex++
			if trace.Snapshot != nil {
				item.Status = strings.TrimSpace(trace.Snapshot.RunStatus)
				item.Sources = snapshotSources(trace.Snapshot)
			}
		}
		messages = append(messages, item)
	}
	if latestSnapshot != nil && (latestSnapshot.RunStatus == "running" || latestSnapshot.RunStatus == "queued") {
		userCount := 0
		assistantCount := 0
		for _, message := range detail.Messages {
			if strings.TrimSpace(message.Role) == "assistant" {
				assistantCount++
			} else if strings.TrimSpace(message.Role) == "user" {
				userCount++
			}
		}
		if userCount > assistantCount {
			messages = append(messages, agentPageMessage{
				ID:     "latest-running",
				Role:   "assistant",
				Status: latestSnapshot.RunStatus,
			})
		}
	}
	return messages, latestSnapshot
}

func snapshotSources(snapshot *agentplatform.RunSnapshot) []agentplatform.Source {
	if snapshot == nil {
		return nil
	}
	merged := append([]agentplatform.Source(nil), snapshotResultSources(snapshot)...)
	for _, step := range snapshot.Steps {
		merged = mergeDisplaySources(merged, step.Sources)
	}
	return merged
}

func mergeDisplaySources(base, incoming []agentplatform.Source) []agentplatform.Source {
	if len(incoming) == 0 {
		return base
	}
	seen := make(map[string]struct{}, len(base)+len(incoming))
	out := make([]agentplatform.Source, 0, len(base)+len(incoming))
	for _, source := range base {
		key := source.KBID + ":" + source.NodeID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, source)
	}
	for _, source := range incoming {
		key := source.KBID + ":" + source.NodeID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, source)
	}
	return out
}

func flattenTraceEvents(traces []agentplatform.RunTrace) []agentplatform.RunEventRecord {
	if len(traces) == 0 {
		return nil
	}
	items := make([]agentplatform.RunEventRecord, 0)
	for _, trace := range traces {
		items = append(items, trace.Events...)
	}
	return items
}

func formatTraceLines(events []agentplatform.RunEventRecord) []string {
	if len(events) == 0 {
		return nil
	}
	items := make([]string, 0, len(events))
	for _, event := range events {
		text := strings.TrimSpace(event.Message)
		if text == "" {
			text = strings.TrimSpace(event.EventType)
		} else {
			text = strings.TrimSpace(event.EventType) + " · " + text
		}
		if !event.CreatedAt.IsZero() {
			text = event.CreatedAt.Format("15:04:05") + " · " + text
		}
		items = append(items, text)
	}
	return items
}

func buildAgentPageSurface(state agentPageState) ([]a2uiComponent, []a2uiDataContent) {
	components := make([]a2uiComponent, 0, 256)
	data := []a2uiDataContent{
		mapContent("meta",
			stringContent("assistantKey", state.AssistantKey),
			stringContent("sessionId", state.SessionID),
			stringContent("currentRunId", state.CurrentRunID),
		),
		mapContent("composer",
			stringContent("message", ""),
		),
		mapContent("runtime",
			stringContent("draftContent", state.DraftContent),
		),
		mapContent("interrupt",
			stringContent("runId", state.CurrentRunID),
			stringContent("interruptId", interruptID(state.PendingInterrupt)),
			stringContent("answer", ""),
			stringContent("subject", state.DraftSubject),
			stringContent("othersDesc", state.DraftOthersDesc),
		),
	}

	rootChildren := []string{"messages-col"}
	if len(state.Messages) == 0 {
		rootChildren = append(rootChildren, "chat-empty-card")
	}
	if state.PendingInterrupt != nil {
		rootChildren = append(rootChildren, "interrupt-card")
	}
	rootChildren = append(rootChildren, "composer-card")

	components = append(components, columnComponent("chat-root", nil, rootChildren...))
	components = append(components, buildChatComponents(state)...)
	return components, data
}

func buildChatComponents(state agentPageState) []a2uiComponent {
	components := []a2uiComponent{
		cardComponent("messages-card", nil, "messages-col"),
	}
	messageChildren := []string{}
	if len(state.Messages) == 0 {
		components = append(components,
			cardComponent("chat-empty-card", nil, "chat-empty-col"),
			columnComponent("chat-empty-col", nil, "empty-title", "empty-body"),
			textComponent("empty-title", literalString(state.EmptyTitle), "h3"),
			textComponent("empty-body", literalString(state.EmptyBody), "body"),
		)
	}
	for index, message := range state.Messages {
		cardID := fmt.Sprintf("chat-card-%d", index)
		bodyID := fmt.Sprintf("chat-body-%d", index)
		metaID := fmt.Sprintf("chat-meta-%d", index)
		colID := fmt.Sprintf("chat-col-%d", index)
		children := []string{metaID, bodyID}
		components = append(components,
			cardComponent(cardID, nil, colID),
			columnComponent(colID, nil, children...),
			textComponent(metaID, literalString(renderMessageMeta(message)), "caption"),
		)
		if message.ID == "runtime-draft" {
			components = append(components, textComponent(bodyID, pathValue("/runtime/draftContent"), "body"))
		} else {
			components = append(components, textComponent(bodyID, literalString(message.Content), "body"))
		}
		if len(message.Sources) > 0 {
			sourceID := fmt.Sprintf("chat-sources-%d", index)
			children = append(children, sourceID)
			components = append(components,
				columnComponent(colID, nil, children...),
				textComponent(sourceID, literalString(renderSources(message.Sources)), "caption"),
			)
		}
		messageChildren = append(messageChildren, cardID)
	}
	if state.RunStatus == "running" || state.RunStatus == "queued" {
		messageChildren = append(messageChildren, "cancel-run-btn")
		components = append(components, buttonComponents("cancel-run-btn", "取消当前执行", nil, actionSpec("cancel_turn", map[string]any{
			"sessionId": pathValue("/meta/sessionId"),
			"runId":     pathValue("/meta/currentRunId"),
		}), "body")...)
	}
	components = append(components, columnComponent("messages-col", nil, messageChildren...))
	if state.PendingInterrupt != nil {
		components = append(components, buildInterruptComponents(state)...)
	}
	components = append(components, buildComposerComponents(state)...)
	return components
}

func buildInterruptComponents(state agentPageState) []a2uiComponent {
	interrupt := state.PendingInterrupt
	if interrupt == nil {
		return nil
	}
	children := []string{"interrupt-title", "interrupt-body"}
	components := []a2uiComponent{
		cardComponent("interrupt-card", nil, "interrupt-col"),
		textComponent("interrupt-title", literalString("待处理操作"), "h3"),
		textComponent("interrupt-body", literalString(strings.TrimSpace(interrupt.Prompt)), "body"),
	}
	if strings.TrimSpace(interrupt.Type) == "need_info" {
		children = append(children, "interrupt-answer", "interrupt-submit")
		components = append(components,
			textFieldComponent("interrupt-answer", nil, "/interrupt/answer", "补充信息", "longText"),
		)
		components = append(components, buttonComponents("interrupt-submit", "提交补充信息", nil, actionSpec("follow_up_submit", map[string]any{
			"sessionId":   pathValue("/meta/sessionId"),
			"runId":       pathValue("/interrupt/runId"),
			"interruptId": pathValue("/interrupt/interruptId"),
			"answer":      pathValue("/interrupt/answer"),
		}), "body")...)
		components = append(components, columnComponent("interrupt-col", nil, children...))
		return components
	}
	children = append(children, "interrupt-subject", "interrupt-others-desc", "interrupt-actions")
	components = append(components,
		textFieldComponent("interrupt-subject", nil, "/interrupt/subject", "工单标题", "shortText"),
		textFieldComponent("interrupt-others-desc", nil, "/interrupt/othersDesc", "故障描述", "longText"),
		rowComponent("interrupt-actions", nil, "interrupt-approve", "interrupt-reject"),
	)
	components = append(components, buttonComponents("interrupt-approve", "确认提交", weightPtr(1), actionSpec("approval_submit", map[string]any{
		"sessionId":   pathValue("/meta/sessionId"),
		"runId":       pathValue("/interrupt/runId"),
		"interruptId": pathValue("/interrupt/interruptId"),
		"approved":    literalBool(true),
		"subject":     pathValue("/interrupt/subject"),
		"othersDesc":  pathValue("/interrupt/othersDesc"),
	}), "body")...)
	components = append(components, buttonComponents("interrupt-reject", "取消流程", weightPtr(1), actionSpec("approval_submit", map[string]any{
		"sessionId":   pathValue("/meta/sessionId"),
		"runId":       pathValue("/interrupt/runId"),
		"interruptId": pathValue("/interrupt/interruptId"),
		"approved":    literalBool(false),
		"subject":     pathValue("/interrupt/subject"),
		"othersDesc":  pathValue("/interrupt/othersDesc"),
	}), "body")...)
	components = append(components, columnComponent("interrupt-col", nil, children...))
	return components
}

func buildComposerComponents(state agentPageState) []a2uiComponent {
	components := []a2uiComponent{
		cardComponent("composer-card", nil, "composer-col"),
	}
	if state.BackendEnabled && state.ComposerEnabled {
		components = append(components,
			columnComponent("composer-col", nil, "composer-title", "composer-input", "composer-send"),
			textComponent("composer-title", literalString("发送消息"), "caption"),
			textFieldComponent("composer-input", nil, "/composer/message", "输入你的问题", "longText"),
		)
		components = append(components, buttonComponents("composer-send", "发送", nil, actionSpec("send_message", map[string]any{
			"sessionId": pathValue("/meta/sessionId"),
			"message":   pathValue("/composer/message"),
		}), "body")...)
		return components
	}

	notice := "当前暂时不能继续输入。"
	if !state.BackendEnabled {
		notice = state.AssistantDescription
	} else if state.PendingInterrupt != nil {
		notice = "请先完成上方待处理操作。"
	} else if state.RunStatus == "running" || state.RunStatus == "queued" {
		notice = "当前智能体仍在处理中。"
	}
	components = append(components,
		columnComponent("composer-col", nil, "composer-title", "composer-notice"),
		textComponent("composer-title", literalString("发送消息"), "caption"),
		textComponent("composer-notice", literalString(notice), "body"),
	)
	return components
}

func renderMessageMeta(message agentPageMessage) string {
	role := strings.TrimSpace(message.Role)
	if role == "assistant" {
		role = "助理"
	} else if role == "user" {
		role = "你"
	}
	status := strings.TrimSpace(message.Status)
	if status == "" || status == "done" {
		return role
	}
	return role + " · " + status
}

func renderSources(sources []agentplatform.Source) string {
	items := make([]string, 0, len(sources))
	for _, source := range sources {
		label := strings.TrimSpace(source.Filename)
		if label == "" {
			label = strings.TrimSpace(source.DocID)
		}
		if label == "" {
			label = strings.TrimSpace(source.NodeID)
		}
		if label == "" {
			continue
		}
		items = append(items, label)
	}
	if len(items) == 0 {
		return ""
	}
	return "来源：" + strings.Join(items, "、")
}

func interruptID(interrupt *itsmv1.AgentInterrupt) string {
	if interrupt == nil {
		return ""
	}
	return strings.TrimSpace(interrupt.InterruptID)
}

func mapContent(key string, value ...a2uiDataContent) a2uiDataContent {
	return a2uiDataContent{Key: key, ValueMap: value}
}

func stringContent(key, value string) a2uiDataContent {
	return a2uiDataContent{Key: key, ValueString: value}
}

func literalString(value string) map[string]any {
	return map[string]any{"literalString": value}
}

func literalBool(value bool) map[string]any {
	return map[string]any{"literalBoolean": value}
}

func pathValue(path string) map[string]any {
	return map[string]any{"path": path}
}

func actionSpec(name string, ctx map[string]any) map[string]any {
	actionItems := make([]map[string]any, 0, len(ctx))
	if len(ctx) > 0 {
		keys := make([]string, 0, len(ctx))
		for key := range ctx {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			actionItems = append(actionItems, map[string]any{
				"key":   key,
				"value": ctx[key],
			})
		}
	}
	spec := map[string]any{"name": name}
	if len(actionItems) > 0 {
		spec["context"] = actionItems
	}
	return spec
}

func weightPtr(value float64) *float64 {
	return &value
}

func childrenRef(children ...string) map[string]any {
	return map[string]any{"explicitList": children}
}

func component(id, kind string, weight *float64, props map[string]any) a2uiComponent {
	return a2uiComponent{ID: id, Weight: weight, Component: map[string]any{kind: props}}
}

func rowComponent(id string, weight *float64, children ...string) a2uiComponent {
	return component(id, "Row", weight, map[string]any{
		"children":     childrenRef(children...),
		"alignment":    "stretch",
		"distribution": "start",
	})
}

func columnComponent(id string, weight *float64, children ...string) a2uiComponent {
	return component(id, "Column", weight, map[string]any{
		"children":     childrenRef(children...),
		"alignment":    "stretch",
		"distribution": "start",
	})
}

func cardComponent(id string, weight *float64, child string) a2uiComponent {
	return component(id, "Card", weight, map[string]any{"child": child})
}

func textComponent(id string, text map[string]any, usage string) a2uiComponent {
	return component(id, "Text", nil, map[string]any{
		"text":      text,
		"usageHint": usage,
	})
}

func buttonComponent(id, child string, weight *float64, action map[string]any) a2uiComponent {
	return component(id, "Button", weight, map[string]any{
		"child":  child,
		"action": action,
	})
}

func buttonComponents(id, label string, weight *float64, action map[string]any, usage string) []a2uiComponent {
	labelID := id + "-label"
	return []a2uiComponent{
		buttonComponent(id, labelID, weight, action),
		textComponent(labelID, literalString(label), usage),
	}
}

func textFieldComponent(id string, weight *float64, path, label, fieldType string) a2uiComponent {
	props := map[string]any{
		"text":  pathValue(path),
		"label": literalString(label),
	}
	if strings.TrimSpace(fieldType) != "" {
		props["type"] = fieldType
	}
	return component(id, "TextField", weight, props)
}

type a2uiActionEnvelope struct {
	UserAction *agentActionInput `json:"userAction,omitempty"`
	Error      map[string]any    `json:"error,omitempty"`
}

type agentActionInput struct {
	Name              string         `json:"name"`
	SurfaceID         string         `json:"surfaceId,omitempty"`
	SourceComponentID string         `json:"sourceComponentId,omitempty"`
	Timestamp         string         `json:"timestamp,omitempty"`
	Context           map[string]any `json:"context,omitempty"`
}

func parseActionString(ctx map[string]any, key string) string {
	if len(ctx) == 0 {
		return ""
	}
	value, ok := ctx[key]
	if !ok || value == nil {
		return ""
	}
	switch item := value.(type) {
	case string:
		return strings.TrimSpace(item)
	case fmt.Stringer:
		return strings.TrimSpace(item.String())
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", item))
	}
}

func parseActionBool(ctx map[string]any, key string) *bool {
	if len(ctx) == 0 {
		return nil
	}
	value, ok := ctx[key]
	if !ok || value == nil {
		return nil
	}
	switch item := value.(type) {
	case bool:
		return &item
	case string:
		normalized := strings.ToLower(strings.TrimSpace(item))
		if normalized == "true" || normalized == "1" || normalized == "yes" {
			result := true
			return &result
		}
		if normalized == "false" || normalized == "0" || normalized == "no" {
			result := false
			return &result
		}
	}
	return nil
}

func streamRuntimeDraftUpdate(w io.Writer, content string) error {
	return writeA2UIMessage(w, a2uiMessage{DataModel: &a2uiDataModel{
		SurfaceID: a2uiSurfaceID,
		Path:      "/runtime",
		Contents:  []a2uiDataContent{{Key: "draftContent", ValueString: content}},
	}})
}

func waitForRunTerminal(ctx context.Context, svc *agentplatform.Service, assistantKey, runID, userUPN string, afterID int64, onEvent func(agentplatform.RunEventRecord) error) error {
	if svc == nil {
		return nil
	}
	flushEvents := func() (int64, bool, error) {
		events, err := svc.ListRunEvents(ctx, &agentplatform.ListRunEventsRequest{
			AssistantKey: assistantKey,
			RunID:        runID,
			UserUPN:      userUPN,
			AfterID:      afterID,
		})
		if err != nil {
			return afterID, false, err
		}
		terminal := false
		for _, event := range events {
			if event.ID <= afterID {
				continue
			}
			afterID = event.ID
			if onEvent != nil {
				if err := onEvent(event); err != nil {
					return afterID, false, err
				}
			}
			if isTerminalEventType(event.EventType) {
				terminal = true
			}
		}
		return afterID, terminal, nil
	}
	stream, unsubscribeLocal := svc.SubscribeRun(runID)
	defer unsubscribeLocal()
	wakeup, unsubscribeWake := svc.SubscribeRunWake(ctx, runID)
	defer unsubscribeWake()
	pollTicker := time.NewTicker(1200 * time.Millisecond)
	defer pollTicker.Stop()
	afterID, terminal, err := flushEvents()
	if err != nil || terminal {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-stream:
			if !ok {
				return nil
			}
			if event.ID <= afterID {
				continue
			}
			afterID = event.ID
			if onEvent != nil {
				if err := onEvent(event); err != nil {
					return err
				}
			}
			if isTerminalEventType(event.EventType) {
				return nil
			}
		case <-wakeup:
			afterID, terminal, err = flushEvents()
			if err != nil || terminal {
				return err
			}
		case <-pollTicker.C:
			afterID, terminal, err = flushEvents()
			if err != nil || terminal {
				return err
			}
		}
	}
}

func isTerminalEventType(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "run_completed", "run_failed", "run_cancelled", "run_waiting_input":
		return true
	default:
		return false
	}
}

func onRuntimeEvent(runtime *agentRuntimeOverlay, w io.Writer, traceEnabled bool) func(agentplatform.RunEventRecord) error {
	return func(event agentplatform.RunEventRecord) error {
		if runtime == nil {
			return nil
		}
		runtime.TraceEvents = append(runtime.TraceEvents, event)
		switch strings.TrimSpace(event.EventType) {
		case "knowledge_answer_chunk":
			payload := parsePayload(event.PayloadJSON)
			if mapped, ok := payload.(map[string]any); ok {
				delta := parseActionString(mapped, "delta")
				runtime.DraftContent += delta
				return streamRuntimeDraftUpdate(w, runtime.DraftContent)
			}
			return nil
		case "itsm_interrupt_emitted":
			payload := parsePayload(event.PayloadJSON)
			if mapped, ok := payload.(map[string]any); ok {
				if rawInterrupts, ok := mapped["interrupts"].([]any); ok && len(rawInterrupts) > 0 {
					data, _ := json.Marshal(rawInterrupts[0])
					var interrupt itsmv1.AgentInterrupt
					if err := json.Unmarshal(data, &interrupt); err == nil {
						runtime.PendingInterrupt = &interrupt
						runtime.DraftStatus = "waiting_input"
					}
				}
			}
		default:
			if strings.TrimSpace(event.EventType) == "run_started" {
				runtime.DraftStatus = "running"
			}
		}
		if traceEnabled && strings.TrimSpace(event.EventType) != "knowledge_answer_chunk" {
			return nil
		}
		return nil
	}
}

func snapshotResultSources(snapshot *agentplatform.RunSnapshot) []agentplatform.Source {
	if snapshot == nil || snapshot.Result == nil {
		return nil
	}
	return snapshot.Result.Sources
}
