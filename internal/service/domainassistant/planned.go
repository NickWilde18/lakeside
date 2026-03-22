package domainassistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"

	"lakeside/internal/service/agentplatform/eventctx"
	legacyitsm "lakeside/internal/service/itsmagent"
	"lakeside/internal/service/moduleapi"
)

const (
	planModeSequential = "sequential"
	planModeSupervisor = "supervisor"
)

type domainExecutionPlan struct {
	Mode  string           `json:"mode"`
	Steps []domainPlanStep `json:"steps"`
}

type domainPlanStep struct {
	AgentKey string `json:"agent_key"`
	Reason   string `json:"reason,omitempty"`
}

type moduleClarifyState struct {
	OriginalMessage string `json:"original_message"`
	Prompt          string `json:"prompt"`
}

type plannedAgent struct {
	key         string
	description string
	instruction string
	leaves      map[string]LeafBinding
	planner     *planner
}

func init() {
	schema.RegisterName[*moduleClarifyState]("lakeside_domain_clarify_state")
}

func (a *plannedAgent) Name(_ context.Context) string {
	return a.key
}

func (a *plannedAgent) Description(_ context.Context) string {
	return a.description
}

func (a *plannedAgent) GetType() string {
	return "DomainWorkflow"
}

func (a *plannedAgent) Assess(ctx context.Context, userMessage string) (moduleapi.Assessment, error) {
	if a == nil || a.planner == nil {
		return moduleapi.Assessment{}, fmt.Errorf("domain planner not initialized")
	}
	return a.planner.Assess(ctx, userMessage)
}

func (a *plannedAgent) Run(ctx context.Context, input *adk.AgentInput, opts ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	eventctx.EmitForNode(ctx, "domain_plan_started", a.key, "正在生成领域执行计划", g.Map{"domain": a.key})
	plan, err := a.planForInput(ctx, input)
	if err != nil {
		g.Log().Warningf(ctx, "domainassistant planning requires clarification or failed, domain=%s err=%v", a.key, err)
		return a.runClarifyOrError(ctx, input, err)
	}
	g.Log().Infof(ctx, "domainassistant plan ready, domain=%s mode=%s steps=%s", a.key, strings.TrimSpace(plan.Mode), planSummary(plan))
	eventctx.EmitForNode(ctx, "domain_plan_ready", a.key, "领域执行计划已生成", g.Map{
		"domain":       a.key,
		"mode":         strings.TrimSpace(plan.Mode),
		"steps":        planStepKeys(plan),
		"step_details": planStepDetails(plan),
	})
	a.storePlan(ctx, plan)
	return a.runWithPlan(ctx, nil, plan, opts...)
}

func (a *plannedAgent) Resume(ctx context.Context, info *adk.ResumeInfo, opts ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	if state, ok := info.InterruptState.(*moduleClarifyState); ok && state != nil {
		return a.resumeClarify(ctx, info, state, opts...)
	}
	eventctx.EmitForNode(ctx, "domain_plan_started", a.key, "正在加载领域执行计划", g.Map{
		"domain": a.key,
		"resume": true,
	})
	plan, err := a.loadPlan(ctx)
	if err != nil {
		return singleErrorIter(fmt.Errorf("domain execution plan not found: %w", err))
	}
	g.Log().Infof(ctx, "domainassistant resume plan loaded, domain=%s mode=%s steps=%s", a.key, strings.TrimSpace(plan.Mode), planSummary(plan))
	eventctx.EmitForNode(ctx, "domain_plan_ready", a.key, "已加载领域执行计划", g.Map{
		"domain":       a.key,
		"mode":         strings.TrimSpace(plan.Mode),
		"steps":        planStepKeys(plan),
		"step_details": planStepDetails(plan),
		"resume":       true,
	})
	seq, err := a.buildSequentialAgent(ctx, plan)
	if err != nil {
		return singleErrorIter(err)
	}
	return seq.Resume(ctx, info, opts...)
}

