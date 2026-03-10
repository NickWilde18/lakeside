package assistant

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"

	"lakeside/api/assistant/v1"
	assistantservice "lakeside/internal/service/assistant"
)

func (c *ControllerV1) AssistantMemoriesClear(ctx context.Context, req *v1.AssistantMemoriesClearReq) (res *v1.AssistantMemoriesClearRes, err error) {
	g.Log().Infof(ctx, "assistant memories clear request received, user_code=%s category=%s canonical_key=%s", req.UserID, req.Category, req.CanonicalKey)
	deleted, err := assistantservice.GetService(ctx).ClearMemories(ctx, &assistantservice.ClearMemoriesRequest{
		UserCode:     req.UserID,
		Category:     req.Category,
		CanonicalKey: req.CanonicalKey,
	})
	if err != nil {
		return nil, err
	}
	return &v1.AssistantMemoriesClearRes{
		Result: v1.AssistantMemoriesClearResult{
			DeletedCount: deleted,
		},
	}, nil
}
