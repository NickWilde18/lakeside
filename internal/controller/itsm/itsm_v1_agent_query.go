package itsm

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"

	"lakeside/api/itsm/v1"
	"lakeside/internal/service/itsmagent"
)

func (c *ControllerV1) AgentQuery(ctx context.Context, req *v1.AgentQueryReq) (res *v1.AgentQueryRes, err error) {
	userCode := req.UserID
	g.Log().Infof(ctx, "itsm query request received, user_code=%s message_len=%d", userCode, len(req.Message))
	rsp, err := itsmagent.GetService(ctx).Query(ctx, &itsmagent.QueryRequest{
		UserCode: userCode,
		Message:  req.Message,
	})
	if err != nil {
		return nil, err
	}
	if rsp == nil {
		return &v1.AgentQueryRes{}, nil
	}
	return &v1.AgentQueryRes{AgentResponse: *rsp}, nil
}
