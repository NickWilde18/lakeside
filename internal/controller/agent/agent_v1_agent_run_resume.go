package agent

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"

	v1 "lakeside/api/agent/v1"
	"lakeside/internal/service/agentplatform"
)

func (c *ControllerV1) AgentRunResume(ctx context.Context, req *v1.AgentRunResumeReq) (res *v1.AgentRunResumeRes, err error) {
	g.Log().Infof(ctx, "agent run resume request received, assistant_key=%s user_upn=%s run_id=%s target_count=%d", req.AssistantKey, req.UserID, req.RunID, len(req.Targets))
	out, err := agentplatform.GetService(ctx).ResumeRun(ctx, &agentplatform.ResumeRunRequest{
		AssistantKey: req.AssistantKey,
		RunID:        req.RunID,
		UserUPN:      req.UserID,
		Targets:      req.Targets,
	})
	if err != nil {
		return nil, err
	}
	return &v1.AgentRunResumeRes{
		AssistantKey: out.AssistantKey,
		RunID:        out.RunID,
		SessionID:    out.SessionID,
		RunStatus:    out.RunStatus,
	}, nil
}
