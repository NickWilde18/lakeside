package v1

import (
	itsmv1 "lakeside/api/itsm/v1"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/goai"
)

type AssistantResponse struct {
	Status       string                  `json:"status" dc:"当前流程状态：need_info、need_confirm、done、error" example:"done"`
	SessionID    string                  `json:"session_id" dc:"顶层 IT 小助手会话 ID" example:"sess-4f8e3652-30ff-4d84-99ea-5df7b359af80"`
	CheckpointID string                  `json:"checkpoint_id,omitempty" dc:"当前顶层 assistant ADK checkpoint_id；resume 时必须回传，done 场景通常为空" example:"ckpt-b64cb049-85a8-433a-a5b7-fb5ad6d2b0f0"`
	ActiveAgent  string                  `json:"active_agent" dc:"当前处理本轮请求的子代理名称，如 assistant 或 itsm" example:"itsm"`
	Interrupts   []itsmv1.AgentInterrupt `json:"interrupts,omitempty" dc:"当前子代理返回的中断详情列表"`
	Result       *itsmv1.AgentResult     `json:"result,omitempty" dc:"流程结束时的执行结果"`
}

type AssistantQueryReq struct {
	g.Meta  `path:"/v1/assistant/query" tags:"Assistant" method:"post" summary:"发起顶层 IT 小助手对话" dc:"由顶层 IT 小助手决定本轮请求是直接处理还是路由到子代理。首期已接入 ITSM 子代理。" example:"{\"message\":\"宿舍 WiFi 坏了，帮我报修\"}"`
	UserID  string `json:"-" in:"header" param:"X-User-ID" v:"required" dc:"当前登录人工号，请求头 X-User-ID" example:"122020255"`
	Message string `json:"message" v:"required" dc:"用户本轮输入内容" example:"宿舍 WiFi 坏了，帮我报修"`
}

type AssistantQueryRes struct {
	AssistantResponse
}

type AssistantResumeReq struct {
	g.Meta       `path:"/v1/assistant/resume" tags:"Assistant" method:"post" summary:"继续顶层 IT 小助手对话" dc:"使用 query 返回的顶层 assistant checkpoint_id 继续中断流程。首期实际中断点来自 ITSM 子代理。"`
	UserID       string                          `json:"-" in:"header" param:"X-User-ID" v:"required" dc:"当前登录人工号，请求头 X-User-ID" example:"122020255"`
	SessionID    string                          `json:"session_id" v:"required" dc:"顶层 IT 小助手会话 ID" example:"sess-4f8e3652-30ff-4d84-99ea-5df7b359af80"`
	CheckpointID string                          `json:"checkpoint_id" v:"required" dc:"当前顶层 assistant 的 checkpoint_id" example:"ckpt-b64cb049-85a8-433a-a5b7-fb5ad6d2b0f0"`
	Targets      map[string]*itsmv1.ResumeTarget `json:"targets" v:"required" dc:"继续当前子代理流程所需的恢复输入，值类型为 lakeside.api.itsm.v1.ResumeTarget" example:"{\"6819cf6c-ea98-49d2-82b3-3e7cbcbc90b7\":{\"confirmed\":true}}"`
}

type AssistantResumeRes struct {
	AssistantResponse
}

type AssistantMemory struct {
	ID              int64   `json:"id" dc:"长期记忆记录 ID" example:"1"`
	Category        string  `json:"category" dc:"长期记忆分类，如 identity、role、location、preference" example:"location"`
	CanonicalKey    string  `json:"canonical_key" dc:"长期记忆的稳定键，用于更新同一条记忆" example:"dormitory_location"`
	Content         string  `json:"content" dc:"给模型注入的自然语言长期记忆内容" example:"用户住在道扬书院C1010"`
	ValueJSON       string  `json:"value_json,omitempty" dc:"补充结构化信息的 JSON 字符串" example:"{}"`
	Confidence      float64 `json:"confidence" dc:"长期记忆置信度" example:"0.95"`
	SourceSessionID string  `json:"source_session_id" dc:"该记忆来源的会话 ID" example:"sess-de2dab67-0678-4a9f-99d9-8e2a5126af53"`
	SourceMessageID int64   `json:"source_message_id" dc:"该记忆来源的消息 ID" example:"10"`
	CreatedAt       string  `json:"created_at" dc:"创建时间" example:"2026-03-10T01:36:06+08:00"`
	UpdatedAt       string  `json:"updated_at" dc:"更新时间" example:"2026-03-10T01:36:06+08:00"`
}

type AssistantMemoriesReq struct {
	g.Meta `path:"/v1/assistant/memories" tags:"Assistant" method:"get" summary:"查看当前用户长期记忆" dc:"返回当前 X-User-ID 对应的长期记忆列表。顶层 assistant 在每次 query/resume 前会把这些长期记忆拼进 assistant_context。"`
	UserID string `json:"-" in:"header" param:"X-User-ID" v:"required" dc:"当前登录人工号，请求头 X-User-ID" example:"122020255"`
	Limit  int    `json:"limit" in:"query" dc:"返回条数上限，默认 20" example:"20"`
}

type AssistantMemoriesRes struct {
	Items []AssistantMemory `json:"items" dc:"当前用户的长期记忆列表"`
}

type AssistantMemoriesClearReq struct {
	g.Meta       `path:"/v1/assistant/memories/clear" tags:"Assistant" method:"post" summary:"清除当前用户长期记忆" dc:"默认清空当前用户全部长期记忆；如果传 category，则清空该分类；如果同时传 category 和 canonical_key，则只删除该条记忆。"`
	UserID       string `json:"-" in:"header" param:"X-User-ID" v:"required" dc:"当前登录人工号，请求头 X-User-ID" example:"122020255"`
	Category     string `json:"category,omitempty" dc:"可选，限定要清除的长期记忆分类" example:"location"`
	CanonicalKey string `json:"canonical_key,omitempty" dc:"可选，限定要清除的长期记忆稳定键；通常与 category 搭配使用" example:"dormitory_location"`
}

