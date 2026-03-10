package openapi

import (
	"sync"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/net/goai"
	assistantv1 "lakeside/api/assistant/v1"

	itsmv1 "lakeside/api/itsm/v1"
)

var patchOnce sync.Once

// PatchServerExamples 在 OpenAPI 已生成后补充业务示例。
func PatchServerExamples(s *ghttp.Server) {
	if s == nil {
		return
	}
	patchOnce.Do(func() {
		patchAgentExamples(s.GetOpenApi())
	})
}

func patchAgentExamples(oai *goai.OpenApiV3) {
	if oai == nil || oai.Paths == nil {
		return
	}
	patchOperationExamples(getOperation(oai, "/v1/assistant/query", "post"), assistantv1.AssistantQueryReqExample, assistantv1.AssistantQueryResExamples)
	patchOperationExamples(getOperation(oai, "/v1/assistant/resume", "post"), assistantv1.AssistantResumeReqExamples, assistantv1.AssistantResumeResExamples)
	patchOperationExamples(getOperation(oai, "/v1/assistant/memories", "get"), nil, assistantv1.AssistantMemoriesResExamples)
	patchOperationExamples(getOperation(oai, "/v1/assistant/memories/clear", "post"), assistantv1.AssistantMemoriesClearReqExample, assistantv1.AssistantMemoriesClearResExamples)
	patchOperationExamples(getOperation(oai, "/v1/itsm/agent/query", "post"), itsmv1.AgentQueryReqExample, itsmv1.AgentQueryResExamples)
	patchOperationExamples(getOperation(oai, "/v1/itsm/agent/resume", "post"), itsmv1.AgentResumeReqExamples, itsmv1.AgentResumeResExamples)
}

func patchOperationExamples(operation *goai.Operation, requestExample interface{}, responseExamples goai.Examples) {
	if operation == nil {
		return
	}
	patchTargetsSchema(operation)
	if operation.RequestBody != nil && operation.RequestBody.Value != nil {
		mediaType := operation.RequestBody.Value.Content["application/json"]
		switch v := requestExample.(type) {
		case goai.Examples:
			mediaType.Examples = v
		default:
			mediaType.Example = v
		}
		operation.RequestBody.Value.Content["application/json"] = mediaType
	}
	if responseRef, ok := operation.Responses["200"]; ok && responseRef.Value != nil {
		mediaType := responseRef.Value.Content["application/json"]
		mediaType.Examples = responseExamples
		responseRef.Value.Content["application/json"] = mediaType
		operation.Responses["200"] = responseRef
	}
}

func patchTargetsSchema(operation *goai.Operation) {
	if operation.RequestBody == nil || operation.RequestBody.Value == nil {
		return
	}
	mediaType, ok := operation.RequestBody.Value.Content["application/json"]
	if !ok || mediaType.Schema == nil || mediaType.Schema.Value == nil {
		return
	}
	targetsSchemaRef := mediaType.Schema.Value.Properties.Get("targets")
	if targetsSchemaRef == nil || targetsSchemaRef.Value == nil {
		return
	}
	targetsSchemaRef.Value.Title = "map[string]lakeside.api.itsm.v1.ResumeTarget"
	targetsSchemaRef.Value.Description = "以 interrupt_id 为 key 的对象，值类型为 lakeside.api.itsm.v1.ResumeTarget。"
	mediaType.Schema.Value.Properties.Set("targets", *targetsSchemaRef)
	operation.RequestBody.Value.Content["application/json"] = mediaType
}

func getOperation(oai *goai.OpenApiV3, path, method string) *goai.Operation {
	pathItem, ok := oai.Paths[path]
	if !ok {
		return nil
	}
	switch method {
	case "post":
		return pathItem.Post
	case "get":
		return pathItem.Get
	case "put":
		return pathItem.Put
	case "delete":
		return pathItem.Delete
	case "patch":
		return pathItem.Patch
	default:
		return nil
	}
}
