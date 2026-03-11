package agent

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"

	v1 "lakeside/api/agent/v1"
	"lakeside/internal/service/agentplatform"
)

func (c *ControllerV1) AgentMemoriesClear(ctx context.Context, req *v1.AgentMemoriesClearReq) (res *v1.AgentMemoriesClearRes, err error) {
	g.Log().Infof(ctx, "agent memories clear request received, assistant_key=%s user_upn=%s category=%s canonical_key=%s", req.AssistantKey, req.UserID, req.Category, req.CanonicalKey)
	deleted, err := agentplatform.GetService(ctx).ClearMemories(ctx, &agentplatform.ClearMemoriesRequest{
		AssistantKey: req.AssistantKey,
		UserUPN:      req.UserID,
		Category:     req.Category,
		CanonicalKey: req.CanonicalKey,
	})
	if err != nil {
		return nil, err
	}
	return &v1.AgentMemoriesClearRes{
		AssistantKey: req.AssistantKey,
		Result: v1.AgentMemoriesClearResult{
			DeletedCount: deleted,
		},
	}, nil
}
