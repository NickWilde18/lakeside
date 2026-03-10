package assistant

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"

	"lakeside/api/assistant/v1"
	assistantservice "lakeside/internal/service/assistant"
)

func (c *ControllerV1) AssistantResume(ctx context.Context, req *v1.AssistantResumeReq) (res *v1.AssistantResumeRes, err error) {
	g.Log().Infof(ctx, "assistant resume request received, user_code=%s session_id=%s checkpoint_id=%s target_count=%d", req.UserID, req.SessionID, req.CheckpointID, len(req.Targets))
	rsp, err := assistantservice.GetService(ctx).Resume(ctx, &assistantservice.ResumeRequest{
		SessionID:    req.SessionID,
		CheckpointID: req.CheckpointID,
		UserCode:     req.UserID,
		Targets:      req.Targets,
	})
	if err != nil {
		return nil, err
	}
	if rsp == nil {
		return &v1.AssistantResumeRes{}, nil
	}
	return &v1.AssistantResumeRes{AssistantResponse: v1.AssistantResponse{
		Status:       rsp.Status,
		SessionID:    rsp.SessionID,
		CheckpointID: rsp.CheckpointID,
		ActiveAgent:  rsp.ActiveAgent,
		Interrupts:   rsp.Interrupts,
		Result:       rsp.Result,
	}}, nil
}
