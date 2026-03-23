package domainassistant

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"lakeside/internal/service/chatmodels"
	"lakeside/internal/service/moduleapi"
)

type planner struct {
	domainKey   string
	description string
	instruction string
	leaves      []LeafBinding
	model       model.ToolCallingChatModel
}

func newPlanner(ctx context.Context, domainKey, description, instruction string, leaves []LeafBinding) *planner {
	return &planner{
		domainKey:   strings.TrimSpace(domainKey),
		description: strings.TrimSpace(description),
		instruction: strings.TrimSpace(instruction),
		leaves:      append([]LeafBinding(nil), leaves...),
		model:       chatmodels.GetChatModel(ctx),
	}
}

func (p *planner) Assess(ctx context.Context, userMessage string) (moduleapi.Assessment, error) {
	if p == nil {
		return moduleapi.Assessment{}, fmt.Errorf("planner is nil")
	}
	if assessment, ok := p.singleLeafAssessment(userMessage); ok {
		return p.normalizeAssessment(assessment), nil
	}
	if assessment, ok := p.assessByHeuristic(userMessage); ok {
		return p.normalizeAssessment(assessment), nil
	}
	if p.model == nil {
		return moduleapi.Assessment{}, fmt.Errorf("planner model is nil")
	}
	messages := []*schema.Message{
		schema.SystemMessage(`你是 Lakeside 顶层模块路由评估器。你的任务不是回答用户问题，而是判断“当前领域模块是否应该处理这条请求”。你必须只返回 JSON。`),
		schema.UserMessage(p.buildAssessmentPrompt(ctx, userMessage)),
	}
	msg, err := p.model.Generate(ctx, messages)
	if err != nil {
		return moduleapi.Assessment{}, err
	}
	if msg == nil || strings.TrimSpace(msg.Content) == "" {
		return moduleapi.Assessment{}, fmt.Errorf("planner assessment returned empty content")
	}
	jsonText, err := findJSONObject(msg.Content)
	if err != nil {
		return moduleapi.Assessment{}, err
	}
	assessment, err := decodeAssessmentJSON(jsonText)
	if err != nil {
		return moduleapi.Assessment{}, err
	}
	return p.normalizeAssessment(assessment), nil
}

func (p *planner) Plan(ctx context.Context, userMessage string) (domainExecutionPlan, error) {
	if p == nil {
		return domainExecutionPlan{}, fmt.Errorf("planner is nil")
	}
	if plan, ok := p.singleLeafPlan(userMessage); ok {
		return p.normalizePlan(plan), nil
	}
	if plan, ok := p.planByHeuristic(userMessage); ok {
		return p.normalizePlan(plan), nil
	}
	if p.model == nil {
		return p.planFallbackOrError(userMessage, fmt.Errorf("planner model is nil"))
	}
	messages := []*schema.Message{
		schema.SystemMessage(`你是 Lakeside 领域子代理执行计划器。你的任务不是回答用户问题，而是决定当前领域下应该按什么顺序调用哪些叶子能力。你必须只返回 JSON。`),
		schema.UserMessage(p.buildPlanPrompt(ctx, userMessage)),
	}
	msg, err := p.model.Generate(ctx, messages)
	if err != nil {
		return p.planFallbackOrError(userMessage, err)
	}
	if msg == nil || strings.TrimSpace(msg.Content) == "" {
		return p.planFallbackOrError(userMessage, fmt.Errorf("planner returned empty content"))
	}
	jsonText, err := findJSONObject(msg.Content)
	if err != nil {
		return p.planFallbackOrError(userMessage, err)
	}
	plan, err := decodePlanJSON(jsonText)
	if err != nil {
		return p.planFallbackOrError(userMessage, err)
	}
	normalized := p.normalizePlan(plan)
	if strings.EqualFold(strings.TrimSpace(normalized.Mode), planModeSupervisor) || len(normalized.Steps) == 0 {
		return p.planFallbackOrError(userMessage, fmt.Errorf("planner cannot derive executable plan"))
	}
	return normalized, nil
}

