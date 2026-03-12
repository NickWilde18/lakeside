package agent

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"

	v1 "lakeside/api/agent/v1"
	"lakeside/internal/service/agentplatform"
)

func (c *ControllerV1) AgentRunCreate(ctx context.Context, req *v1.AgentRunCreateReq) (res *v1.AgentRunCreateRes, err error) {
	g.Log().Infof(ctx, "agent run create request received, assistant_key=%s user_upn=%s session_id=%s message_len=%d", req.AssistantKey, req.UserID, req.SessionID, len(req.Message))
	out, err := agentplatform.GetService(ctx).CreateRun(ctx, &agentplatform.CreateRunRequest{
		AssistantKey: req.AssistantKey,
		UserUPN:      req.UserID,
		SessionID:    req.SessionID,
		Message:      req.Message,
	})
	if err != nil {
		return nil, err
	}
	return &v1.AgentRunCreateRes{
		AssistantKey: out.AssistantKey,
		RunID:        out.RunID,
		SessionID:    out.SessionID,
		RunStatus:    out.RunStatus,
	}, nil
}
