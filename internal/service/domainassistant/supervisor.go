package domainassistant

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

// LeafBinding 描述一个可供领域助手调度的叶子 agent。
type LeafBinding struct {
	Key           string
	Description   string
	Kind          string
	Interruptible bool
	Agent         adk.Agent
}

// New 创建领域助手。
//
// 领域层优先使用“计划器 + 官方 SequentialAgent”的执行方式：
// 1. 先让模型产出结构化的叶子执行计划；
// 2. 再由代码按计划顺序执行叶子 agent；
// 3. 当计划器异常或无法判断时，回退到官方 supervisor 兜底。
func New(ctx context.Context, key, description, instruction string, maxIterations int, subAgents []adk.Agent, leaves []LeafBinding) (adk.ResumableAgent, error) {
	fallback, err := newSupervisor(ctx, key, description, instruction, maxIterations, subAgents)
	if err != nil {
		return nil, err
	}
	if len(leaves) == 0 {
		return fallback, nil
	}
	return newPlannedAgent(ctx, key, description, instruction, leaves, fallback)
}

func newSupervisor(ctx context.Context, key, description, instruction string, maxIterations int, subAgents []adk.Agent) (adk.ResumableAgent, error) {
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
