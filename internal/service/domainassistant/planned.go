package domainassistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/gogf/gf/v2/frame/g"

	"lakeside/internal/service/agentplatform/eventctx"
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

type plannedAgent struct {
	key         string
	description string
	instruction string
	leaves      map[string]LeafBinding
	planner     *planner
	fallback    adk.ResumableAgent
}

func newPlannedAgent(ctx context.Context, key, description, instruction string, leaves []LeafBinding, fallback adk.ResumableAgent) (adk.ResumableAgent, error) {
	items := make(map[string]LeafBinding, len(leaves))
	for _, leaf := range leaves {
		leaf.Key = strings.TrimSpace(leaf.Key)
		if leaf.Key == "" || leaf.Agent == nil {
			continue
		}
		items[leaf.Key] = leaf
	}
	if len(items) == 0 {
		return fallback, nil
	}
	return &plannedAgent{
		key:         strings.TrimSpace(key),
		description: strings.TrimSpace(description),
		instruction: strings.TrimSpace(instruction),
		leaves:      items,
		planner:     newPlanner(ctx, key, instruction, leaves),
		fallback:    fallback,
	}, nil
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

func (a *plannedAgent) Run(ctx context.Context, input *adk.AgentInput, opts ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	eventctx.EmitForNode(ctx, "domain_plan_started", a.key, "正在生成领域执行计划", g.Map{
		"domain": a.key,
	})
	plan, err := a.planForInput(ctx, input)
	if err != nil {
		g.Log().Warningf(ctx, "domainassistant planner fallback to supervisor, domain=%s err=%v", a.key, err)
		plan = domainExecutionPlan{Mode: planModeSupervisor}
		eventctx.EmitForNode(ctx, "domain_supervisor_fallback", a.key, "规划不稳定，改用动态调度继续处理", g.Map{
			"domain": a.key,
			"reason": err.Error(),
		})
	} else {
		g.Log().Infof(ctx, "domainassistant plan ready, domain=%s mode=%s steps=%s", a.key, strings.TrimSpace(plan.Mode), planSummary(plan))
		eventctx.EmitForNode(ctx, "domain_plan_ready", a.key, "领域执行计划已生成", g.Map{
			"domain":       a.key,
			"mode":         strings.TrimSpace(plan.Mode),
			"steps":        planStepKeys(plan),
			"step_details": planStepDetails(plan),
		})
	}
	a.storePlan(ctx, plan)
	return a.runWithPlan(ctx, input, nil, plan, opts...)
}

func (a *plannedAgent) Resume(ctx context.Context, info *adk.ResumeInfo, opts ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	eventctx.EmitForNode(ctx, "domain_plan_started", a.key, "正在加载领域执行计划", g.Map{
		"domain": a.key,
		"resume": true,
	})
	plan, err := a.loadPlan(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "domainassistant resume missing plan, fallback to supervisor, domain=%s err=%v", a.key, err)
		eventctx.EmitForNode(ctx, "domain_supervisor_fallback", a.key, "未找到既有计划，改用动态调度继续处理", g.Map{
			"domain": a.key,
			"reason": err.Error(),
		})
		if a.fallback == nil {
			return singleErrorIter(fmt.Errorf("domain execution plan not found"))
		}
		return a.fallback.Resume(ctx, info, opts...)
	}
	g.Log().Infof(ctx, "domainassistant resume plan loaded, domain=%s mode=%s steps=%s", a.key, strings.TrimSpace(plan.Mode), planSummary(plan))
	eventctx.EmitForNode(ctx, "domain_plan_ready", a.key, "已加载领域执行计划", g.Map{
		"domain":       a.key,
		"mode":         strings.TrimSpace(plan.Mode),
		"steps":        planStepKeys(plan),
		"step_details": planStepDetails(plan),
		"resume":       true,
	})
	return a.runWithPlan(ctx, nil, info, plan, opts...)
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
	return plan, nil
}

func (a *plannedAgent) runWithPlan(ctx context.Context, input *adk.AgentInput, info *adk.ResumeInfo, plan domainExecutionPlan, opts ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	mode := strings.ToLower(strings.TrimSpace(plan.Mode))
	emitMode := mode
	if emitMode == "" {
		emitMode = planModeSequential
	}
	eventctx.EmitForNode(ctx, "domain_execute_started", a.key, "开始执行领域计划", g.Map{
		"domain":       a.key,
		"mode":         emitMode,
		"steps":        planStepKeys(plan),
		"step_details": planStepDetails(plan),
	})
	switch mode {
	case "", planModeSequential:
		g.Log().Infof(ctx, "domainassistant execute sequential, domain=%s steps=%s", a.key, planSummary(plan))
		seq, err := a.buildSequentialAgent(ctx, plan)
		if err != nil {
			g.Log().Warningf(ctx, "domainassistant build sequential failed, fallback to supervisor, domain=%s err=%v", a.key, err)
			eventctx.EmitForNode(ctx, "domain_supervisor_fallback", a.key, "执行计划不可用，改用动态调度继续处理", g.Map{
				"domain": a.key,
				"reason": err.Error(),
			})
			return a.runFallback(ctx, input, info, opts...)
		}
		if info != nil {
			return seq.Resume(ctx, info, opts...)
		}
		return seq.Run(ctx, input, opts...)
	case planModeSupervisor:
		g.Log().Infof(ctx, "domainassistant execute supervisor fallback, domain=%s", a.key)
		eventctx.EmitForNode(ctx, "domain_supervisor_fallback", a.key, "按计划切换到动态调度执行", g.Map{
			"domain": a.key,
			"reason": "plan_mode_supervisor",
		})
		return a.runFallback(ctx, input, info, opts...)
	default:
		g.Log().Warningf(ctx, "domainassistant unknown plan mode, fallback to supervisor, domain=%s mode=%s", a.key, mode)
		eventctx.EmitForNode(ctx, "domain_supervisor_fallback", a.key, "计划模式不可识别，改用动态调度继续处理", g.Map{
			"domain": a.key,
			"mode":   mode,
			"reason": "unknown_plan_mode",
		})
		return a.runFallback(ctx, input, info, opts...)
	}
}

func (a *plannedAgent) runFallback(ctx context.Context, input *adk.AgentInput, info *adk.ResumeInfo, opts ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	if a == nil || a.fallback == nil {
		return singleErrorIter(fmt.Errorf("domain fallback supervisor is nil"))
	}
	if info != nil {
		return a.fallback.Resume(ctx, info, opts...)
	}
	return a.fallback.Run(ctx, input, opts...)
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
		items = append(items, g.Map{
			"agent_key": key,
			"reason":    reason,
		})
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
		if msg == nil || msg.Role != "user" {
			continue
		}
		if strings.TrimSpace(msg.Content) != "" {
			return strings.TrimSpace(msg.Content)
		}
	}
	return ""
}

func singleErrorIter(err error) *adk.AsyncIterator[*adk.AgentEvent] {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		gen.Send(&adk.AgentEvent{Err: err})
		gen.Close()
	}()
	return iter
}