type AssistantMemoriesClearResult struct {
	DeletedCount int64 `json:"deleted_count" dc:"本次删除的长期记忆条数" example:"1"`
}

type AssistantMemoriesClearRes struct {
	Result AssistantMemoriesClearResult `json:"result" dc:"清理结果"`
}

var (
	AssistantQueryReqExample = g.Map{
		"message": "宿舍 WiFi 坏了，帮我报修。",
	}
	AssistantQueryResExamples = goai.Examples{
		"itsm_need_info": {
			Value: &goai.Example{
				Summary: "路由到 ITSM 并要求补充信息",
				Value: g.Map{
					"code":    0,
					"message": "",
					"data": g.Map{
						"status":        "need_info",
						"session_id":    "sess-4f8e3652-30ff-4d84-99ea-5df7b359af80",
						"checkpoint_id": "ckpt-b64cb049-85a8-433a-a5b7-fb5ad6d2b0f0",
						"active_agent":  "itsm",
						"interrupts": []g.Map{{
							"interrupt_id":   "83120df4-a30d-44a4-b958-98a94689b8c7",
							"type":           "need_info",
							"prompt":         "信息还不完整，请补充：问题描述。补充说明：请提供寝室具体位置（楼号、房间号）及故障现象。",
							"missing_fields": []string{"othersDesc"},
						}},
					},
				},
			},
		},
		"assistant_fallback": {
			Value: &goai.Example{
				Summary: "未命中已接入子代理时由顶层助手直接回复",
				Value: g.Map{
					"code":    0,
					"message": "",
					"data": g.Map{
						"status":       "done",
						"session_id":   "sess-4f8e3652-30ff-4d84-99ea-5df7b359af80",
						"active_agent": "assistant",
						"result": g.Map{
							"success": false,
							"message": "目前 IT 小助手首期只接入了工单建单能力。",
						},
					},
				},
			},
		},
	}
	AssistantResumeReqExamples = goai.Examples{
		"need_info": {
			Value: &goai.Example{
				Summary: "继续 ITSM 补信息阶段",
				Value: g.Map{
					"session_id":    "sess-4f8e3652-30ff-4d84-99ea-5df7b359af80",
					"checkpoint_id": "ckpt-b64cb049-85a8-433a-a5b7-fb5ad6d2b0f0",
					"targets": g.Map{
						"83120df4-a30d-44a4-b958-98a94689b8c7": g.Map{
							"answer": "道扬书院C1010，WiFi能搜到但连接后无法上网，宿舍里多台设备都受影响。",
						},
					},
				},
			},
		},
		"need_confirm": {
			Value: &goai.Example{
				Summary: "继续 ITSM 确认阶段",
				Value: g.Map{
					"session_id":    "sess-4f8e3652-30ff-4d84-99ea-5df7b359af80",
					"checkpoint_id": "ckpt-b64cb049-85a8-433a-a5b7-fb5ad6d2b0f0",
					"targets": g.Map{
						"6819cf6c-ea98-49d2-82b3-3e7cbcbc90b7": g.Map{
							"confirmed": true,
						},
					},
				},
			},
		},
	}
	AssistantResumeResExamples = goai.Examples{
		"done_success": {
			Value: &goai.Example{
				Summary: "ITSM 子代理提交成功",
				Value: g.Map{
					"code":    0,
					"message": "",
					"data": g.Map{
						"status":       "done",
						"session_id":   "sess-4f8e3652-30ff-4d84-99ea-5df7b359af80",
						"active_agent": "itsm",
						"result": g.Map{
							"success":   true,
							"ticket_no": "SQ26030001",
							"message":   "保存服务工单成功",
							"code":      500,
						},
					},
				},
			},
		},
		"done_cancel": {
			Value: &goai.Example{
				Summary: "用户取消 ITSM 提交",
				Value: g.Map{
					"code":    0,
					"message": "",
					"data": g.Map{
						"status":       "done",
						"session_id":   "sess-4f8e3652-30ff-4d84-99ea-5df7b359af80",
						"active_agent": "itsm",
						"result": g.Map{
							"success": false,
							"message": "用户取消提交工单",
						},
					},
				},
			},
		},
	}
	AssistantMemoriesResExamples = goai.Examples{
		"list": {
			Value: &goai.Example{
				Summary: "查看当前用户长期记忆",
				Value: g.Map{
					"code":    0,
					"message": "",
					"data": g.Map{
						"items": []g.Map{{
							"id":                1,
							"category":          "location",
							"canonical_key":     "dormitory_location",
							"content":           "用户住在道扬书院C1010",
							"confidence":        0.95,
							"source_session_id": "sess-de2dab67-0678-4a9f-99d9-8e2a5126af53",
							"source_message_id": 10,
							"created_at":        "2026-03-10T01:36:06+08:00",
							"updated_at":        "2026-03-10T01:36:06+08:00",
						}},
					},
				},
			},
		},
	}
	AssistantMemoriesClearReqExample = g.Map{
		"category":      "location",
		"canonical_key": "dormitory_location",
	}
	AssistantMemoriesClearResExamples = goai.Examples{
		"clear_specific": {
			Value: &goai.Example{
				Summary: "定向清除一条长期记忆",
				Value: g.Map{
					"code":    0,
					"message": "",
					"data": g.Map{
						"result": g.Map{
							"deleted_count": 1,
						},
					},
				},
			},
		},
	}
)
