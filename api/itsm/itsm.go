// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package itsm

import (
	"context"

	"lakeside/api/itsm/v1"
)

type IItsmV1 interface {
	AgentQuery(ctx context.Context, req *v1.AgentQueryReq) (res *v1.AgentQueryRes, err error)
	AgentResume(ctx context.Context, req *v1.AgentResumeReq) (res *v1.AgentResumeRes, err error)
}