func (a *plannedAgent) planForInput(ctx context.Context, input *adk.AgentInput) (domainExecutionPlan, error) {
	if a == nil || a.planner == nil {
		return domainExecutionPlan{}, fmt.Errorf("planner not initialized")
	}
	message := latestUserMessage(ctx, input)
	if message == "" {
		return domainExecutionPlan{}, fmt.Errorf("latest user message is empty")
	}
	plan, err := a.planner.Plan(ctx, message)
	if err != nil {
		return domainExecutionPlan{}, err
	}
	if strings.TrimSpace(plan.Mode) == "" {
		plan.Mode = planModeSequential
	}
	if strings.EqualFold(strings.TrimSpace(plan.Mode), planModeSupervisor) || len(plan.Steps) == 0 {
		return domainExecutionPlan{}, fmt.Errorf("planner cannot derive executable plan")
	}
	return plan, nil
}

func (a *plannedAgent) runClarifyOrError(ctx context.Context, input *adk.AgentInput, planErr error) *adk.AsyncIterator[*adk.AgentEvent] {
	message := latestUserMessage(ctx, input)
	assessment, err := a.Assess(ctx, message)
	if err == nil && assessment.Status == moduleapi.AssessmentNeedClarify && strings.TrimSpace(assessment.FollowUpPrompt) != "" {
		interrupt := &legacyitsm.TicketInterruptInfo{
			Type:   "need_info",
			Prompt: strings.TrimSpace(assessment.FollowUpPrompt),
		}
		eventctx.EmitForNode(ctx, "domain_clarify_needed", a.key, interrupt.Prompt, g.Map{
			"domain": a.key,
			"reason": assessment.Reason,
		})
		return singleEventIter(adk.StatefulInterrupt(ctx, interrupt, &moduleClarifyState{
			OriginalMessage: message,
			Prompt:          interrupt.Prompt,
		}))
	}
	if err == nil && strings.Contains(strings.ToLower(strings.TrimSpace(planErr.Error())), "planner cannot derive executable plan") {
		prompt := "请再补充一点信息，说明你要咨询的具体问题，或者你希望我执行的具体操作。"
		if a != nil && a.planner != nil {
			prompt = a.planner.defaultClarifyPrompt()
		}
		interrupt := &legacyitsm.TicketInterruptInfo{
			Type:   "need_info",
			Prompt: strings.TrimSpace(prompt),
		}
		eventctx.EmitForNode(ctx, "domain_clarify_needed", a.key, interrupt.Prompt, g.Map{
			"domain": a.key,
			"reason": strings.TrimSpace(planErr.Error()),
		})
		return singleEventIter(adk.StatefulInterrupt(ctx, interrupt, &moduleClarifyState{
			OriginalMessage: message,
			Prompt:          interrupt.Prompt,
		}))
	}
	if err != nil {
		return singleErrorIter(fmt.Errorf("domain assessment failed: %w", err))
	}
	return singleErrorIter(planErr)
}

func (a *plannedAgent) resumeClarify(ctx context.Context, info *adk.ResumeInfo, state *moduleClarifyState, opts ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	if info == nil || !info.WasInterrupted {
		return singleErrorIter(fmt.Errorf("invalid clarify resume context"))
	}
	if !info.IsResumeTarget {
		interrupt := &legacyitsm.TicketInterruptInfo{Type: "need_info", Prompt: state.Prompt}
		return singleEventIter(adk.StatefulInterrupt(ctx, interrupt, state))
	}
	resume, ok := info.ResumeData.(*legacyitsm.ResumeCollectData)
	if !ok || strings.TrimSpace(resume.Answer) == "" {
		interrupt := &legacyitsm.TicketInterruptInfo{Type: "need_info", Prompt: state.Prompt}
		return singleEventIter(adk.StatefulInterrupt(ctx, interrupt, state))
	}
	clarified := combineClarifiedMessage(state.OriginalMessage, resume.Answer)
	adk.AddSessionValue(ctx, "latest_user_message", clarified)
	plan, err := a.planForInput(ctx, &adk.AgentInput{Messages: []*schema.Message{schema.UserMessage(clarified)}})
	if err != nil {
		return a.runClarifyOrError(ctx, &adk.AgentInput{Messages: []*schema.Message{schema.UserMessage(clarified)}}, err)
	}
	a.storePlan(ctx, plan)
	return a.runWithPlan(ctx, nil, plan, opts...)
}

