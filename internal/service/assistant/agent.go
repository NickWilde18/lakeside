package assistant

import (
	"context"
	"fmt"
	"strings"

	"lakeside/internal/service/chatmodels"
	"lakeside/internal/service/itsmagent"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/patchtoolcalls"
	"github.com/cloudwego/eino/adk/middlewares/summarization"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/gogf/gf/v2/frame/g"
)

const assistantAgentName = "it_assistant_agent"

func newRunner(ctx context.Context, store checkpointStore) (*adk.Runner, string, string, error) {
	agent, itsmAgentName, err := newAgent(ctx)
	if err != nil {
		return nil, "", "", err
	}
	return adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: false,
		CheckPointStore: store,
	}), assistantAgentName, itsmAgentName, nil
}

func newAgent(ctx context.Context) (adk.Agent, string, error) {
	itsmAgent := itsmagent.GetAgent(ctx)
	itsmAgentName := itsmAgent.Name(ctx)
	itsmTool := adk.NewAgentTool(ctx, itsmAgent)

	summaryMiddleware, err := summarization.New(ctx, &summarization.Config{
		Model: chatmodels.GetChatModel(ctx),
		Trigger: &summarization.TriggerCondition{
			ContextTokens: summaryTriggerTokens(ctx),
		},
		UserInstruction: `请对当前校园 IT 助手会话做滚动压缩，总结后续继续服务所必需的上下文。
保留重点：
- 用户当前要解决的问题与明确目标
- 尚未完成的流程、中断点、待补充信息、待确认事项
- 已确认的事实、设备信息、地点、身份线索、语言偏好
- 已经得到的工具结果或系统返回结论
不要保留：
- 寒暄、重复措辞、无关客套
- 逐字复述整段历史
输出要求：
- 使用与用户当前对话一致的语言
- 简洁、准确、可直接作为后续上下文
- 不要输出 JSON，不要加解释`,
		PreserveUserMessages: &summarization.PreserveUserMessages{
			Enabled:   true,
			MaxTokens: preservedUserMessageTokens(ctx),
		},
		Callback: func(ctx context.Context, before, after adk.ChatModelAgentState) error {
			g.Log().Infof(ctx, "assistant summarization applied, before_messages=%d after_messages=%d", len(before.Messages), len(after.Messages))
			return nil
		},
	})
	if err != nil {
		return nil, "", err
	}

	patchToolCalls, err := patchtoolcalls.New(ctx, &patchtoolcalls.Config{})
	if err != nil {
		return nil, "", err
	}

	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        assistantAgentName,
		Description: "Top-level campus IT assistant that decides whether to answer directly or use the ITSM ticketing subagent.",
		Instruction: buildAssistantInstruction(itsmAgentName),
		Model:       chatmodels.GetChatModel(ctx),
		MaxIterations: func() int {
			v := g.Cfg().MustGet(ctx, "assistant.agent.maxIterations", 6).Int()
			if v <= 0 {
				return 6
			}
			return v
		}(),
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: []tool.BaseTool{itsmTool},
			},
			ReturnDirectly: map[string]bool{
				itsmAgentName: true,
			},
			EmitInternalEvents: true,
		},
		Handlers: []adk.ChatModelAgentMiddleware{
			patchToolCalls,
			summaryMiddleware,
		},
	})
	if err != nil {
		return nil, "", err
	}
	return agent, itsmAgentName, nil
}

func buildAssistantInstruction(itsmAgentName string) string {
	return strings.TrimSpace(fmt.Sprintf(`你是校园 IT 小助手。
当前登录用户工号：{user_code}
系统已知的长期记忆与背景信息如下：
{assistant_context}

工作规则：
- 你当前已经接入的外部执行能力主要是 ITSM 工单子代理 %s。
- 当用户表达报障、维修、网络异常、WiFi 问题、账号问题、权限问题、需要人工 IT 支持、需要提交工单时，必须调用 %s。
- 调用 %s 时，忠实转述用户当前诉求，不要擅自补造地点、时间、设备或工单结果。
- 如果请求不适合走已接入能力，直接诚实说明当前首期主要支持 IT 工单创建，并给出最简短的下一步建议。
- 始终使用用户当前使用的语言回复；如果用户使用英文或其他外语，就不要切回中文。
- 不要伪造工单号、学校政策、老师答复或系统执行结果。`, itsmAgentName, itsmAgentName, itsmAgentName))
}

func summaryTriggerTokens(ctx context.Context) int {
	v := g.Cfg().MustGet(ctx, "assistant.summarization.contextTokens", 12000).Int()
	if v <= 0 {
		return 12000
	}
	return v
}

func preservedUserMessageTokens(ctx context.Context) int {
	v := g.Cfg().MustGet(ctx, "assistant.summarization.preserveUserMessageTokens", 2000).Int()
	if v <= 0 {
		return 2000
	}
	return v
}
