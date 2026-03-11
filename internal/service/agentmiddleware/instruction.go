package agentmiddleware

import (
	"context"
	"strings"

	"github.com/cloudwego/eino/adk"
)

// instructionTemplateMiddleware 在每次运行前，把 instruction 模板中的占位符替换为当前会话值。
//
// 当前支持：
// - {assistant_key}
// - {user_upn}
// - {assistant_context}
// - {latest_user_message}
type instructionTemplateMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
}

// NewInstructionTemplateMiddleware 创建 instruction 模板替换中间件。
func NewInstructionTemplateMiddleware() adk.ChatModelAgentMiddleware {
	return &instructionTemplateMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
	}
}

func (m *instructionTemplateMiddleware) BeforeAgent(ctx context.Context, runCtx *adk.ChatModelAgentContext) (context.Context, *adk.ChatModelAgentContext, error) {
	if runCtx == nil || strings.TrimSpace(runCtx.Instruction) == "" {
		return ctx, runCtx, nil
	}
	runCtx.Instruction = renderInstructionTemplate(runCtx.Instruction, map[string]string{
		"assistant_key":       sessionValueString(ctx, "assistant_key"),
		"user_upn":            sessionValueString(ctx, "user_upn"),
		"assistant_context":   sessionValueString(ctx, "assistant_context"),
		"latest_user_message": sessionValueString(ctx, "latest_user_message"),
	})
	return ctx, runCtx, nil
}

func renderInstructionTemplate(template string, values map[string]string) string {
	template = strings.TrimSpace(template)
	if template == "" || len(values) == 0 {
		return template
	}
	replacerArgs := make([]string, 0, len(values)*2)
	for key, value := range values {
		replacerArgs = append(replacerArgs, "{"+strings.TrimSpace(key)+"}", strings.TrimSpace(value))
	}
	return strings.NewReplacer(replacerArgs...).Replace(template)
}

func sessionValueString(ctx context.Context, key string) string {
	value, ok := adk.GetSessionValue(ctx, key)
	if !ok || value == nil {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}