func (p *planner) assessByHeuristic(userMessage string) (moduleapi.Assessment, bool) {
	msg := strings.ToLower(strings.TrimSpace(userMessage))
	if msg == "" {
		return moduleapi.Assessment{
			Status:         moduleapi.AssessmentNeedClarify,
			Phase:          moduleapi.PhaseRead,
			Score:          0.55,
			Reason:         "user message is empty",
			FollowUpPrompt: p.defaultClarifyPrompt(),
		}, true
	}

	if p.looksVague(msg) {
		return moduleapi.Assessment{
			Status:         moduleapi.AssessmentNeedClarify,
			Phase:          moduleapi.PhaseRead,
			Score:          0.62,
			Reason:         "request is too vague to route reliably",
			FollowUpPrompt: p.defaultClarifyPrompt(),
		}, true
	}

	itSignals := containsAny(msg,
		"it", "vpn", "wifi", "wi-fi", "wireless", "network", "internet", "ethernet", "lan", "login", "log in", "password", "account", "email", "mailbox", "outlook", "microsoft 365", "office 365", "teams", "onedrive", "canvas", "blackboard", "software", "app", "application", "system", "portal", "printing", "printer", "laptop", "电脑", "网络", "校园网", "宿舍网", "wifi", "账号", "账户", "登录", "密码", "邮箱", "vpn", "打印", "打印机", "软件", "系统", "电脑", "连不上", "无法上网", "打不开", "报修", "工单", "提单", "报障",
	)
	nonITSignals := containsAny(msg,
		"canteen", "dining", "meal", "food", "scholarship", "course", "class registration", "grades", "exam", "library", "gym", "dorm application", "leave request", "counseling", "食堂", "饭堂", "奖学金", "选课", "教务", "成绩", "考试", "图书馆", "体育馆", "请假", "宿舍申请", "心理咨询",
	)
	explicitWrite := containsAny(msg,
		"帮我报修", "帮我提工单", "帮我提交工单", "帮我开工单", "现在报修", "现在提工单", "提工单吧", "提个工单", "提交工单", "开工单", "开个工单", "直接提单", "直接报修", "立刻报修", "马上报修", "请帮我报修", "报修吧", "报个修", "报障吧", "我想报修", "我想提工单", "report a ticket", "create ticket", "submit ticket", "open a ticket", "file a ticket", "raise a ticket",
	)

	if nonITSignals && !itSignals {
		return moduleapi.Assessment{
			Status: moduleapi.AssessmentReject,
			Phase:  moduleapi.PhaseRead,
			Score:  0.05,
			Reason: "request appears unrelated to this domain",
		}, true
	}

	if itSignals {
		phase := moduleapi.PhaseRead
		reason := "request matches domain knowledge/support capability"
		score := 0.84
		if explicitWrite && p.hasInterruptibleLeaf() {
			phase = moduleapi.PhaseWrite
			reason = "request asks for a formal interruptible workflow"
			score = 0.9
		}
		return moduleapi.Assessment{
			Status: moduleapi.AssessmentReady,
			Phase:  phase,
			Score:  score,
			Reason: reason,
		}, true
	}

	return moduleapi.Assessment{}, false
}

func (p *planner) singleLeafAssessment(userMessage string) (moduleapi.Assessment, bool) {
	leaf, ok := p.singleExecutableLeaf()
	if !ok {
		return moduleapi.Assessment{}, false
	}
	msg := strings.ToLower(strings.TrimSpace(userMessage))
	if msg == "" || p.looksVague(msg) {
		return moduleapi.Assessment{
			Status:         moduleapi.AssessmentNeedClarify,
			Phase:          moduleapi.PhaseRead,
			Score:          0.62,
			Reason:         "single-leaf domain still needs a minimally specific request",
			FollowUpPrompt: p.defaultClarifyPrompt(),
		}, true
	}
	phase := moduleapi.PhaseRead
	if leaf.Interruptible {
		phase = moduleapi.PhaseWrite
	}
	return moduleapi.Assessment{
		Status: moduleapi.AssessmentReady,
		Phase:  phase,
		Score:  0.91,
		Reason: fmt.Sprintf("single-leaf domain routes directly to %s", strings.TrimSpace(leaf.Key)),
	}, true
}

