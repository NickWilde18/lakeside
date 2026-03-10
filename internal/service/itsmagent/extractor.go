package itsmagent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"

	"lakeside/internal/service/chatmodels"
)

type Extractor struct{}

func NewExtractor() *Extractor {
	return &Extractor{}
}

func (e *Extractor) FillDraft(ctx context.Context, draft TicketDraft, userInput string, assistantContext string) (TicketDraft, string, error) {
	// FillDraft 只负责“尽量补齐草稿”，不会在这里决定是否可以进入下一阶段。
	// 是否完整由 agent.go 中的 needInfoInterrupt 统一裁决。
	res, err := e.Extract(ctx, userInput, draft, assistantContext)
	if err != nil {
		return draft, "", err
	}

	if s := strings.TrimSpace(res.Subject); s != "" {
		draft.Subject = s
	}
	if s := strings.TrimSpace(res.OthersDesc); s != "" {
		draft.OthersDesc = s
	}

	if s, ok := normalizeServiceLevel(res.ServiceLevel); ok {
		draft.ServiceLevel = s
		if res.ServiceLevelConfidence > 0 {
			draft.ServiceLevelConfidence = res.ServiceLevelConfidence
		} else {
			// 模型已给出明确枚举值但未回置信度时，按高置信处理，避免重复追问。
			draft.ServiceLevelConfidence = 1
		}
	}

	if s, ok := normalizePriority(res.Priority); ok {
		draft.Priority = s
		if res.PriorityConfidence > 0 {
			draft.PriorityConfidence = res.PriorityConfidence
		} else {
			// 与 serviceLevel 一致：命中合法枚举且缺省置信度时补为 1。
			draft.PriorityConfidence = 1
		}
	}

	if draft.OthersDesc == "" {
		// 最差兜底：把用户原话放到描述，避免草稿出现空描述。
		draft.OthersDesc = strings.TrimSpace(userInput)
	}

	return draft, strings.TrimSpace(res.ClarifyQuestion), nil
}

func (e *Extractor) Extract(ctx context.Context, userInput string, current TicketDraft, assistantContext string) (*ExtractResult, error) {
	// Extract 与模型的唯一契约是“返回 JSON”，因此 system/user prompt 都强调只输出结构化结果。
	startedAt := time.Now()
	g.Log().Infof(ctx, "itsm extractor started input_len=%d assistant_context_len=%d", len(strings.TrimSpace(userInput)), len(strings.TrimSpace(assistantContext)))
	prompt := buildExtractPrompt(current, userInput, assistantContext)
	messages := []*schema.Message{
		schema.SystemMessage(`你是 ITSM 工单字段抽取助手。请根据用户输入和当前草稿补全工单字段。只返回 JSON 对象，不要输出解释，不要输出 Markdown。`),
		schema.UserMessage(prompt),
	}

	msg, err := chatmodels.GetChatModel(ctx).Generate(ctx, messages)
	if err != nil {
		g.Log().Warningf(ctx, "itsm extractor generate failed duration_ms=%d err=%v", time.Since(startedAt).Milliseconds(), err)
		return nil, fmt.Errorf("extract fields generate failed: %w", err)
	}
	if msg == nil || strings.TrimSpace(msg.Content) == "" {
		g.Log().Warningf(ctx, "itsm extractor got empty output duration_ms=%d", time.Since(startedAt).Milliseconds())
		return nil, fmt.Errorf("extract fields got empty model output")
	}

	jsonText, err := findJSONObject(msg.Content)
	if err != nil {
		g.Log().Warningf(ctx, "itsm extractor failed to locate json duration_ms=%d err=%v", time.Since(startedAt).Milliseconds(), err)
		return nil, fmt.Errorf("extract fields response has no json object: %w", err)
	}

	var out ExtractResult
	if err := json.Unmarshal([]byte(jsonText), &out); err != nil {
		g.Log().Warningf(ctx, "itsm extractor decode failed duration_ms=%d err=%v", time.Since(startedAt).Milliseconds(), err)
		return nil, fmt.Errorf("extract fields decode failed: %w", err)
	}
	g.Log().Infof(ctx, "itsm extractor completed duration_ms=%d subject_set=%t service_level=%s priority=%s", time.Since(startedAt).Milliseconds(), strings.TrimSpace(out.Subject) != "", strings.TrimSpace(out.ServiceLevel), strings.TrimSpace(out.Priority))
	return &out, nil
}

