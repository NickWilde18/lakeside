package agent

import (
	"context"

	v1 "lakeside/api/agent/v1"
	"lakeside/internal/service/agentplatform"
)

func (c *ControllerV1) AgentSessionCreate(ctx context.Context, req *v1.AgentSessionCreateReq) (res *v1.AgentSessionCreateRes, err error) {
	result, err := agentplatform.GetService(ctx).CreateSession(ctx, &agentplatform.CreateSessionRequest{
		AssistantKey: req.AssistantKey,
		UserUPN:      req.UserID,
		Language:     "zh",
	})
	if err != nil {
		return nil, err
	}
	return &v1.AgentSessionCreateRes{
		AssistantKey: result.AssistantKey,
		SessionID:    result.SessionID,
	}, nil
}
