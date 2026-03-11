package knowledgeagent

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/prompt"
	componentretriever "github.com/cloudwego/eino/components/retriever"
	flowmultiquery "github.com/cloudwego/eino/flow/retriever/multiquery"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"

	"lakeside/internal/infra/ragclient"
)

// NewKnowledgeAgent 创建一个绑定固定 kb_id 集合的知识子代理。
func NewKnowledgeAgent(cfg Config, client ragAPI, chatModel model.ToolCallingChatModel) adk.Agent {
	baseRetriever := &ragRetriever{
		agentName:   cfg.Name,
		client:      client,
		kbIDs:       append([]string(nil), cfg.KBIDs...),
		defaultTopK: cfg.TopK,
	}
	return &knowledgeAgent{
		name:           cfg.Name,
		description:    cfg.Description,
		kbIDs:          append([]string(nil), cfg.KBIDs...),
		defaultTopK:    cfg.TopK,
		rewriteQueries: cfg.RewriteQueries,
		maxContextDocs: cfg.MaxContextDocs,
		sourceLimit:    cfg.SourceLimit,
		retriever:      buildKnowledgeRetriever(cfg, baseRetriever, chatModel),
		ragClient:      client,
		chatModel:      chatModel,
	}
}

func buildKnowledgeRetriever(cfg Config, base componentretriever.Retriever, chatModel model.ToolCallingChatModel) componentretriever.Retriever {
	if base == nil {
		return nil
	}
	if cfg.RewriteQueries <= 1 || chatModel == nil {
		return base
	}
	rewriteModel, ok := chatModel.(model.ChatModel)
	if !ok {
		return base
	}
	retriever, err := flowmultiquery.NewRetriever(context.Background(), &flowmultiquery.Config{
		RewriteLLM: rewriteModel,
		RewriteTemplate: prompt.FromMessages(schema.Jinja2, schema.UserMessage(`你是知识检索查询改写助手。
请基于用户问题生成多个不同但互补的检索查询，以提高知识库召回率。
要求：
- 可以拆分并列子问题
- 可以补充常见同义词、别名、缩写或更具体的关键词
- 不要回答问题
- 每行只输出一个检索查询
- 不要编号
用户问题：{{query}}`)),
		QueryVar:      "query",
		MaxQueriesNum: cfg.RewriteQueries,
		LLMOutputParser: func(_ context.Context, message *schema.Message) ([]string, error) {
			if message == nil {
				return nil, nil
			}
			lines := strings.Split(message.Content, "\n")
			out := make([]string, 0, len(lines))
			seen := make(map[string]struct{}, len(lines))
			for _, line := range lines {
				line = strings.TrimSpace(strings.TrimLeft(line, "-*0123456789. "))
				if line == "" {
					continue
				}
				if _, ok := seen[line]; ok {
					continue
				}
				seen[line] = struct{}{}
				out = append(out, line)
			}
			return out, nil
		},
		OrigRetriever: base,
	})
	if err != nil {
		g.Log().Warningf(context.Background(), "init knowledge multiquery retriever failed, fallback to single retrieve: %v", err)
		return base
	}
	return retriever
}

func (a *knowledgeAgent) Name(_ context.Context) string {
	return a.name
}

func (a *knowledgeAgent) Description(_ context.Context) string {
	return a.description
}

