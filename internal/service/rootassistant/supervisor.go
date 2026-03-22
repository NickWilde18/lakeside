package rootassistant

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"

	"lakeside/internal/service/agentplatform/eventctx"
	"lakeside/internal/service/chatmodels"
	legacyitsm "lakeside/internal/service/itsmagent"
	"lakeside/internal/service/moduleapi"
)

const readyThreshold = 0.55

type rootExecutionPlan struct {
	ReadModules  []string `json:"read_modules,omitempty"`
	WriteModules []string `json:"write_modules,omitempty"`
}

type rootClarifyState struct {
	OriginalMessage string `json:"original_message"`
	Prompt          string `json:"prompt"`
}

type campusAgent struct {
	key         string
	description string
	instruction string
	moduleOrder []string
	modules     map[string]moduleapi.Module
	model       model.ToolCallingChatModel
}

func init() {
	schema.RegisterName[*rootClarifyState]("lakeside_root_clarify_state")
}

func New(ctx context.Context, key, description, instruction string, _ int, modules []moduleapi.Module) (adk.ResumableAgent, error) {
	orderedKeys := make([]string, 0, len(modules))
	items := make(map[string]moduleapi.Module, len(modules))
	for _, module := range modules {
		if module == nil {
			continue
		}
		name := strings.TrimSpace(module.Name(ctx))
		if name == "" {
			continue
		}
		items[name] = module
		orderedKeys = append(orderedKeys, name)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("root assistant %s has no modules", strings.TrimSpace(key))
	}
	return &campusAgent{
		key:         strings.TrimSpace(key),
		description: strings.TrimSpace(description),
		instruction: strings.TrimSpace(instruction),
		moduleOrder: orderedKeys,
		modules:     items,
		model:       chatmodels.GetChatModel(ctx),
	}, nil
}

func (a *campusAgent) Name(_ context.Context) string {
	return a.key
}

func (a *campusAgent) Description(_ context.Context) string {
	return a.description
}

func (a *campusAgent) GetType() string {
	return "CampusRegistryRoot"
}

func (a *campusAgent) Run(ctx context.Context, input *adk.AgentInput, opts ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	message := latestUserMessage(ctx, input)
	if strings.TrimSpace(message) == "" {
		return a.interruptForClarify(ctx, "请先说明你想咨询的校园问题或希望我帮你执行的操作。", message)
	}
	assessments := a.assessModules(ctx, message)
	plan, clarify := a.chooseRoute(assessments)
	if clarify != nil {
		eventctx.EmitForNode(ctx, "campus_clarify_needed", a.key, clarify.FollowUpPrompt, g.Map{
			"module": clarify.ModuleKey,
			"reason": clarify.Reason,
		})
		return a.interruptForClarify(ctx, clarify.FollowUpPrompt, message)
	}
	if plan.isEmpty() {
		return a.runFallback(ctx, message)
	}
	a.storePlan(ctx, plan)
	eventctx.EmitForNode(ctx, "campus_plan_ready", a.key, "顶层模块路由已生成", g.Map{
		"read_modules":  append([]string(nil), plan.ReadModules...),
		"write_modules": append([]string(nil), plan.WriteModules...),
	})
	return a.runWithPlan(ctx, input, plan, opts...)
}

func (a *campusAgent) Resume(ctx context.Context, info *adk.ResumeInfo, opts ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	if state, ok := info.InterruptState.(*rootClarifyState); ok && state != nil {
		return a.resumeClarify(ctx, info, state, opts...)
	}
	plan, err := a.loadPlan(ctx)
	if err != nil {
		return singleErrorIter(err)
	}
	workflow, err := a.buildWorkflow(ctx, plan)
	if err != nil {
		return singleErrorIter(err)
	}
	return workflow.Resume(ctx, info, opts...)
}

