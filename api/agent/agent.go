// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package agent

import (
	"context"

	"lakeside/api/agent/v1"
)

type IAgentV1 interface {
	AgentRender(ctx context.Context, req *v1.AgentRenderReq) (res *v1.AgentRenderRes, err error)
	AgentActions(ctx context.Context, req *v1.AgentActionsReq) (res *v1.AgentActionsRes, err error)
	AgentSessionCreate(ctx context.Context, req *v1.AgentSessionCreateReq) (res *v1.AgentSessionCreateRes, err error)
	AgentSessions(ctx context.Context, req *v1.AgentSessionsReq) (res *v1.AgentSessionsRes, err error)
	AgentSessionDetail(ctx context.Context, req *v1.AgentSessionDetailReq) (res *v1.AgentSessionDetailRes, err error)
	AgentSessionDelete(ctx context.Context, req *v1.AgentSessionDeleteReq) (res *v1.AgentSessionDeleteRes, err error)
	AgentMemories(ctx context.Context, req *v1.AgentMemoriesReq) (res *v1.AgentMemoriesRes, err error)
	AgentMemoriesClear(ctx context.Context, req *v1.AgentMemoriesClearReq) (res *v1.AgentMemoriesClearRes, err error)
}