func (a *knowledgeAgent) Run(ctx context.Context, input *adk.AgentInput, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	query := sessionString(ctx, "latest_user_message")
	if strings.TrimSpace(query) == "" {
		query = latestUserMessage(input)
	}
	if strings.TrimSpace(query) == "" {
		return singleEventIter(&adk.AgentEvent{Err: fmt.Errorf("empty user message")})
	}
	lang := detectUserLanguage(query)

	docs, err := a.retriever.Retrieve(ctx, query, componentretriever.WithTopK(a.defaultTopK))
	if err != nil {
		message := localizeText(lang, "知识库检索暂时不可用，请稍后再试。", "The knowledge retrieval service is temporarily unavailable. Please try again later.")
		return singleEventIter(finalKnowledgeEvent(message, &Result{AgentName: a.name, Success: false, Message: message}))
	}
	if len(docs) == 0 {
		message := localizeText(lang, "当前知识库没有找到足够依据来回答这个问题。", "The current knowledge base does not contain enough evidence to answer this question.")
		return singleEventIter(finalKnowledgeEvent(message, &Result{AgentName: a.name, Success: false, Message: message}))
	}

	userUPN := currentUserUPN(ctx)
	a.attachFilenames(ctx, userUPN, docs)
	contextDocs := truncateDocuments(docs, a.maxContextDocs)
	answerBody, err := a.generateAnswer(ctx, query, lang, contextDocs)
	if err != nil {
		message := localizeText(lang, "知识回答生成失败，请稍后再试。", "Failed to generate a knowledge-based answer. Please try again later.")
		return singleEventIter(finalKnowledgeEvent(message, &Result{AgentName: a.name, Success: false, Message: message}))
	}
	sources := sourcesFromDocuments(docs, a.sourceLimit)
	message := formatAnswerWithSources(strings.TrimSpace(answerBody), sources, lang)
	return singleEventIter(finalKnowledgeEvent(message, &Result{AgentName: a.name, Success: true, Message: message, Sources: sources}))
}

func (a *knowledgeAgent) attachFilenames(ctx context.Context, userUPN string, docs []*schema.Document) {
	if a == nil || a.ragClient == nil || strings.TrimSpace(userUPN) == "" || len(docs) == 0 {
		return
	}
	docIDsByKB := make(map[string][]string)
	seen := make(map[string]struct{})
	for _, doc := range docs {
		kbID := metadataString(doc, metaKBID)
		docID := metadataString(doc, metaDocID)
		if kbID == "" || docID == "" {
			continue
		}
		key := kbID + ":" + docID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		docIDsByKB[kbID] = append(docIDsByKB[kbID], docID)
	}
	filenames := make(map[string]string)
	for kbID, docIDs := range docIDsByKB {
		items, err := a.ragClient.BatchGetDocuments(ctx, ragclient.BatchGetDocumentsRequest{UserUPN: userUPN, KBID: kbID, DocIDs: docIDs})
		if err != nil {
			g.Log().Warningf(ctx, "knowledge batch_get failed, agent=%s kb_id=%s err=%v", a.name, kbID, err)
			continue
		}
		for _, item := range items {
			if strings.TrimSpace(item.ID) == "" {
				continue
			}
			filenames[kbID+":"+strings.TrimSpace(item.ID)] = strings.TrimSpace(item.Filename)
		}
	}
	for _, doc := range docs {
		kbID := metadataString(doc, metaKBID)
		docID := metadataString(doc, metaDocID)
		if kbID == "" || docID == "" {
			continue
		}
		if name := strings.TrimSpace(filenames[kbID+":"+docID]); name != "" {
			if doc.MetaData == nil {
				doc.MetaData = make(map[string]any)
			}
			doc.MetaData[metaFilename] = name
		}
	}
}

func (a *knowledgeAgent) generateAnswer(ctx context.Context, query, lang string, docs []*schema.Document) (string, error) {
	if a == nil || a.chatModel == nil {
		return "", fmt.Errorf("knowledge chat model is nil")
	}
	messages := []*schema.Message{
		schema.SystemMessage(localizeText(lang,
			"你是校园 IT 知识库助手。你只能基于提供的知识片段回答问题，不要编造制度、流程、时间、联系方式或结论。若证据不足，直接说明当前知识库没有足够依据。你只回答当前知识片段能够支持的那部分内容，不要主动扩展到工单、报修、人工支持或后续步骤。不要追问，不要邀请用户继续提问，不要说“如需进一步帮助我可以……”。只输出回答正文，不要自行追加引用列表。",
			"You are a campus IT knowledge base assistant. Answer only from the provided passages. Do not invent policies, steps, timelines, contacts, or conclusions. If the evidence is insufficient, say the current knowledge base does not contain enough evidence. Answer only the part supported by the passages. Do not proactively expand into ticket submission, manual support, or next-step workflow. Do not ask follow-up questions, do not invite the user to continue, and do not say things like 'I can help further'. Output only the answer body and do not append your own citation list.")),
		schema.UserMessage(buildKnowledgePrompt(query, docs, lang)),
	}
	msg, err := a.chatModel.Generate(ctx, messages)
	if err != nil {
		return "", err
	}
	if msg == nil || strings.TrimSpace(msg.Content) == "" {
		return "", fmt.Errorf("knowledge model returned empty content")
	}
	return strings.TrimSpace(msg.Content), nil
}

