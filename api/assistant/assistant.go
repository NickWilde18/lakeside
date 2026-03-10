// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package assistant

import (
	"context"

	"lakeside/api/assistant/v1"
)

type IAssistantV1 interface {
	AssistantQuery(ctx context.Context, req *v1.AssistantQueryReq) (res *v1.AssistantQueryRes, err error)
	AssistantResume(ctx context.Context, req *v1.AssistantResumeReq) (res *v1.AssistantResumeRes, err error)
	AssistantMemories(ctx context.Context, req *v1.AssistantMemoriesReq) (res *v1.AssistantMemoriesRes, err error)
	AssistantMemoriesClear(ctx context.Context, req *v1.AssistantMemoriesClearReq) (res *v1.AssistantMemoriesClearRes, err error)
}
