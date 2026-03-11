package knowledgeagent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	componentretriever "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"

	"lakeside/internal/infra/ragclient"
	"lakeside/internal/service/agentplatform/eventctx"
)

func (r *ragRetriever) Retrieve(ctx context.Context, query string, opts ...componentretriever.Option) ([]*schema.Document, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("knowledge retriever is nil")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("knowledge query is empty")
	}
	userUPN := currentUserUPN(ctx)
	if userUPN == "" {
		return nil, fmt.Errorf("missing X-User-ID")
	}

	common := componentretriever.GetCommonOptions(&componentretriever.Options{}, opts...)
	topK := r.defaultTopK
	if common.TopK != nil && *common.TopK > 0 {
		topK = *common.TopK
	}
	if topK <= 0 {
		topK = 5
	}

	docsByKB := make(map[string][]*schema.Document, len(r.kbIDs))
	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		firstErr error
	)

	for _, kbID := range r.kbIDs {
		kbID := strings.TrimSpace(kbID)
		if kbID == "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			startedAt := time.Now()
			eventctx.EmitForNode(ctx, "knowledge_retrieve_started", r.agentName, "开始检索知识库", map[string]any{
				"kb_id":  kbID,
				"query":  query,
				"top_k":  topK,
				"agent":  r.agentName,
				"user_upn": userUPN,
			})
			g.Log().Debugf(ctx, "knowledge retrieve dispatched, user_upn=%s kb_id=%s top_k=%d query=%q", userUPN, kbID, topK, query)
			nodes, err := r.client.Retrieve(ctx, ragclient.RetrieveRequest{
				UserUPN: userUPN,
				KBID:    kbID,
				Query:   query,
				TopK:    topK,
			})
			eventPayload := map[string]any{
				"kb_id":       kbID,
				"query":       query,
				"top_k":       topK,
				"agent":       r.agentName,
				"user_upn":    userUPN,
				"duration_ms": time.Since(startedAt).Milliseconds(),
			}
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				eventPayload["error"] = err.Error()
				eventctx.EmitForNode(ctx, "knowledge_retrieve_finished", r.agentName, "知识库检索失败", eventPayload)
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			eventPayload["hit_count"] = len(nodes)
			eventctx.EmitForNode(ctx, "knowledge_retrieve_finished", r.agentName, "知识库检索完成", eventPayload)
			docsByKB[kbID] = nodesToDocuments(nodes)
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return mergeDocumentsRoundRobin(r.kbIDs, docsByKB, topK), nil
}

func nodesToDocuments(nodes []ragclient.RetrievedNode) []*schema.Document {
	docs := make([]*schema.Document, 0, len(nodes))
	for _, node := range nodes {
		doc := &schema.Document{
			ID:      strings.TrimSpace(node.NodeID),
			Content: strings.TrimSpace(node.Text),
			MetaData: map[string]any{
				metaKBID:          strings.TrimSpace(node.KBID),
				metaDocID:         strings.TrimSpace(node.DocID),
				metaNodeID:        strings.TrimSpace(node.NodeID),
				metaNamespaceUUID: strings.TrimSpace(node.NamespaceUUID),
			},
		}
		doc.WithScore(node.Score)
		docs = append(docs, doc)
	}
	return docs
}

func mergeDocumentsRoundRobin(order []string, docsByKB map[string][]*schema.Document, limit int) []*schema.Document {
	if limit <= 0 {
		limit = 5
	}
	indexes := make(map[string]int, len(order))
	out := make([]*schema.Document, 0, limit)
	for len(out) < limit {
		advanced := false
		for _, kbID := range order {
			items := docsByKB[strings.TrimSpace(kbID)]
			idx := indexes[kbID]
			if idx >= len(items) {
				continue
			}
			out = append(out, items[idx])
			indexes[kbID] = idx + 1
			advanced = true
			if len(out) >= limit {
				return out
			}
		}
		if !advanced {
			break
		}
	}
	return out
}
