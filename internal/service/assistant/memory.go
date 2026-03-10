package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"

	"lakeside/internal/service/chatmodels"
)

type MemoryExtractor struct{}

type memoryOutput struct {
	Memories []MemoryItem `json:"memories"`
}

func NewMemoryExtractor() *MemoryExtractor {
	return &MemoryExtractor{}
}

func (e *MemoryExtractor) Extract(ctx context.Context, messages []MessageRecord, existing []MemoryRecord, language string) ([]MemoryItem, error) {
	startedAt := time.Now()
	if len(messages) == 0 {
		return nil, nil
	}
	prompt := fmt.Sprintf(`你是校园 IT 小助手的长期记忆提取器。
请从新增消息中提取“跨会话仍有价值”的稳定事实，只返回 JSON。

可选 category：identity, role, location, preference, relationship, device, habit, contact
提取规则：
- 只保留长期稳定、未来能帮助服务的事实
- 不要保存一次性的工单状态、临时故障、情绪化表达
- canonical_key 要稳定，便于更新同一条记忆
- content 用自然语言简洁描述
- value_json 可以是补充结构化 JSON 字符串
- confidence 取 0~1

已有记忆：%s
新增消息：
%s

返回格式：
{"memories":[{"category":"preference","canonical_key":"preferred_it_teacher","content":"用户倾向联系张老师处理网络问题","value_json":"{}","confidence":0.82}]}`,
		joinMemories(existing), formatMessages(messages))
	messagesInput := []*schema.Message{
		schema.SystemMessage(`你是长期记忆提取器，只返回 JSON。`),
		schema.UserMessage(prompt),
	}
	msg, err := chatmodels.GetChatModel(ctx).Generate(ctx, messagesInput)
	if err != nil {
		return nil, err
	}
	jsonText := extractJSONObject(msg.Content)
	var out memoryOutput
	if err := json.Unmarshal([]byte(jsonText), &out); err != nil {
		return nil, err
	}
	items := make([]MemoryItem, 0, len(out.Memories))
	for _, item := range out.Memories {
		if strings.TrimSpace(item.Category) == "" || strings.TrimSpace(item.CanonicalKey) == "" || strings.TrimSpace(item.Content) == "" {
			continue
		}
		if item.Confidence <= 0 {
			item.Confidence = 0.6
		}
		items = append(items, item)
	}
	g.Log().Infof(ctx, "assistant memory extracted, new_message_count=%d memory_count=%d duration_ms=%d", len(messages), len(items), time.Since(startedAt).Milliseconds())
	return items, nil
}

func joinMemories(memories []MemoryRecord) string {
	if len(memories) == 0 {
		return "无"
	}
	parts := make([]string, 0, len(memories))
	for _, memory := range memories {
		parts = append(parts, fmt.Sprintf("[%s/%s] %s", memory.Category, memory.CanonicalKey, memory.Content))
	}
	return strings.Join(parts, "；")
}

func runMemoryWorker(ctx context.Context, repo Repository, extractor *MemoryExtractor, jobs <-chan MemoryJob) {
	for job := range jobs {
		startedAt := time.Now()
		messages, err := repo.ListRecentMessages(ctx, job.SessionID, 20)
		if err != nil {
			g.Log().Warningf(ctx, "assistant memory worker list recent messages failed, session_id=%s err=%v", job.SessionID, err)
			continue
		}
		memories, err := repo.ListMemories(ctx, job.UserCode, 20)
		if err != nil {
			g.Log().Warningf(ctx, "assistant memory worker list memories failed, user_code=%s err=%v", job.UserCode, err)
			continue
		}
		items, err := extractor.Extract(ctx, messages, memories, job.Language)
		if err != nil {
			g.Log().Warningf(ctx, "assistant memory worker extract failed, session_id=%s err=%v", job.SessionID, err)
			continue
		}
		if len(items) == 0 {
			continue
		}
		lastID := messages[len(messages)-1].ID
		if err := repo.UpsertMemories(ctx, job.UserCode, job.SessionID, lastID, items); err != nil {
			g.Log().Warningf(ctx, "assistant memory worker upsert failed, session_id=%s err=%v", job.SessionID, err)
			continue
		}
		g.Log().Infof(ctx, "assistant memory worker completed, session_id=%s memory_count=%d duration_ms=%d", job.SessionID, len(items), time.Since(startedAt).Milliseconds())
	}
}

func formatMessages(messages []MessageRecord) string {
	var builder strings.Builder
	for _, message := range messages {
		builder.WriteString(message.Role)
		builder.WriteString(": ")
		builder.WriteString(message.Content)
		builder.WriteString("\n")
	}
	return builder.String()
}

func extractJSONObject(content string) string {
	content = strings.TrimSpace(content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		return content[start : end+1]
	}
	return `{"memories":[]}`
}
