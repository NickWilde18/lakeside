package knowledge

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	componenttool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"

	"lakeside/internal/infra/ragclient"
	legacy "lakeside/internal/service/knowledgeagent"
)

type fakeRAGClient struct {
	nodes []ragclient.RetrievedNode
	docs  []ragclient.Document
	err   error
}

func (f *fakeRAGClient) Retrieve(_ context.Context, _ ragclient.RetrieveRequest) ([]ragclient.RetrievedNode, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]ragclient.RetrievedNode(nil), f.nodes...), nil
}

func (f *fakeRAGClient) BatchGetDocuments(_ context.Context, _ ragclient.BatchGetDocumentsRequest) ([]ragclient.Document, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]ragclient.Document(nil), f.docs...), nil
}

type fakeChatModel struct {
	content string
}

func (f *fakeChatModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return schema.AssistantMessage(f.content, nil), nil
}

func (f *fakeChatModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}

func (f *fakeChatModel) WithTools(_ []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return f, nil
}

type fakeTool struct {
	output string
	err    error
}

func (f *fakeTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name:        "fake_knowledge_tool",
		Desc:        "fake",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{}),
	}, nil
}

func (f *fakeTool) InvokableRun(_ context.Context, _ string, _ ...componenttool.Option) (string, error) {
	return f.output, f.err
}

func TestKnowledgeToolBackedAgentRunReturnsSources(t *testing.T) {
	t.Parallel()

	agent := New(context.Background(), Config{
		Key:            "campus_it_kb",
		Description:    "校园 IT 知识查询",
		KBIDs:          []string{"kb-it"},
		TopK:           2,
		RewriteQueries: 1,
		MaxContextDocs: 2,
		SourceLimit:    2,
	}, &fakeRAGClient{
		nodes: []ragclient.RetrievedNode{
			{
				KBID:   "kb-it",
				DocID:  "doc-1",
				NodeID: "node-1",
				Text:   "先连接学校 VPN。",
				Score:  0.92,
			},
		},
		docs: []ragclient.Document{
			{ID: "doc-1", Filename: "vpn-user-guide.md"},
		},
	}, &fakeChatModel{content: "请先连接学校 VPN，再访问校内资源。"})

	runner := adk.NewRunner(context.Background(), adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: false,
	})

	iter := runner.Query(context.Background(), "怎么访问校内资源？",
		adk.WithSessionValues(map[string]any{"user_upn": "122020255@link.cuhk.edu.cn"}))

	var result *legacy.Result
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		require.NoError(t, event.Err)
		if event.Output == nil || event.Output.CustomizedOutput == nil {
			continue
		}
		result = legacy.ResultFromAny(event.Output.CustomizedOutput)
	}

	require.NotNil(t, result)
	require.True(t, result.Success)
	require.Equal(t, "campus_it_kb", result.AgentName)
	require.Equal(t, "请先连接学校 VPN，再访问校内资源。", result.Message)
	require.Len(t, result.Sources, 1)
	require.Equal(t, "kb-it", result.Sources[0].KBID)
	require.Equal(t, "vpn-user-guide.md", result.Sources[0].Filename)
}

func TestKnowledgeToolBackedAgentRunReturnsErrorOnInvalidToolOutput(t *testing.T) {
	t.Parallel()

	agent := NewFromTool("campus_it_kb", "校园 IT 知识查询", &fakeTool{output: "not-json"})
	runner := adk.NewRunner(context.Background(), adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: false,
	})

	iter := runner.Query(context.Background(), "VPN 怎么连接？")
	event, ok := iter.Next()
	require.True(t, ok)
	require.NotNil(t, event)
	require.Error(t, event.Err)
	require.Contains(t, event.Err.Error(), "decode knowledge tool output failed")
}
