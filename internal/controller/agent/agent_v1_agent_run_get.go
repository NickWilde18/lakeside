package agent

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"

	v1 "lakeside/api/agent/v1"
	"lakeside/internal/service/agentplatform"
)

func (c *ControllerV1) AgentRunGet(ctx context.Context, req *v1.AgentRunGetReq) (res *v1.AgentRunGetRes, err error) {
	g.Log().Infof(ctx, "agent run get request received, assistant_key=%s user_upn=%s run_id=%s", req.AssistantKey, req.UserID, req.RunID)
	snapshot, err := agentplatform.GetService(ctx).GetRun(ctx, &agentplatform.GetRunRequest{
		AssistantKey: req.AssistantKey,
		RunID:        req.RunID,
		UserUPN:      req.UserID,
	})
	if err != nil {
		return nil, err
	}
	return &v1.AgentRunGetRes{AgentRunSnapshot: buildAgentRunSnapshot(snapshot)}, nil
}