func (a *plannedAgent) runWithPlan(ctx context.Context, input *adk.AgentInput, plan domainExecutionPlan, opts ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	mode := strings.ToLower(strings.TrimSpace(plan.Mode))
	if mode == "" {
		mode = planModeSequential
	}
	eventctx.EmitForNode(ctx, "domain_execute_started", a.key, "开始执行领域计划", g.Map{
		"domain":       a.key,
		"mode":         mode,
		"steps":        planStepKeys(plan),
		"step_details": planStepDetails(plan),
	})
	if mode != planModeSequential {
		return singleErrorIter(fmt.Errorf("unsupported domain execution mode: %s", mode))
	}
	seq, err := a.buildSequentialAgent(ctx, plan)
	if err != nil {
		return singleErrorIter(err)
	}
	return seq.Run(ctx, input, opts...)
}

func (a *plannedAgent) buildSequentialAgent(ctx context.Context, plan domainExecutionPlan) (adk.ResumableAgent, error) {
	subAgents := make([]adk.Agent, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		key := strings.TrimSpace(step.AgentKey)
		if key == "" {
			continue
		}
		leaf, ok := a.leaves[key]
		if !ok || leaf.Agent == nil {
			return nil, fmt.Errorf("unknown planned leaf agent: %s", key)
		}
		subAgents = append(subAgents, leaf.Agent)
	}
	if len(subAgents) == 0 {
		return nil, fmt.Errorf("domain execution plan has no executable steps")
	}
	return adk.NewSequentialAgent(ctx, &adk.SequentialAgentConfig{
		Name:        a.internalWorkflowName(),
		Description: fmt.Sprintf("planned sequential workflow for domain %s", a.key),
		SubAgents:   subAgents,
	})
}

func (a *plannedAgent) internalWorkflowName() string {
	if a == nil || strings.TrimSpace(a.key) == "" {
		return "__domain_workflow"
	}
	return "__" + strings.TrimSpace(a.key) + "_workflow"
}

func (a *plannedAgent) storePlan(ctx context.Context, plan domainExecutionPlan) {
	if a == nil {
		return
	}
	planJSON, err := json.Marshal(plan)
	if err != nil {
		g.Log().Warningf(ctx, "domainassistant store plan marshal failed, domain=%s err=%v", a.key, err)
		return
	}
	adk.AddSessionValue(ctx, sessionPlanKey(a.key), string(planJSON))
}

func (a *plannedAgent) loadPlan(ctx context.Context) (domainExecutionPlan, error) {
	if a == nil {
		return domainExecutionPlan{}, fmt.Errorf("planned agent is nil")
	}
	raw, ok := adk.GetSessionValue(ctx, sessionPlanKey(a.key))
	if !ok {
		return domainExecutionPlan{}, fmt.Errorf("plan session value %s not found", sessionPlanKey(a.key))
	}
	planText, ok := raw.(string)
	if !ok || strings.TrimSpace(planText) == "" {
		return domainExecutionPlan{}, fmt.Errorf("plan session value %s is invalid", sessionPlanKey(a.key))
	}
	var plan domainExecutionPlan
	if err := json.Unmarshal([]byte(planText), &plan); err != nil {
		return domainExecutionPlan{}, err
	}
	return plan, nil
}

func sessionPlanKey(domainKey string) string {
	return "domain_execution_plan:" + strings.TrimSpace(domainKey)
}

func planSummary(plan domainExecutionPlan) string {
	if len(plan.Steps) == 0 {
		return "-"
	}
	items := planStepKeys(plan)
	if len(items) == 0 {
		return "-"
	}
	return strings.Join(items, " -> ")
}

func planStepKeys(plan domainExecutionPlan) []string {
	items := make([]string, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		key := strings.TrimSpace(step.AgentKey)
		if key == "" {
			continue
		}
		items = append(items, key)
	}
	return items
}

func planStepDetails(plan domainExecutionPlan) []g.Map {
	items := make([]g.Map, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		key := strings.TrimSpace(step.AgentKey)
		reason := strings.TrimSpace(step.Reason)
		if key == "" {
			continue
		}
		items = append(items, g.Map{"agent_key": key, "reason": reason})
	}
	return items
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
