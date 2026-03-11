package agent

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/frame/g"

	v1 "lakeside/api/agent/v1"
	"lakeside/internal/service/agentplatform"
)

func (c *ControllerV1) AgentMemories(ctx context.Context, req *v1.AgentMemoriesReq) (res *v1.AgentMemoriesRes, err error) {
	g.Log().Infof(ctx, "agent memories request received, assistant_key=%s user_upn=%s limit=%d", req.AssistantKey, req.UserID, req.Limit)
	items, err := agentplatform.GetService(ctx).ListMemories(ctx, &agentplatform.ListMemoriesRequest{
		AssistantKey: req.AssistantKey,
		UserUPN:      req.UserID,
		Limit:        req.Limit,
	})
	if err != nil {
		return nil, err
	}
	result := make([]v1.AgentMemory, 0, len(items))
	for _, item := range items {
		result = append(result, v1.AgentMemory{
			ID:              item.ID,
			Category:        item.Category,
			CanonicalKey:    item.CanonicalKey,
			Content:         item.Content,
			ValueJSON:       item.ValueJSON,
			Confidence:      item.Confidence,
			SourceSessionID: item.SourceSessionID,
			SourceMessageID: item.SourceMessageID,
			CreatedAt:       item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:       item.UpdatedAt.Format(time.RFC3339),
		})
	}
	return &v1.AgentMemoriesRes{
		AssistantKey: req.AssistantKey,
		Items:        result,
	}, nil
}