func (a *campusAgent) assessModules(ctx context.Context, userMessage string) []moduleapi.Assessment {
	if a == nil {
		return nil
	}
	results := make([]moduleapi.Assessment, 0, len(a.moduleOrder))
	resultCh := make(chan moduleapi.Assessment, len(a.moduleOrder))
	var wg sync.WaitGroup
	for _, key := range a.moduleOrder {
		module := a.modules[key]
		if module == nil {
			continue
		}
		wg.Add(1)
		go func(moduleKey string, m moduleapi.Module) {
			defer wg.Done()
			assessment, err := m.Assess(ctx, userMessage)
			if err != nil {
				g.Log().Warningf(ctx, "module assessment failed, root=%s module=%s err=%v", a.key, moduleKey, err)
				assessment = moduleapi.Assessment{
					ModuleKey: moduleKey,
					Status:    moduleapi.AssessmentReject,
					Phase:     moduleapi.PhaseRead,
					Score:     0,
					Reason:    err.Error(),
				}
			}
			assessment.ModuleKey = strings.TrimSpace(moduleKey)
			resultCh <- assessment
		}(key, module)
	}
	wg.Wait()
	close(resultCh)
	for assessment := range resultCh {
		results = append(results, assessment)
	}
	order := a.moduleOrderIndex()
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return order[results[i].ModuleKey] < order[results[j].ModuleKey]
	})
	return results
}

func (a *campusAgent) chooseRoute(assessments []moduleapi.Assessment) (rootExecutionPlan, *moduleapi.Assessment) {
	var (
		plan            rootExecutionPlan
		bestClarify     *moduleapi.Assessment
		bestClarifyScor float64
	)
	for _, assessment := range assessments {
		switch assessment.Status {
		case moduleapi.AssessmentReady:
			if assessment.Score < readyThreshold {
				continue
			}
			if assessment.Phase == moduleapi.PhaseWrite {
				plan.WriteModules = append(plan.WriteModules, assessment.ModuleKey)
			} else {
				plan.ReadModules = append(plan.ReadModules, assessment.ModuleKey)
			}
		case moduleapi.AssessmentNeedClarify:
			if strings.TrimSpace(assessment.FollowUpPrompt) == "" {
				continue
			}
			if bestClarify == nil || assessment.Score > bestClarifyScor {
				copied := assessment
				bestClarify = &copied
				bestClarifyScor = assessment.Score
			}
		}
	}
	plan.ReadModules = dedupeStrings(plan.ReadModules)
	plan.WriteModules = dedupeStrings(plan.WriteModules)
	if !plan.isEmpty() {
		return plan, nil
	}
	return plan, bestClarify
}

func (a *campusAgent) runWithPlan(ctx context.Context, input *adk.AgentInput, plan rootExecutionPlan, opts ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	workflow, err := a.buildWorkflow(ctx, plan)
	if err != nil {
		return singleErrorIter(err)
	}
	return workflow.Run(ctx, input, opts...)
}

func (a *campusAgent) buildWorkflow(ctx context.Context, plan rootExecutionPlan) (adk.ResumableAgent, error) {
	readStage, err := a.buildReadStage(ctx, plan.ReadModules)
	if err != nil {
		return nil, err
	}
	writeStage, err := a.buildWriteStage(ctx, plan.WriteModules)
	if err != nil {
		return nil, err
	}
	if readStage == nil && writeStage == nil {
		return nil, fmt.Errorf("campus execution plan has no modules")
	}
	if readStage != nil && writeStage == nil {
		return readStage, nil
	}
	if readStage == nil && writeStage != nil {
		return writeStage, nil
	}
	return adk.NewSequentialAgent(ctx, &adk.SequentialAgentConfig{
		Name:        a.workflowName("route"),
		Description: fmt.Sprintf("campus route workflow for %s", a.key),
		SubAgents:   []adk.Agent{readStage, writeStage},
	})
}

func (a *campusAgent) buildReadStage(ctx context.Context, moduleKeys []string) (adk.ResumableAgent, error) {
	modules, err := a.collectModules(moduleKeys)
	if err != nil || len(modules) == 0 {
		return nil, err
	}
	if len(modules) == 1 {
		return modules[0], nil
	}
	subAgents := make([]adk.Agent, 0, len(modules))
	for _, module := range modules {
		subAgents = append(subAgents, module)
	}
	return adk.NewParallelAgent(ctx, &adk.ParallelAgentConfig{
		Name:        a.workflowName("read_phase"),
		Description: fmt.Sprintf("parallel read modules for %s", a.key),
		SubAgents:   subAgents,
	})
}

