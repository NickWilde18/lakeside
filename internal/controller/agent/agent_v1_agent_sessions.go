package agent

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"

	v1 "lakeside/api/agent/v1"
	"lakeside/internal/service/agentplatform"
)

func (c *ControllerV1) AgentSessions(ctx context.Context, req *v1.AgentSessionsReq) (res *v1.AgentSessionsRes, err error) {
	g.Log().Infof(ctx, "agent sessions request received, assistant_key=%s user_upn=%s limit=%d", req.AssistantKey, req.UserID, req.Limit)
	items, err := agentplatform.GetService(ctx).ListSessions(ctx, &agentplatform.ListSessionsRequest{
		AssistantKey: req.AssistantKey,
		UserUPN:      req.UserID,
		Limit:        req.Limit,
	})
	if err != nil {
		return nil, err
	}
	result := make([]v1.AgentSessionSummary, 0, len(items))
	for _, item := range items {
		result = append(result, buildAgentSessionSummary(item))
	}
	return &v1.AgentSessionsRes{
		AssistantKey: req.AssistantKey,
		Items:        result,
	}, nil
}
