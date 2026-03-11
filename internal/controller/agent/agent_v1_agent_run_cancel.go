package agent

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"

	v1 "lakeside/api/agent/v1"
	"lakeside/internal/service/agentplatform"
)

func (c *ControllerV1) AgentRunCancel(ctx context.Context, req *v1.AgentRunCancelReq) (res *v1.AgentRunCancelRes, err error) {
	g.Log().Infof(ctx, "agent run cancel request received, assistant_key=%s user_upn=%s run_id=%s", req.AssistantKey, req.UserID, req.RunID)
	if err := agentplatform.GetService(ctx).CancelRun(ctx, &agentplatform.CancelRunRequest{
		AssistantKey: req.AssistantKey,
		RunID:        req.RunID,
		UserUPN:      req.UserID,
	}); err != nil {
		return nil, err
	}
	return &v1.AgentRunCancelRes{
		AssistantKey: req.AssistantKey,
		RunID:        req.RunID,
		Result: v1.AgentRunCancelResult{
			Cancelled: true,
		},
	}, nil
}
