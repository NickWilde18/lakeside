package domainassistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"lakeside/internal/service/chatmodels"
)

type planner struct {
	domainKey   string
	instruction string
	leaves      []LeafBinding
	model       model.ToolCallingChatModel
}

func newPlanner(ctx context.Context, domainKey, instruction string, leaves []LeafBinding) *planner {
	return &planner{
		domainKey:   strings.TrimSpace(domainKey),
		instruction: strings.TrimSpace(instruction),
		leaves:      append([]LeafBinding(nil), leaves...),
		model:       chatmodels.GetChatModel(ctx),
	}
}

func (p *planner) Plan(ctx context.Context, userMessage string) (domainExecutionPlan, error) {
	if p == nil || p.model == nil {
		return domainExecutionPlan{}, fmt.Errorf("planner model is nil")
	}
	if plan, ok := p.planByHeuristic(userMessage); ok {
		return p.normalizePlan(plan), nil
	}
	messages := []*schema.Message{
		schema.SystemMessage(`你是 Lakeside 领域子代理执行计划器。你的任务不是回答用户问题，而是决定当前领域下应该按什么顺序调用哪些叶子 agent。你必须只返回 JSON。`),
		schema.UserMessage(p.buildPrompt(ctx, userMessage)),
	}
	msg, err := p.model.Generate(ctx, messages)
	if err != nil {
		return domainExecutionPlan{}, err
	}
	if msg == nil || strings.TrimSpace(msg.Content) == "" {
		return domainExecutionPlan{}, fmt.Errorf("planner returned empty content")
	}
	jsonText, err := findJSONObject(msg.Content)
	if err != nil {
		return domainExecutionPlan{}, err
	}
	plan, err := decodePlanJSON(jsonText)
	if err != nil {
		return domainExecutionPlan{}, err
	}
	return p.normalizePlan(plan), nil
}

func (p *planner) planByHeuristic(userMessage string) (domainExecutionPlan, bool) {
	msg := strings.ToLower(strings.TrimSpace(userMessage))
	if msg == "" {
		return domainExecutionPlan{}, false
	}
	knowledgeKey := p.primaryKnowledgeAgent()
	interruptKey := p.primaryInterruptibleAgent()
	if knowledgeKey == "" && interruptKey == "" {
		return domainExecutionPlan{}, false
	}

	wantsKnowledgeGuidance := containsAny(msg,
		"怎么办", "怎么处理", "如何处理", "怎么排查", "如何排查", "排查", "先帮我排查", "先排查", "给我步骤", "请给步骤", "排查建议", "安装指引", "为什么", "原因是什么", "怎么解决",
		"how to", "what should i do", "troubleshoot", "troubleshooting", "guide", "instruction", "why", "how can i fix",
	)
	explicitSubmit := containsAny(msg,
		"帮我报修", "帮我提工单", "帮我提交工单", "帮我开工单", "现在报修", "现在提工单", "提工单吧", "提个工单", "提交工单", "开工单", "开个工单",
		"直接提单", "直接报修", "立刻报修", "马上报修", "请帮我报修", "先报修", "先提工单", "报修吧", "报个修", "报障吧", "我想报修", "我想提工单",
		"report a ticket", "create ticket", "submit ticket", "open a ticket", "file a ticket", "raise a ticket",
	)
	askHowToReport := containsAny(msg,
		"怎么报修", "如何报修", "报修流程", "报修入口", "怎么提工单", "如何提工单", "怎么投诉", "如何投诉", "投诉流程", "怎么反馈", "如何反馈",
		"how to report", "how do i report", "how to submit", "reporting process", "ticket process",
	)
	knowledgeAlreadyTried := containsAny(msg,
		"还是不行", "还是不可以", "还是失败", "依然不行", "仍然不行", "还是连不上", "还是打不开", "没用", "我试过了", "我尝试了", "尝试之后", "试了之后",
		"still not working", "still doesn't work", "i tried", "i've tried", "no luck", "after trying",
	)
	hasProcessWords := containsAny(msg,
		"报修", "工单", "提单", "报障", "投诉", "反馈",
		"ticket", "report", "complain", "complaint", "feedback",
	)

	// “怎么报修/如何报修”属于知识咨询，不应直接进入中断型工单流程。
	if askHowToReport && !explicitSubmit && knowledgeKey != "" {
		return domainExecutionPlan{
			Mode: planModeSequential,
			Steps: []domainPlanStep{
				{AgentKey: knowledgeKey, Reason: "用户询问报修方式，先给知识说明"},
			},
		}, true
	}

	// 用户明确表示“前面的建议已经试过但仍然不行”，并且当前目的是发起正式流程时，
	// 不要再重复调用 knowledge，直接进入 itsm。
	if interruptKey != "" && hasProcessWords && (explicitSubmit || knowledgeAlreadyTried) && !wantsKnowledgeGuidance {
		return domainExecutionPlan{
			Mode: planModeSequential,
			Steps: []domainPlanStep{
				{AgentKey: interruptKey, Reason: "用户明确要求正式提交流程，不再重复知识排查"},
			},
		}, true
	}

	if explicitSubmit {
		steps := make([]domainPlanStep, 0, 2)
		if wantsKnowledgeGuidance && knowledgeKey != "" {
			steps = append(steps, domainPlanStep{AgentKey: knowledgeKey, Reason: "先给排查建议"})
		}
		if interruptKey != "" {
			steps = append(steps, domainPlanStep{AgentKey: interruptKey, Reason: "用户明确要求发起正式流程"})
		}
		if len(steps) > 0 {
			return domainExecutionPlan{Mode: planModeSequential, Steps: steps}, true
		}
	}

	if wantsKnowledgeGuidance && !hasProcessWords && knowledgeKey != "" {
		return domainExecutionPlan{
			Mode: planModeSequential,
			Steps: []domainPlanStep{
				{AgentKey: knowledgeKey, Reason: "纯知识排查诉求"},
			},
		}, true
	}

	return domainExecutionPlan{}, false
}

