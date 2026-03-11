// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package agent

import (
	"context"

	"lakeside/api/agent/v1"
)

type IAgentV1 interface {
	AgentRunCreate(ctx context.Context, req *v1.AgentRunCreateReq) (res *v1.AgentRunCreateRes, err error)
	AgentRunGet(ctx context.Context, req *v1.AgentRunGetReq) (res *v1.AgentRunGetRes, err error)
	AgentRunResume(ctx context.Context, req *v1.AgentRunResumeReq) (res *v1.AgentRunResumeRes, err error)
	AgentRunCancel(ctx context.Context, req *v1.AgentRunCancelReq) (res *v1.AgentRunCancelRes, err error)
	AgentRunEvents(ctx context.Context, req *v1.AgentRunEventsReq) (res *v1.AgentRunEventsRes, err error)
	AgentMemories(ctx context.Context, req *v1.AgentMemoriesReq) (res *v1.AgentMemoriesRes, err error)
	AgentMemoriesClear(ctx context.Context, req *v1.AgentMemoriesClearReq) (res *v1.AgentMemoriesClearRes, err error)
}
