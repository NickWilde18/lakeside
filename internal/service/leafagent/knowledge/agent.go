package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	componenttool "github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"

	"lakeside/internal/infra/ragclient"
	legacy "lakeside/internal/service/knowledgeagent"
)

type Config struct {
	Key            string
	Description    string
	KBIDs          []string
	TopK           int
	RewriteQueries int
	MaxContextDocs int
	SourceLimit    int
}

type ragAPI interface {
	Retrieve(ctx context.Context, req ragclient.RetrieveRequest) ([]ragclient.RetrievedNode, error)
	BatchGetDocuments(ctx context.Context, req ragclient.BatchGetDocumentsRequest) ([]ragclient.Document, error)
}

type queryToolInput struct {
	Request string `json:"request"`
}

// NewTool 创建绑定固定知识库集合的 Eino InvokableTool。
func NewTool(_ context.Context, cfg Config, client ragAPI, chatModel model.ToolCallingChatModel) componenttool.InvokableTool {
	inner := legacy.NewKnowledgeAgent(legacy.Config{
		Name:           cfg.Key,
		Description:    cfg.Description,
		KBIDs:          append([]string(nil), cfg.KBIDs...),
		TopK:           cfg.TopK,
		RewriteQueries: cfg.RewriteQueries,
		MaxContextDocs: cfg.MaxContextDocs,
		SourceLimit:    cfg.SourceLimit,
	}, client, chatModel)

	info := &schema.ToolInfo{
		Name: cfg.Key,
		Desc: strings.TrimSpace(cfg.Description),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"request": {
				Type:     schema.String,
				Required: true,
				Desc:     "User question to query against the bound knowledge bases.",
			},
		}),
	}

	return toolutils.NewTool(info, func(ctx context.Context, input queryToolInput) (*legacy.Result, error) {
		query := strings.TrimSpace(input.Request)
		if query == "" {
			return nil, fmt.Errorf("knowledge tool request is empty")
		}
		iter := inner.Run(ctx, &adk.AgentInput{
			Messages: []*schema.Message{schema.UserMessage(query)},
		})
		return consumeToolResult(cfg.Key, iter)
	})
}

// NewFromTool 创建一个极薄的 agent adapter，仅用于接入当前的 supervisor/sequential 编排。
func NewFromTool(key, description string, knowledgeTool componenttool.InvokableTool) adk.Agent {
	return &toolAgent{
		key:         strings.TrimSpace(key),
		description: strings.TrimSpace(description),
		tool:        knowledgeTool,
	}
}

// New 保留向后兼容：对外仍可拿到一个 leaf agent，但实际执行体已经收敛为 tool。
func New(ctx context.Context, cfg Config, client ragAPI, chatModel model.ToolCallingChatModel) adk.Agent {
	return NewFromTool(cfg.Key, cfg.Description, NewTool(ctx, cfg, client, chatModel))
}

type toolAgent struct {
	key         string
	description string
	tool        componenttool.InvokableTool
}

func (a *toolAgent) Name(_ context.Context) string {
	return a.key
}

func (a *toolAgent) Description(_ context.Context) string {
	return a.description
}

func (a *toolAgent) GetType() string {
	return "KnowledgeToolAgent"
}

func (a *toolAgent) Run(ctx context.Context, input *adk.AgentInput, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		defer gen.Close()
		if a == nil || a.tool == nil {
			gen.Send(&adk.AgentEvent{Err: fmt.Errorf("knowledge tool is nil")})
			return
		}
		query := latestQuery(ctx, input)
		if query == "" {
			gen.Send(&adk.AgentEvent{Err: fmt.Errorf("empty user message")})
			return
		}
		result, err := runTool(ctx, a.tool, a.key, query)
		if err != nil {
			gen.Send(&adk.AgentEvent{Err: err})
			return
		}
		gen.Send(finalToolEvent(a.key, result))
	}()
	return iter
}

func latestQuery(ctx context.Context, input *adk.AgentInput) string {
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
		if msg == nil || msg.Role != schema.User {
			continue
		}
		if strings.TrimSpace(msg.Content) != "" {
			return strings.TrimSpace(msg.Content)
		}
	}
	return ""
}

func runTool(ctx context.Context, knowledgeTool componenttool.InvokableTool, agentKey, query string) (*legacy.Result, error) {
	args, err := json.Marshal(queryToolInput{Request: strings.TrimSpace(query)})
	if err != nil {
		return nil, fmt.Errorf("marshal knowledge tool input failed: %w", err)
	}
	output, err := knowledgeTool.InvokableRun(ctx, string(args))
	if err != nil {
		return nil, err
	}
	result := &legacy.Result{}
	if err := json.Unmarshal([]byte(output), result); err != nil {
		return nil, fmt.Errorf("decode knowledge tool output failed: %w", err)
	}
	if strings.TrimSpace(result.AgentName) == "" {
		result.AgentName = strings.TrimSpace(agentKey)
	}
	if strings.TrimSpace(result.Message) == "" {
		return nil, fmt.Errorf("knowledge tool returned empty message")
	}
	return result, nil
}

func consumeToolResult(agentKey string, iter *adk.AsyncIterator[*adk.AgentEvent]) (*legacy.Result, error) {
	var (
		result      *legacy.Result
		lastMessage string
	)
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			return nil, event.Err
		}
		if event.Output != nil && event.Output.CustomizedOutput != nil {
			if out := legacy.ResultFromAny(event.Output.CustomizedOutput); out != nil {
				copied := *out
				if strings.TrimSpace(copied.AgentName) == "" {
					copied.AgentName = strings.TrimSpace(agentKey)
				}
				result = &copied
			}
		}
		msg, _, err := adk.GetMessage(event)
		if err != nil {
			return nil, err
		}
		if msg != nil && strings.TrimSpace(msg.Content) != "" {
			lastMessage = strings.TrimSpace(msg.Content)
		}
	}
	if result != nil {
		return result, nil
	}
	if lastMessage == "" {
		return nil, fmt.Errorf("knowledge tool produced no output")
	}
	return &legacy.Result{
		AgentName: strings.TrimSpace(agentKey),
		Success:   true,
		Message:   lastMessage,
	}, nil
}

func finalToolEvent(agentKey string, result *legacy.Result) *adk.AgentEvent {
	msg := schema.AssistantMessage(strings.TrimSpace(result.Message), nil)
	return &adk.AgentEvent{
		AgentName: strings.TrimSpace(agentKey),
		Output: &adk.AgentOutput{
			MessageOutput: &adk.MessageVariant{
				IsStreaming: false,
				Message:     msg,
				Role:        schema.Assistant,
			},
			CustomizedOutput: result,
		},
	}
}
