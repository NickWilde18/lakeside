package itsm

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"

	"lakeside/api/itsm/v1"
	"lakeside/internal/service/itsmagent"
)

func (c *ControllerV1) AgentResume(ctx context.Context, req *v1.AgentResumeReq) (res *v1.AgentResumeRes, err error) {
	userCode := req.UserID
	g.Log().Infof(ctx, "itsm resume request received, user_code=%s checkpoint_id=%s target_count=%d", userCode, req.CheckpointID, len(req.Targets))
	rsp, err := itsmagent.GetService(ctx).Resume(ctx, &itsmagent.ResumeRequest{
		CheckpointID: req.CheckpointID,
		UserCode:     userCode,
		Targets:      req.Targets,
	})
	if err != nil {
		return nil, err
	}
	if rsp == nil {
		return &v1.AgentResumeRes{}, nil
	}
	return &v1.AgentResumeRes{AgentResponse: *rsp}, nil
}