func (p *planner) planByHeuristic(userMessage string) (domainExecutionPlan, bool) {
	msg := strings.ToLower(strings.TrimSpace(userMessage))
	if msg == "" {
		return domainExecutionPlan{}, false
	}
	knowledgeKey := p.preferredKnowledgeAgent(msg)
	interruptKey := p.primaryInterruptibleAgent()
	if knowledgeKey == "" && interruptKey == "" {
		return domainExecutionPlan{}, false
	}

	wantsKnowledgeGuidance := containsAny(msg,
		"怎么办", "怎么处理", "如何处理", "怎么排查", "如何排查", "排查", "先帮我排查", "先排查", "给我步骤", "请给步骤", "排查建议", "安装指引", "为什么", "原因是什么", "怎么解决",
		"how to", "what should i do", "troubleshoot", "troubleshooting", "guide", "instruction", "why", "how can i fix",
	)
	wantsKnowledgeLookup := containsAny(msg,
		"是什么", "是啥", "是什么？", "多少", "几点", "哪里", "在哪", "在哪里", "地址", "入口", "链接", "流程", "要求", "规则", "说明", "文档", "邮箱地址", "群组邮箱", "邮箱群组",
		"what is", "where is", "where can i", "which", "how many", "address", "link", "document", "faq",
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

	if askHowToReport && !explicitSubmit && knowledgeKey != "" {
		return domainExecutionPlan{
			Mode: planModeSequential,
			Steps: []domainPlanStep{{
				AgentKey: knowledgeKey,
				Reason:   "用户询问报修方式，先给知识说明",
			}},
		}, true
	}

	if interruptKey != "" && hasProcessWords && (explicitSubmit || knowledgeAlreadyTried) && !wantsKnowledgeGuidance {
		return domainExecutionPlan{
			Mode: planModeSequential,
			Steps: []domainPlanStep{{
				AgentKey: interruptKey,
				Reason:   "用户明确要求正式提交流程，不再重复知识排查",
			}},
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

	if (wantsKnowledgeGuidance || wantsKnowledgeLookup) && !hasProcessWords && knowledgeKey != "" {
		return domainExecutionPlan{
			Mode: planModeSequential,
			Steps: []domainPlanStep{{
				AgentKey: knowledgeKey,
				Reason:   "纯知识查询诉求",
			}},
		}, true
	}

	return domainExecutionPlan{}, false
}

func (p *planner) singleLeafPlan(userMessage string) (domainExecutionPlan, bool) {
	leaf, ok := p.singleExecutableLeaf()
	if !ok {
		return domainExecutionPlan{}, false
	}
	assessment, assessed := p.singleLeafAssessment(userMessage)
	if !assessed || assessment.Status != moduleapi.AssessmentReady {
		return domainExecutionPlan{}, false
	}
	return domainExecutionPlan{
		Mode: planModeSequential,
		Steps: []domainPlanStep{{
			AgentKey: strings.TrimSpace(leaf.Key),
			Reason:   "单叶子领域直接进入唯一执行能力",
		}},
	}, true
}

func (p *planner) preferredKnowledgeAgent(msg string) string {
	if p == nil {
		return ""
	}
	if p.isStudentAssistantInternalKnowledgeRequest(msg) {
		if key := p.matchingKnowledgeAgent(func(leaf LeafBinding) bool {
			return containsAny(strings.ToLower(strings.TrimSpace(leaf.Key)), "itso", "student_assistant", "studentassistant") ||
				containsAny(strings.ToLower(strings.TrimSpace(leaf.Description)), "itso", "学生助理", "内部")
		}); key != "" {
			return key
		}
	}
	if key := p.matchingKnowledgeAgent(func(leaf LeafBinding) bool {
		return !containsAny(strings.ToLower(strings.TrimSpace(leaf.Key)), "itso", "student_assistant", "studentassistant") &&
			!containsAny(strings.ToLower(strings.TrimSpace(leaf.Description)), "itso", "学生助理", "内部")
	}); key != "" {
		return key
	}
	return p.primaryKnowledgeAgent()
}

func (p *planner) fallbackExecutablePlan(userMessage string) (domainExecutionPlan, bool) {
	if p == nil {
		return domainExecutionPlan{}, false
	}
	if plan, ok := p.singleLeafPlan(userMessage); ok {
		return plan, true
	}
	assessment, ok := p.assessByHeuristic(userMessage)
	if !ok {
		return domainExecutionPlan{}, false
	}
	assessment = p.normalizeAssessment(assessment)
	if assessment.Status != moduleapi.AssessmentReady {
		return domainExecutionPlan{}, false
	}
	msg := strings.ToLower(strings.TrimSpace(userMessage))
	switch assessment.Phase {
	case moduleapi.PhaseWrite:
		if key := p.primaryInterruptibleAgent(); key != "" {
			return domainExecutionPlan{
				Mode: planModeSequential,
				Steps: []domainPlanStep{{
					AgentKey: key,
					Reason:   "规划兜底：依据评估结果进入正式流程",
				}},
			}, true
		}
	case moduleapi.PhaseRead:
		if key := p.preferredKnowledgeAgent(msg); key != "" {
			return domainExecutionPlan{
				Mode: planModeSequential,
				Steps: []domainPlanStep{{
					AgentKey: key,
					Reason:   "规划兜底：依据评估结果进入知识查询",
				}},
			}, true
		}
	}
	return domainExecutionPlan{}, false
}

func (p *planner) singleExecutableLeaf() (LeafBinding, bool) {
	if p == nil || len(p.leaves) != 1 {
		return LeafBinding{}, false
	}
	leaf := p.leaves[0]
	if strings.TrimSpace(leaf.Key) == "" {
		return LeafBinding{}, false
	}
	return leaf, true
}

func (p *planner) planFallbackOrError(userMessage string, cause error) (domainExecutionPlan, error) {
	if fallback, ok := p.fallbackExecutablePlan(userMessage); ok {
		return fallback, nil
	}
	return domainExecutionPlan{}, cause
}

func (p *planner) primaryKnowledgeAgent() string {
	return p.matchingKnowledgeAgent(func(leaf LeafBinding) bool { return true })
}

func (p *planner) matchingKnowledgeAgent(match func(leaf LeafBinding) bool) string {
	for _, leaf := range p.leaves {
		key := strings.TrimSpace(leaf.Key)
		if key == "" {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(leaf.Kind))
		if (kind == "knowledge" || !leaf.Interruptible) && match(leaf) {
			return key
		}
	}
	return ""
}

func (p *planner) isStudentAssistantInternalKnowledgeRequest(msg string) bool {
	return containsAny(msg,
		"学生助理", "itso", "学生群组邮箱", "群组邮箱", "邮箱群组", "学生邮箱群组", "语料库", "引导话术", "内部规范", "内部手册",
		"student assistant", "internal handbook", "internal guideline",
	)
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

func (p *planner) hasInterruptibleLeaf() bool {
	return p.primaryInterruptibleAgent() != ""
}

func (p *planner) looksVague(msg string) bool {
	if len([]rune(msg)) <= 4 {
		return true
	}
	return containsAny(msg,
		"帮帮我", "有问题", "出问题了", "不行了", "有故障", "帮我看看", "看一下", "处理一下", "help", "please help", "something is wrong",
	)
}

func (p *planner) defaultClarifyPrompt() string {
	label := strings.TrimSpace(p.description)
	if label == "" {
		label = strings.TrimSpace(p.domainKey)
	}
	if label == "" {
		return "请再补充一点信息，说明你要咨询的具体问题或希望我执行的操作。"
	}
	return fmt.Sprintf("请再补充一点信息，说明这次和%s相关的具体问题或你希望我执行的操作。", label)
}

func (p *planner) buildAssessmentPrompt(ctx context.Context, userMessage string) string {
	var builder strings.Builder
	builder.WriteString("当前领域模块：")
	builder.WriteString(p.domainKey)
	builder.WriteString("\n描述：")
	builder.WriteString(p.description)
	builder.WriteString("\n\n领域规则：\n")
	builder.WriteString(renderRuntimeTemplate(ctx, p.instruction))
	builder.WriteString("\n\n当前领域可用能力：\n")
	builder.WriteString(p.renderLeafCatalog())
	builder.WriteString("\n\n用户原始问题：\n")
	builder.WriteString(strings.TrimSpace(userMessage))
	builder.WriteString("\n\n评估要求：\n")
	builder.WriteString(`- 你的目标是判断“这个领域模块是否应该处理当前请求”，不是给最终答案。
- status 只能是 ready、need_clarify、reject 三选一。
- ready：当前领域足以处理该请求。
- need_clarify：当前领域可能能处理，但还缺一个最小澄清信息。
- reject：当前领域不应接手，应该由其他模块或通用兜底处理。
- phase 只能是 read 或 write。
- 纯知识问答、查询、排查建议属于 read。
- 明确要求正式提交/报修/投诉/发起流程，且当前领域存在中断型能力时，属于 write。
- 如果 status 不是 need_clarify，follow_up_prompt 必须为空字符串。
- 只返回 JSON。\n\n你必须只返回如下 JSON 之一：
{"status":"ready","phase":"read","score":0.91,"reason":"用户咨询校园网/VPN 使用方式","follow_up_prompt":""}
{"status":"need_clarify","phase":"read","score":0.55,"reason":"请求太模糊，尚不清楚是否属于该领域","follow_up_prompt":"请补充具体问题现象或你希望我执行的操作。"}
{"status":"reject","phase":"read","score":0.05,"reason":"请求更像其他校园部门问题","follow_up_prompt":""}`)
	return builder.String()
}

func (p *planner) buildPlanPrompt(ctx context.Context, userMessage string) string {
	var builder strings.Builder
	builder.WriteString("当前领域：")
	builder.WriteString(p.domainKey)
	builder.WriteString("\n描述：")
	builder.WriteString(p.description)
	builder.WriteString("\n\n当前领域规则：\n")
	builder.WriteString(renderRuntimeTemplate(ctx, p.instruction))
	builder.WriteString("\n\n可用叶子能力：\n")
	builder.WriteString(p.renderLeafCatalog())
	builder.WriteString("\n\n用户原始问题：\n")
	builder.WriteString(strings.TrimSpace(userMessage))
	builder.WriteString("\n\n规划要求：\n")
	builder.WriteString(`- 只选择当前真的需要调用的最少叶子能力。
- 如果一个非中断型 knowledge tool 足以回答知识部分，就只选它。
- 如果同一轮同时包含“先解释/排查/给步骤/查询知识”和“再报修/投诉/提交正式流程”，默认先选非中断型 tool，再选中断型 agent。
- 只有当用户明确要求“先报修”“先提工单”“不要讲步骤直接提单”时，才允许把中断型 agent 放到前面。
- 不要因为某个能力存在就机械选它；必须依据用户问题与能力 description 匹配。
- 除非确有必要，否则不要重复选择同一个能力。
- 如果信息不足导致当前无法可靠规划，返回空 steps，交给上层 follow-up 处理。

你必须只返回如下 JSON：
{"mode":"sequential","steps":[{"agent_key":"knowledge_agent_key","reason":"先回答知识问题"},{"agent_key":"itsm","reason":"再进入正式流程"}]}
或
{"mode":"sequential","steps":[]}`)
	return builder.String()
}

func (p *planner) renderLeafCatalog() string {
	items := make([]string, 0, len(p.leaves))
	for _, leaf := range p.leaves {
		key := strings.TrimSpace(leaf.Key)
		if key == "" {
			continue
		}
		var builder strings.Builder
		builder.WriteString("- key=")
		builder.WriteString(key)
		builder.WriteString(", type=")
		builder.WriteString(strings.TrimSpace(leaf.Kind))
		builder.WriteString(", execution=")
		if leaf.Tool != nil {
			builder.WriteString("tool")
		} else {
			builder.WriteString("agent")
		}
		builder.WriteString(", interruptible=")
		if leaf.Interruptible {
			builder.WriteString("true")
		} else {
			builder.WriteString("false")
		}
		builder.WriteString(", description=")
		builder.WriteString(strings.TrimSpace(leaf.Description))
		items = append(items, builder.String())
	}
	return strings.Join(items, "\n")
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
		steps = append(steps, domainPlanStep{AgentKey: key, Reason: strings.TrimSpace(step.Reason)})
	}
	return domainExecutionPlan{Mode: mode, Steps: steps}
}

func (p *planner) normalizeAssessment(assessment moduleapi.Assessment) moduleapi.Assessment {
	assessment.ModuleKey = strings.TrimSpace(p.domainKey)
	switch assessment.Status {
	case moduleapi.AssessmentReady, moduleapi.AssessmentNeedClarify, moduleapi.AssessmentReject:
	default:
		assessment.Status = moduleapi.AssessmentReject
	}
	switch assessment.Phase {
	case moduleapi.PhaseRead, moduleapi.PhaseWrite:
	default:
		assessment.Phase = moduleapi.PhaseRead
	}
	assessment.Score = clampScore(assessment.Score)
	if assessment.Score == 0 {
		switch assessment.Status {
		case moduleapi.AssessmentReady:
			assessment.Score = 0.8
		case moduleapi.AssessmentNeedClarify:
			assessment.Score = 0.55
		default:
			assessment.Score = 0.05
		}
	}
	assessment.Reason = strings.TrimSpace(assessment.Reason)
	assessment.FollowUpPrompt = strings.TrimSpace(assessment.FollowUpPrompt)
	if assessment.Status != moduleapi.AssessmentNeedClarify {
		assessment.FollowUpPrompt = ""
	}
	if assessment.Status == moduleapi.AssessmentNeedClarify && assessment.FollowUpPrompt == "" {
		assessment.FollowUpPrompt = p.defaultClarifyPrompt()
	}
	return assessment
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

func decodeAssessmentJSON(raw string) (moduleapi.Assessment, error) {
	var assessment moduleapi.Assessment
	if err := json.Unmarshal([]byte(raw), &assessment); err != nil {
		return moduleapi.Assessment{}, err
	}
	return assessment, nil
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

func clampScore(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func sortAssessmentsByScore(items []moduleapi.Assessment, order map[string]int) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score != items[j].Score {
			return items[i].Score > items[j].Score
		}
		return order[items[i].ModuleKey] < order[items[j].ModuleKey]
	})
}