func buildExtractPrompt(current TicketDraft, userInput string, assistantContext string) string {
	return fmt.Sprintf(`任务：根据用户输入抽取 ITSM 工单字段，并补全当前草稿。

当前草稿 JSON：
{"userCode":%q,"subject":%q,"serviceLevel":%q,"priority":%q,"othersDesc":%q}

主助手补充上下文：
%s

用户输入：
%q

请严格按以下 JSON 结构返回：
{
  "subject": "string",
  "othersDesc": "string",
  "serviceLevel": "1|2|3|4|\"\"",
  "serviceLevel_confidence": 0.0,
  "priority": "1|2|3|4|\"\"",
  "priority_confidence": 0.0,
  "clarify_question": "string"
}

规则：
- 必须只返回 JSON 对象，不要输出解释，不要输出 Markdown。
- subject 和 othersDesc 要简洁、可直接用于创建工单。
- 即使没有单独字段，地点、故障现象、影响范围等关键信息也要尽量体现在 subject 和 othersDesc 中。
- WiFi/网络故障类问题，完整信息优先覆盖：地点、故障现象、影响范围。
- 如果缺地点，clarify_question 优先追问楼号和房间号。
- 如果缺故障现象，clarify_question 优先追问是搜不到信号、连上没网、间歇断连，还是其他具体现象。
- 如果缺影响范围，clarify_question 优先追问是单个设备、多台设备，还是整间宿舍/整个房间受影响。
- serviceLevel 判定规则：
  - 默认填写 3（中）。
  - 当用户明确描述“同时有多个人 / 多个寝室 / 多位同事 / 多台终端”出现同类问题时，填写 2（高）。
  - 只有当用户明确描述大规模网络中断、大规模打印机无法使用、大规模邮件系统故障无法收发邮件等全局性或大范围故障时，才填写 1（最高）。
  - 不要仅凭单个用户的网络差、网页打不开、单点打印失败，或者“猜测可能很多人也遇到同样问题”就填写 1。
  - 如果用户描述不足以判断是否为多人同时受影响，优先保持 3，并通过 clarify_question 追问影响范围。
  - 跨用户集中爆发的升级由系统在提交前根据近期相似工单聚合结果处理，不由你自行推断填写 1。
- priority 判定规则：
  - 1=咨询：用户是在咨询业务、流程、规则、用法或其他说明性问题。
  - 2=服务：用户希望 ITSO 提供服务，例如预约会议支持、现场支持、协助配置等。
  - 3=故障：系统、网络、打印机、邮箱、账号、设备等出现异常、不可用、报错、无法连接、无法收发等故障。
  - 4=反馈：用户是在提出建议、投诉、意见或其他反馈。
  - 对“WiFi 坏了、网页打不开、连不上网、打印机报错、邮箱无法收发”这类表达，通常应判为 3（故障）。
- confidence 判定规则：
  - 当用户描述直接、明确命中上述规则时，serviceLevel_confidence 和 priority_confidence 应给高分。
  - 当需要根据较弱线索推断时，confidence 降低。
  - 如果完全无法判断，可保留空值，但要优先追问能帮助判断 serviceLevel 或 priority 的事实信息。
- 不要在 clarify_question 中要求用户直接填写 serviceLevel 或 priority，要追问能帮助系统判断这两个枚举的事实信息。
- 如果不确定，不要猜测，保留空值，并提供具体的 clarify_question。
- clarify_question、subject、othersDesc 应尽量使用与用户输入相同的语言。`,
		current.UserCode,
		current.Subject,
		current.ServiceLevel,
		current.Priority,
		current.OthersDesc,
		assistantContext,
		userInput,
	)
}

func findJSONObject(s string) (string, error) {
	// 实际模型偶尔会包 markdown fence 或额外说明，这里做最小限度的容错提取。
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("empty input")
	}

	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		return s, nil
	}

	fenceRegex := regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*?\\})\\s*```")
	fenced := fenceRegex.FindStringSubmatch(s)
	if len(fenced) == 2 {
		candidate := strings.TrimSpace(fenced[1])
		if strings.HasPrefix(candidate, "{") && strings.HasSuffix(candidate, "}") {
			return candidate, nil
		}
	}

	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return strings.TrimSpace(s[start : end+1]), nil
	}
	return "", fmt.Errorf("json object not found")
}

func normalizeServiceLevel(v string) (string, bool) {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" {
		return "", false
	}
	if _, err := strconv.Atoi(v); err == nil {
		if v >= "1" && v <= "4" {
			return v, true
		}
	}

	switch {
	case strings.Contains(v, "最高") || strings.Contains(v, "critical") || strings.Contains(v, "urgent") || v == "highest":
		return "1", true
	case strings.Contains(v, "高") || v == "high":
		return "2", true
	case strings.Contains(v, "中") || strings.Contains(v, "medium") || strings.Contains(v, "normal"):
		return "3", true
	case strings.Contains(v, "低") || strings.Contains(v, "low"):
		return "4", true
	default:
		return "", false
	}
}

func normalizePriority(v string) (string, bool) {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" {
		return "", false
	}
	if _, err := strconv.Atoi(v); err == nil {
		if v >= "1" && v <= "4" {
			return v, true
		}
	}

	switch {
	case strings.Contains(v, "咨询") || strings.Contains(v, "consult"):
		return "1", true
	case strings.Contains(v, "服务") || strings.Contains(v, "service"):
		return "2", true
	case strings.Contains(v, "故障") || strings.Contains(v, "incident") || strings.Contains(v, "bug") || strings.Contains(v, "error"):
		return "3", true
	case strings.Contains(v, "反馈") || strings.Contains(v, "feedback"):
		return "4", true
	default:
		return "", false
	}
}
