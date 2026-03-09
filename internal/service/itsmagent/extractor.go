package itsmagent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/schema"

	"lakeside/internal/service/chatmodels"
)

type Extractor struct{}

func NewExtractor() *Extractor {
	return &Extractor{}
}

func (e *Extractor) FillDraft(ctx context.Context, draft TicketDraft, userInput string) (TicketDraft, string, error) {
	res, err := e.Extract(ctx, userInput, draft)
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

func (e *Extractor) Extract(ctx context.Context, userInput string, current TicketDraft) (*ExtractResult, error) {
	prompt := buildExtractPrompt(current, userInput)
	messages := []*schema.Message{
		schema.SystemMessage(`You are an ITSM ticket field extractor. Return ONLY a JSON object. No markdown, no explanation.`),
		schema.UserMessage(prompt),
	}

	msg, err := chatmodels.GetChatModel(ctx).Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("extract fields generate failed: %w", err)
	}
	if msg == nil || strings.TrimSpace(msg.Content) == "" {
		return nil, fmt.Errorf("extract fields got empty model output")
	}

	jsonText, err := findJSONObject(msg.Content)
	if err != nil {
		return nil, fmt.Errorf("extract fields response has no json object: %w", err)
	}

	var out ExtractResult
	if err := json.Unmarshal([]byte(jsonText), &out); err != nil {
		return nil, fmt.Errorf("extract fields decode failed: %w", err)
	}
	return &out, nil
}

func buildExtractPrompt(current TicketDraft, userInput string) string {
	return fmt.Sprintf(`Task: Extract ITSM ticket fields from user input.

Current draft JSON:
{"userCode":%q,"subject":%q,"serviceLevel":%q,"priority":%q,"othersDesc":%q}

User input:
%q

Output JSON schema exactly:
{
  "subject": "string",
  "othersDesc": "string",
  "serviceLevel": "1|2|3|4|\"\"",
  "serviceLevel_confidence": 0.0,
  "priority": "1|2|3|4|\"\"",
  "priority_confidence": 0.0,
  "clarify_question": "string"
}

Rules:
- subject/othersDesc should be concise and useful.
- serviceLevel mapping: 1=highest,2=high,3=medium,4=low.
- priority mapping: 1=consultation,2=service,3=incident,4=feedback.
- If uncertain, keep field empty and provide clarify_question.
- Output JSON only.`,
		current.UserCode,
		current.Subject,
		current.ServiceLevel,
		current.Priority,
		current.OthersDesc,
		userInput,
	)
}

func findJSONObject(s string) (string, error) {
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