func (a *campusAgent) buildWriteStage(ctx context.Context, moduleKeys []string) (adk.ResumableAgent, error) {
	modules, err := a.collectModules(moduleKeys)
	if err != nil || len(modules) == 0 {
		return nil, err
	}
	if len(modules) == 1 {
		return modules[0], nil
	}
	subAgents := make([]adk.Agent, 0, len(modules))
	for _, module := range modules {
		subAgents = append(subAgents, module)
	}
	return adk.NewSequentialAgent(ctx, &adk.SequentialAgentConfig{
		Name:        a.workflowName("write_phase"),
		Description: fmt.Sprintf("sequential write modules for %s", a.key),
		SubAgents:   subAgents,
	})
}

func (a *campusAgent) collectModules(keys []string) ([]moduleapi.Module, error) {
	modules := make([]moduleapi.Module, 0, len(keys))
	for _, key := range keys {
		module, ok := a.modules[strings.TrimSpace(key)]
		if !ok || module == nil {
			return nil, fmt.Errorf("unknown campus module: %s", key)
		}
		modules = append(modules, module)
	}
	return modules, nil
}

func (a *campusAgent) interruptForClarify(ctx context.Context, prompt, originalMessage string) *adk.AsyncIterator[*adk.AgentEvent] {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		prompt = "请补充一点信息，帮助我判断应该交给哪个校园模块处理。"
	}
	interrupt := &legacyitsm.TicketInterruptInfo{Type: "need_info", Prompt: prompt}
	return singleEventIter(adk.StatefulInterrupt(ctx, interrupt, &rootClarifyState{
		OriginalMessage: strings.TrimSpace(originalMessage),
		Prompt:          prompt,
	}))
}

func (a *campusAgent) resumeClarify(ctx context.Context, info *adk.ResumeInfo, state *rootClarifyState, opts ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	if info == nil || !info.WasInterrupted {
		return singleErrorIter(fmt.Errorf("invalid campus clarify resume context"))
	}
	if !info.IsResumeTarget {
		return a.interruptForClarify(ctx, state.Prompt, state.OriginalMessage)
	}
	resume, ok := info.ResumeData.(*legacyitsm.ResumeCollectData)
	if !ok || strings.TrimSpace(resume.Answer) == "" {
		return a.interruptForClarify(ctx, state.Prompt, state.OriginalMessage)
	}
	clarified := combineClarifiedMessage(state.OriginalMessage, resume.Answer)
	adk.AddSessionValue(ctx, "latest_user_message", clarified)
	return a.Run(ctx, &adk.AgentInput{Messages: []*schema.Message{schema.UserMessage(clarified)}}, opts...)
}

func (a *campusAgent) runFallback(ctx context.Context, userMessage string) *adk.AsyncIterator[*adk.AgentEvent] {
	reply, err := a.generateFallback(ctx, userMessage)
	if err != nil {
		return singleErrorIter(err)
	}
	eventctx.Emit(ctx, "campus_fallback_answer", []string{a.key}, reply, g.Map{"assistant": a.key})
	return singleEventIter(finalMessageEvent(a.key, reply))
}

func (a *campusAgent) generateFallback(ctx context.Context, userMessage string) (string, error) {
	userMessage = strings.TrimSpace(userMessage)
	if userMessage == "" {
		return "目前还没有足够信息来判断应该使用哪个校园模块，请补充更具体的问题。", nil
	}
	if a.model == nil {
		return fallbackText(userMessage), nil
	}
	messages := []*schema.Message{
		schema.SystemMessage(`你是 Lakeside 校园通用兜底助手。当前没有任何专用模块能高置信处理这条请求。请给出尽量有帮助的通用回答，并明确说明这是“通用回答”，不是某个专用校园模块的正式处理结果。`),
		schema.UserMessage(userMessage),
	}
	msg, err := a.model.Generate(ctx, messages)
	if err != nil {
		return fallbackText(userMessage), nil
	}
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return fallbackText(userMessage), nil
	}
	if !strings.Contains(content, "通用") && !strings.Contains(strings.ToLower(content), "general") {
		content = "以下为通用回答，暂未命中专用校园模块：\n\n" + content
	}
	return content, nil
}