func buildKnowledgePrompt(query string, docs []*schema.Document, lang string) string {
	var builder strings.Builder
	builder.WriteString(localizeText(lang, "用户问题：\n", "User question:\n"))
	builder.WriteString(strings.TrimSpace(query))
	builder.WriteString("\n\n")
	builder.WriteString(localizeText(lang, "知识片段：\n", "Knowledge passages:\n"))
	for i, doc := range docs {
		builder.WriteString(fmt.Sprintf("[%d] ", i+1))
		filename := metadataString(doc, metaFilename)
		docID := metadataString(doc, metaDocID)
		nodeID := metadataString(doc, metaNodeID)
		if filename != "" {
			builder.WriteString(fmt.Sprintf("(%s, doc_id=%s, node_id=%s)\n", filename, docID, nodeID))
		} else {
			builder.WriteString(fmt.Sprintf("(doc_id=%s, node_id=%s)\n", docID, nodeID))
		}
		builder.WriteString(strings.TrimSpace(doc.Content))
		builder.WriteString("\n\n")
	}
	builder.WriteString(localizeText(lang, "请基于以上内容直接回答。", "Please answer directly based on the passages above."))
	return builder.String()
}

func truncateDocuments(docs []*schema.Document, limit int) []*schema.Document {
	if limit <= 0 || len(docs) <= limit {
		return docs
	}
	out := make([]*schema.Document, 0, limit)
	out = append(out, docs[:limit]...)
	return out
}

func sourcesFromDocuments(docs []*schema.Document, limit int) []Source {
	if limit <= 0 {
		limit = 4
	}
	out := make([]Source, 0, limit)
	for _, doc := range docs {
		if len(out) >= limit {
			break
		}
		out = append(out, Source{
			KBID:     metadataString(doc, metaKBID),
			DocID:    metadataString(doc, metaDocID),
			NodeID:   metadataString(doc, metaNodeID),
			Filename: metadataString(doc, metaFilename),
			Snippet:  buildSnippet(doc.Content),
			Score:    doc.Score(),
		})
	}
	return out
}

func buildSnippet(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	content = strings.Join(strings.Fields(content), " ")
	const maxRunes = 160
	runes := []rune(content)
	if len(runes) <= maxRunes {
		return content
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "..."
}

func formatAnswerWithSources(answer string, sources []Source, lang string) string {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		answer = localizeText(lang, "当前知识库没有足够依据来回答这个问题。", "The current knowledge base does not contain enough evidence to answer this question.")
	}
	if len(sources) == 0 {
		return answer
	}
	var builder strings.Builder
	builder.WriteString(answer)
	builder.WriteString("\n\n")
	builder.WriteString(localizeText(lang, "参考来源：\n", "Sources:\n"))
	for i, source := range sources {
		label := source.Filename
		if strings.TrimSpace(label) == "" {
			label = source.DocID
		}
		builder.WriteString(fmt.Sprintf("%d. %s (kb=%s, node=%s)", i+1, label, source.KBID, source.NodeID))
		if i < len(sources)-1 {
			builder.WriteString("\n")
		}
	}
	return builder.String()
}

func metadataString(doc *schema.Document, key string) string {
	if doc == nil || doc.MetaData == nil {
		return ""
	}
	value, _ := doc.MetaData[key].(string)
	return strings.TrimSpace(value)
}

func finalKnowledgeEvent(text string, result *Result) *adk.AgentEvent {
	msg := schema.AssistantMessage(strings.TrimSpace(text), nil)
	return &adk.AgentEvent{
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
