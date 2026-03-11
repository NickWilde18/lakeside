package rootassistant

import (
	"context"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/patchtoolcalls"
	"github.com/cloudwego/eino/adk/middlewares/summarization"
	supervisoragent "github.com/cloudwego/eino/adk/prebuilt/supervisor"
	"github.com/gogf/gf/v2/frame/g"

	"lakeside/internal/service/agentmiddleware"
	"lakeside/internal/service/chatmodels"
)

// New 创建顶层助手。
//
// 顶层助手现在直接使用 Eino 官方 prebuilt/supervisor，
// 由一个 ChatModelAgent 作为 supervisor，统一协调下级领域助手。
func New(ctx context.Context, key, description, instruction string, maxIterations int, subAgents []adk.Agent) (adk.ResumableAgent, error) {
	patchToolCalls, err := patchtoolcalls.New(ctx, &patchtoolcalls.Config{})
	if err != nil {
		return nil, err
	}
	summaryMiddleware, err := summarization.New(ctx, &summarization.Config{
		Model: chatmodels.GetChatModel(ctx),
		Trigger: &summarization.TriggerCondition{
			ContextTokens: summarizationContextTokens(ctx),
		},
		PreserveUserMessages: &summarization.PreserveUserMessages{
			Enabled:   true,
			MaxTokens: summarizationPreserveTokens(ctx),
		},
	})
	if err != nil {
		return nil, err
	}

	supervisorModel, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          key,
		Description:   description,
		Instruction:   instruction,
		Model:         chatmodels.GetChatModel(ctx),
		MaxIterations: normalizeMaxIterations(maxIterations),
		Handlers: []adk.ChatModelAgentMiddleware{
			agentmiddleware.NewInstructionTemplateMiddleware(),
			patchToolCalls,
			summaryMiddleware,
		},
	})
	if err != nil {
		return nil, err
	}

	return supervisoragent.New(ctx, &supervisoragent.Config{
		Supervisor: supervisorModel,
		SubAgents:  subAgents,
	})
}

func normalizeMaxIterations(v int) int {
	if v <= 0 {
		return 6
	}
	return v
}

func summarizationContextTokens(ctx context.Context) int {
	v := g.Cfg().MustGet(ctx, "agents.summarization.contextTokens", 12000).Int()
	if v <= 0 {
		return 12000
	}
	return v
}

func summarizationPreserveTokens(ctx context.Context) int {
	v := g.Cfg().MustGet(ctx, "agents.summarization.preserveUserMessageTokens", 2000).Int()
	if v <= 0 {
		return 2000
	}
	return v
}