func fallbackText(userMessage string) string {
	return "以下为通用回答，暂未命中专用校园模块：\n\n请补充更具体的校园场景、系统名称、故障现象或你希望执行的操作，我再继续判断。原始问题：" + strings.TrimSpace(userMessage)
}

func (a *campusAgent) storePlan(ctx context.Context, plan rootExecutionPlan) {
	data, err := json.Marshal(plan)
	if err != nil {
		g.Log().Warningf(ctx, "store campus plan failed, root=%s err=%v", a.key, err)
		return
	}
	adk.AddSessionValue(ctx, sessionPlanKey(a.key), string(data))
}

func (a *campusAgent) loadPlan(ctx context.Context) (rootExecutionPlan, error) {
	raw, ok := adk.GetSessionValue(ctx, sessionPlanKey(a.key))
	if !ok {
		return rootExecutionPlan{}, fmt.Errorf("campus plan not found")
	}
	text, ok := raw.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return rootExecutionPlan{}, fmt.Errorf("campus plan is invalid")
	}
	var plan rootExecutionPlan
	if err := json.Unmarshal([]byte(text), &plan); err != nil {
		return rootExecutionPlan{}, err
	}
	plan.ReadModules = dedupeStrings(plan.ReadModules)
	plan.WriteModules = dedupeStrings(plan.WriteModules)
	return plan, nil
}

func (a *campusAgent) moduleOrderIndex() map[string]int {
	index := make(map[string]int, len(a.moduleOrder))
	for i, key := range a.moduleOrder {
		index[key] = i
	}
	return index
}

func (a *campusAgent) workflowName(suffix string) string {
	base := strings.TrimSpace(a.key)
	if base == "" {
		base = "campus"
	}
	suffix = strings.TrimSpace(suffix)
	if suffix == "" {
		return "__" + base + "_workflow"
	}
	return "__" + base + "_" + suffix
}

func sessionPlanKey(rootKey string) string {
	return "campus_execution_plan:" + strings.TrimSpace(rootKey)
}

func latestUserMessage(ctx context.Context, input *adk.AgentInput) string {
	if value, ok := adk.GetSessionValue(ctx, "latest_user_message"); ok {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	if input == nil || len(input.Messages) == 0 {
		return ""
	}
	for i := len(input.Messages) - 1; i >= 0; i-- {
		msg := input.Messages[i]
		if msg == nil || msg.Role != schema.User {
			continue
		}
		if strings.TrimSpace(msg.Content) != "" {
			return strings.TrimSpace(msg.Content)
		}
	}
	return ""
}

func combineClarifiedMessage(originalMessage, answer string) string {
	originalMessage = strings.TrimSpace(originalMessage)
	answer = strings.TrimSpace(answer)
	if originalMessage == "" {
		return answer
	}
	if answer == "" {
		return originalMessage
	}
	return originalMessage + "\n\n补充信息：" + answer
}

func finalMessageEvent(agentName, content string) *adk.AgentEvent {
	message := schema.AssistantMessage(strings.TrimSpace(content), nil)
	return &adk.AgentEvent{
		AgentName: strings.TrimSpace(agentName),
		Output: &adk.AgentOutput{
			MessageOutput: &adk.MessageVariant{
				IsStreaming: false,
				Message:     message,
				Role:        schema.Assistant,
			},
		},
	}
}

func singleErrorIter(err error) *adk.AsyncIterator[*adk.AgentEvent] {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		gen.Send(&adk.AgentEvent{Err: err})
		gen.Close()
	}()
	return iter
}

func singleEventIter(event *adk.AgentEvent) *adk.AsyncIterator[*adk.AgentEvent] {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		gen.Send(event)
		gen.Close()
	}()
	return iter
}

func dedupeStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func (p rootExecutionPlan) isEmpty() bool {
	return len(p.ReadModules) == 0 && len(p.WriteModules) == 0
}
