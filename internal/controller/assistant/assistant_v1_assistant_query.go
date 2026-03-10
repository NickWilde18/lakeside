package assistant

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"

	"lakeside/api/assistant/v1"
	assistantservice "lakeside/internal/service/assistant"
)

func (c *ControllerV1) AssistantQuery(ctx context.Context, req *v1.AssistantQueryReq) (res *v1.AssistantQueryRes, err error) {
	g.Log().Infof(ctx, "assistant query request received, user_code=%s message_len=%d", req.UserID, len(req.Message))
	rsp, err := assistantservice.GetService(ctx).Query(ctx, &assistantservice.QueryRequest{
		UserCode: req.UserID,
		Message:  req.Message,
	})
	if err != nil {
		return nil, err
	}
	if rsp == nil {
		return &v1.AssistantQueryRes{}, nil
	}
	return &v1.AssistantQueryRes{AssistantResponse: v1.AssistantResponse{
		Status:       rsp.Status,
		SessionID:    rsp.SessionID,
		CheckpointID: rsp.CheckpointID,
		ActiveAgent:  rsp.ActiveAgent,
		Interrupts:   rsp.Interrupts,
		Result:       rsp.Result,
	}}, nil
}
