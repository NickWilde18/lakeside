package agent

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"

	v1 "lakeside/api/agent/v1"
	"lakeside/internal/service/agentplatform"
)

func (c *ControllerV1) AgentSessionDetail(ctx context.Context, req *v1.AgentSessionDetailReq) (res *v1.AgentSessionDetailRes, err error) {
	g.Log().Infof(ctx, "agent session detail request received, assistant_key=%s user_upn=%s session_id=%s", req.AssistantKey, req.UserID, req.SessionID)
	detail, err := agentplatform.GetService(ctx).GetSessionDetail(ctx, &agentplatform.GetSessionRequest{
		AssistantKey: req.AssistantKey,
		SessionID:    req.SessionID,
		UserUPN:      req.UserID,
	})
	if err != nil {
		return nil, err
	}
	return &v1.AgentSessionDetailRes{
		Detail: buildAgentSessionDetail(detail),
	}, nil
}
