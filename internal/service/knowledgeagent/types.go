package knowledgeagent

import (
	"context"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	componentretriever "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"

	"lakeside/internal/infra/ragclient"
)

const (
	metaKBID          = "kb_id"
	metaDocID         = "doc_id"
	metaNodeID        = "node_id"
	metaNamespaceUUID = "namespace_uuid"
	metaFilename      = "filename"
)

// Config 定义单个知识子代理配置。
type Config struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	KBIDs          []string `json:"kbIDs"`
	TopK           int      `json:"topK"`
	RewriteQueries int      `json:"rewriteQueries"`
	MaxContextDocs int      `json:"maxContextDocs"`
	SourceLimit    int      `json:"sourceLimit"`
}

// AgentInfo 表示 knowledge subagent 的对外说明。
type AgentInfo struct {
	Name        string
	Description string
}

// Source 表示知识回答引用的来源。
type Source struct {
	KBID     string  `json:"kb_id"`
	DocID    string  `json:"doc_id"`
	NodeID   string  `json:"node_id"`
	Filename string  `json:"filename,omitempty"`
	Snippet  string  `json:"snippet,omitempty"`
	Score    float64 `json:"score,omitempty"`
}

// Result 表示 knowledge subagent 的最终输出。
type Result struct {
	AgentName string   `json:"agent_name,omitempty"`
	Success   bool     `json:"success"`
	Message   string   `json:"message"`
	Sources   []Source `json:"sources,omitempty"`
}

type ragAPI interface {
	Retrieve(ctx context.Context, req ragclient.RetrieveRequest) ([]ragclient.RetrievedNode, error)
	BatchGetDocuments(ctx context.Context, req ragclient.BatchGetDocumentsRequest) ([]ragclient.Document, error)
}

type knowledgeAgent struct {
	name           string
	description    string
	kbIDs          []string
	defaultTopK    int
	rewriteQueries int
	maxContextDocs int
	sourceLimit    int
	retriever      componentretriever.Retriever
	ragClient      ragAPI
	chatModel      model.ToolCallingChatModel
}

type ragRetriever struct {
	agentName   string
	client      ragAPI
	kbIDs       []string
	defaultTopK int
}

// Registry 保存当前所有 knowledge subagent 实例。
type Registry struct {
	agents []adk.Agent
	infos  []AgentInfo
}

// ResultFromAny 从 ADK CustomizedOutput 中取回 knowledge 结果。
func ResultFromAny(v any) *Result {
	switch out := v.(type) {
	case *Result:
		return out
	case Result:
		copied := out
		return &copied
	default:
		return nil
	}
}

func singleEventIter(event *adk.AgentEvent) *adk.AsyncIterator[*adk.AgentEvent] {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		gen.Send(event)
		gen.Close()
	}()
	return iter
}

func latestUserMessage(input *adk.AgentInput) string {
	if input == nil || len(input.Messages) == 0 {
		return ""
	}
	for i := len(input.Messages) - 1; i >= 0; i-- {
		m := input.Messages[i]
		if m == nil {
			continue
		}
		if m.Role == schema.User {
			return strings.TrimSpace(m.Content)
		}
	}
	return strings.TrimSpace(input.Messages[len(input.Messages)-1].Content)
}

func sessionString(ctx context.Context, key string) string {
	v, ok := adk.GetSessionValue(ctx, key)
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func currentUserUPN(ctx context.Context) string {
	if upn := sessionString(ctx, "user_upn"); upn != "" {
		return upn
	}
	return sessionString(ctx, "user_code")
}

func detectUserLanguage(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "zh"
	}
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FFF {
			return "zh"
		}
	}
	return "en"
}

func isChineseLanguage(lang string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "zh")
}

func localizeText(lang, zh, en string) string {
	if isChineseLanguage(lang) {
		return zh
	}
	return en
}
