package assistant

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/frame/g"

	"lakeside/api/assistant/v1"
	assistantservice "lakeside/internal/service/assistant"
)

func (c *ControllerV1) AssistantMemories(ctx context.Context, req *v1.AssistantMemoriesReq) (res *v1.AssistantMemoriesRes, err error) {
	g.Log().Infof(ctx, "assistant memories request received, user_code=%s limit=%d", req.UserID, req.Limit)
	items, err := assistantservice.GetService(ctx).ListMemories(ctx, &assistantservice.ListMemoriesRequest{
		UserCode: req.UserID,
		Limit:    req.Limit,
	})
	if err != nil {
		return nil, err
	}
	res = &v1.AssistantMemoriesRes{
		Items: make([]v1.AssistantMemory, 0, len(items)),
	}
	for _, item := range items {
		res.Items = append(res.Items, v1.AssistantMemory{
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
	return res, nil
}
