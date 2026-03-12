package agent

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"

	"lakeside/api/agent/v1"
	"lakeside/internal/service/agentplatform"
)

func (c *ControllerV1) AgentSessionDelete(ctx context.Context, req *v1.AgentSessionDeleteReq) (res *v1.AgentSessionDeleteRes, err error) {
	g.Log().Infof(ctx, "agent session delete request received, assistant_key=%s user_upn=%s session_id=%s", req.AssistantKey, req.UserID, req.SessionID)
	if err = agentplatform.GetService(ctx).DeleteSession(ctx, &agentplatform.DeleteSessionRequest{
		AssistantKey: req.AssistantKey,
		SessionID:    req.SessionID,
		UserUPN:      req.UserID,
	}); err != nil {
		return nil, err
	}
	return &v1.AgentSessionDeleteRes{
		AssistantKey: req.AssistantKey,
		SessionID:    req.SessionID,
		Result: v1.AgentSessionDeleteResult{
			Deleted: true,
		},
	}, nil
}