func (p *planner) primaryKnowledgeAgent() string {
	for _, leaf := range p.leaves {
		key := strings.TrimSpace(leaf.Key)
		if key == "" {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(leaf.Kind))
		if kind == "knowledge" || !leaf.Interruptible {
			return key
		}
	}
	return ""
}

func (p *planner) primaryInterruptibleAgent() string {
	for _, leaf := range p.leaves {
		key := strings.TrimSpace(leaf.Key)
		if key == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(leaf.Kind), "itsm") {
			return key
		}
	}
	for _, leaf := range p.leaves {
		key := strings.TrimSpace(leaf.Key)
		if key == "" {
			continue
		}
		if leaf.Interruptible {
			return key
		}
	}
	return ""
}

func containsAny(text string, patterns ...string) bool {
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(strings.ToLower(pattern))
		if pattern == "" {
			continue
		}
		if strings.Contains(text, pattern) {
			return true
		}
	}
	return false
}

func (p *planner) buildPrompt(ctx context.Context, userMessage string) string {
	var builder strings.Builder
	builder.WriteString("当前领域：")
	builder.WriteString(p.domainKey)
	builder.WriteString("\n\n")
	builder.WriteString("当前领域规则：\n")
	builder.WriteString(renderRuntimeTemplate(ctx, p.instruction))
	builder.WriteString("\n\n")
	builder.WriteString("可用叶子 agent：\n")
	for _, leaf := range p.leaves {
		builder.WriteString("- key=")
		builder.WriteString(strings.TrimSpace(leaf.Key))
		builder.WriteString(", type=")
		builder.WriteString(strings.TrimSpace(leaf.Kind))
		builder.WriteString(", interruptible=")
		if leaf.Interruptible {
			builder.WriteString("true")
		} else {
			builder.WriteString("false")
		}
		builder.WriteString(", description=")
		builder.WriteString(strings.TrimSpace(leaf.Description))
		builder.WriteString("\n")
	}
	builder.WriteString("\n用户原始问题：\n")
	builder.WriteString(strings.TrimSpace(userMessage))
	builder.WriteString("\n\n")
	builder.WriteString(`规划要求：
- 只选择当前真的需要调用的最少叶子 agent。
- 如果一个非中断型 knowledge 叶子 agent 足以回答知识部分，就只选它。
- 如果同一轮同时包含“先解释/排查/给步骤/查询知识”和“再报修/投诉/提交正式流程”，默认先选非中断型 agent，再选中断型 agent。
- 只有当用户明确要求“先报修”“先提工单”“不要讲步骤直接提单”时，才允许把中断型 agent 放到前面。
- 不要因为某个 agent 存在就机械选它；必须依据用户问题与 agent description 匹配。
- 除非确有必要，否则不要重复选择同一个 agent。
- 如果当前无法可靠判断顺序，返回 {"mode":"supervisor","steps":[]} 让上层回退到 supervisor。

你必须只返回如下 JSON 之一：
{"mode":"sequential","steps":[{"agent_key":"knowledge_agent_key","reason":"先回答知识问题"},{"agent_key":"itsm","reason":"再进入正式流程"}]}
{"mode":"supervisor","steps":[]}`)
	return builder.String()
}

func (p *planner) normalizePlan(plan domainExecutionPlan) domainExecutionPlan {
	allowed := make(map[string]LeafBinding, len(p.leaves))
	for _, leaf := range p.leaves {
		allowed[strings.TrimSpace(leaf.Key)] = leaf
	}
	mode := strings.ToLower(strings.TrimSpace(plan.Mode))
	if mode == "" {
		mode = planModeSequential
	}
	steps := make([]domainPlanStep, 0, len(plan.Steps))
	seen := make(map[string]struct{}, len(plan.Steps))
	for _, step := range plan.Steps {
		key := strings.TrimSpace(step.AgentKey)
		if key == "" {
			continue
		}
		if _, ok := allowed[key]; !ok {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		steps = append(steps, domainPlanStep{
			AgentKey: key,
			Reason:   strings.TrimSpace(step.Reason),
		})
	}
	if len(steps) == 0 && mode != planModeSupervisor {
		mode = planModeSupervisor
	}
	return domainExecutionPlan{
		Mode:  mode,
		Steps: steps,
	}
}

func renderRuntimeTemplate(ctx context.Context, text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	values := adkSessionStrings(ctx)
	replacements := make([]string, 0, len(values)*2)
	for key, value := range values {
		replacements = append(replacements, "{"+key+"}", value)
	}
	if len(replacements) == 0 {
		return text
	}
	return strings.NewReplacer(replacements...).Replace(text)
}

func adkSessionStrings(ctx context.Context) map[string]string {
	values := make(map[string]string)
	for key, value := range adk.GetSessionValues(ctx) {
		switch v := value.(type) {
		case string:
			values[key] = strings.TrimSpace(v)
		}
	}
	return values
}

func decodePlanJSON(raw string) (domainExecutionPlan, error) {
	var plan domainExecutionPlan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return domainExecutionPlan{}, err
	}
	return plan, nil
}

func findJSONObject(content string) (string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", fmt.Errorf("planner output is empty")
	}
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return "", fmt.Errorf("planner output has no json object")
	}
	return strings.TrimSpace(content[start : end+1]), nil
}
