package knowledgeagent

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	componentretriever "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

type fakeRetriever struct {
	docs []*schema.Document
	err  error
}

func (f *fakeRetriever) Retrieve(_ context.Context, _ string, _ ...componentretriever.Option) ([]*schema.Document, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.docs, nil
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

func TestMergeDocumentsRoundRobin(t *testing.T) {
	t.Parallel()

	docs := mergeDocumentsRoundRobin([]string{"kb-a", "kb-b"}, map[string][]*schema.Document{
		"kb-a": {&schema.Document{ID: "a1"}, &schema.Document{ID: "a2"}},
		"kb-b": {&schema.Document{ID: "b1"}, &schema.Document{ID: "b2"}},
	}, 3)
	require.Len(t, docs, 3)
	require.Equal(t, "a1", docs[0].ID)
	require.Equal(t, "b1", docs[1].ID)
	require.Equal(t, "a2", docs[2].ID)
}

func TestKnowledgeAgentRunReturnsSources(t *testing.T) {
	t.Parallel()

	doc := (&schema.Document{
		ID:      "node-1",
		Content: "先连接学校 VPN。",
		MetaData: map[string]any{
			metaKBID:     "kb-it",
			metaDocID:    "doc-1",
			metaNodeID:   "node-1",
			metaFilename: "vpn-user-guide.md",
		},
	}).WithScore(0.92)

	agent := &knowledgeAgent{
		name:           "campus_it_kb",
		description:    "test",
		maxContextDocs: 2,
		sourceLimit:    2,
		retriever: &fakeRetriever{docs: []*schema.Document{
			doc,
		}},
		chatModel: &fakeChatModel{content: "请先连接学校 VPN，再访问校内资源。"},
	}

	iter := agent.Run(context.Background(), &adk.AgentInput{Messages: []*schema.Message{schema.UserMessage("怎么访问校内资源？")}})
	event, ok := iter.Next()
	require.True(t, ok)
	require.NotNil(t, event)
	require.NotNil(t, event.Output)
	result := ResultFromAny(event.Output.CustomizedOutput)
	require.NotNil(t, result)
	require.True(t, result.Success)
	require.Len(t, result.Sources, 1)
	require.Equal(t, "kb-it", result.Sources[0].KBID)
	require.Equal(t, "先连接学校 VPN。", result.Sources[0].Snippet)
	require.Equal(t, "请先连接学校 VPN，再访问校内资源。", result.Message)
	require.NotContains(t, result.Message, "参考来源")
	require.NotContains(t, result.Message, "node-1")
}

func TestKnowledgeAgentRunReturnsFailureOnEmptyRetrieve(t *testing.T) {
	t.Parallel()

	agent := &knowledgeAgent{
		name:        "campus_it_kb",
		description: "test",
		retriever:   &fakeRetriever{},
		chatModel:   &fakeChatModel{content: "unused"},
	}

	iter := agent.Run(context.Background(), &adk.AgentInput{Messages: []*schema.Message{schema.UserMessage("What is VPN?")}})
	event, ok := iter.Next()
	require.True(t, ok)
	result := ResultFromAny(event.Output.CustomizedOutput)
	require.NotNil(t, result)
	require.False(t, result.Success)
	require.Contains(t, result.Message, "knowledge base")
}

func TestNormalizeAssistantTextCollapsesPunctuationBurst(t *testing.T) {
	t.Parallel()

	got := normalizeAssistantText("您好!!!!!!!!!!!!!!!!??????????")
	require.Equal(t, "您好!!!???", got)
}
