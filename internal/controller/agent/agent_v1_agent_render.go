package agent

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"

	v1 "lakeside/api/agent/v1"
	"lakeside/internal/service/agentplatform"
)

func (c *ControllerV1) AgentRender(ctx context.Context, req *v1.AgentRenderReq) (res *v1.AgentRenderRes, err error) {
	svc := agentplatform.GetService(ctx)
	state, err := buildAgentPageState(ctx, svc, req.AssistantKey, req.UserID, req.SessionID, req.AdvancedTrace, nil)
	if err != nil {
		return nil, err
	}
	r := g.RequestFromCtx(ctx)
	r.Response.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	r.Response.Header().Set("Cache-Control", "no-cache")
	if err := writeA2UIRender(r.Response.Writer, state); err != nil {
		return nil, err
	}
	r.Response.Flush()
	return nil, nil
}
