package v1

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/goai"
)

// TicketDraft 表示 agent 当前整理出的工单草稿。
type TicketDraft struct {
	UserCode     string `json:"userCode" dc:"发起人工号" example:"122020255"`
	Subject      string `json:"subject" dc:"工单主题" example:"道扬书院C1010 WiFi故障"`
	ServiceLevel string `json:"serviceLevel" dc:"服务级别，1最高，2高，3中，4低" example:"3"`
	Priority     string `json:"priority" dc:"工单类型，1咨询，2服务，3故障，4反馈" example:"3"`
	OthersDesc   string `json:"othersDesc" dc:"问题描述" example:"WiFi能搜到但连接后无法上网，从昨晚开始"`
}

// AgentInterrupt 表示 agent 中断的原因，以及前端继续流程时需要回传的数据。
type AgentInterrupt struct {
	InterruptID    string       `json:"interrupt_id" dc:"中断点 ID，resume 时作为 targets 的 key" example:"6819cf6c-ea98-49d2-82b3-3e7cbcbc90b7"`
	Type           string       `json:"type" dc:"中断类型，如 need_info 或 need_confirm" example:"need_confirm"`
	Prompt         string       `json:"prompt" dc:"展示给用户的提示文案" example:"请确认工单信息。你可以编辑 subject 和 othersDesc，确认后将正式提交。"`
	MissingFields  []string     `json:"missing_fields,omitempty" dc:"当前仍需补充的字段名列表" example:"serviceLevel"`
	EditableFields []string     `json:"editable_fields,omitempty" dc:"确认阶段允许前端修改的字段名列表" example:"subject,othersDesc"`
	ReadonlyFields []string     `json:"readonly_fields,omitempty" dc:"确认阶段只读展示的字段名列表" example:"userCode,serviceLevel,priority"`
	Draft          *TicketDraft `json:"draft,omitempty" dc:"当前工单草稿"`
}

type AgentResult struct {
	Success  bool   `json:"success" dc:"本次执行是否成功" example:"true"`
	TicketNo string `json:"ticket_no,omitempty" dc:"创建成功后的工单单号" example:"SQ26030001"`
	Message  string `json:"message" dc:"返回给用户的结果说明" example:"保存服务工单成功"`
	Code     int    `json:"code,omitempty" dc:"下游工单系统返回的业务码，不是 HTTP 状态码" example:"500"`
}

// AgentResponse 是 query 和 resume 共用的响应体。
// status=need_info：展示 prompt，并在 resume.targets[interrupt_id] 中回传 answer。
// status=need_confirm：展示草稿给用户确认，回传 confirmed，可选覆盖 subject/othersDesc。
// status=done：流程结束，直接展示 result。
type AgentResponse struct {
	Status       string           `json:"status" dc:"当前流程状态：need_info、need_confirm、done、error" example:"done"`
	SessionID    string           `json:"session_id" dc:"本轮 agent 会话 ID" example:"sess-d1b6bce5-ec98-488c-8976-25e32801ca28"`
	CheckpointID string           `json:"checkpoint_id,omitempty" dc:"流程快照 ID，resume 时必须回传；done 场景通常为空" example:"ckpt-b64cb049-85a8-433a-a5b7-fb5ad6d2b0f0"`
	Interrupts   []AgentInterrupt `json:"interrupts,omitempty" dc:"中断详情列表"`
	Result       *AgentResult     `json:"result,omitempty" dc:"流程结束时的执行结果"`
}

// ResumeTarget 有两种用法：
// 1. need_info：只传 answer。
// 2. need_confirm：传 confirmed，可选覆盖 subject/othersDesc。
type ResumeTarget struct {
	Answer     string `json:"answer" dc:"补信息阶段提交的用户回答" example:"服务级别填3。道扬书院C1010，WiFi能搜到但连接后无法上网。"`
	Confirmed  *bool  `json:"confirmed" dc:"确认阶段是否确认提交" example:"true"`
	Subject    string `json:"subject" dc:"确认阶段可回写的工单主题" example:"道扬书院C1010宿舍 WiFi故障"`
	OthersDesc string `json:"othersDesc" dc:"确认阶段可回写的问题描述" example:"WiFi能搜到但连接后无法上网，从昨晚开始，多台设备均无法使用。"`
}

type AgentQueryReq struct {
	g.Meta  `path:"/v1/itsm/agent/query" tags:"ITSM" method:"post" summary:"发起 ITSM 工单对话" dc:"根据用户自然语言生成工单草稿，必要时返回 need_info 或 need_confirm" example:"{\"message\":\"寝室 WiFi 坏了，连不上网\"}"`
	UserID  string `json:"-" in:"header" param:"X-User-ID" v:"required" dc:"当前登录人工号，请求头 X-User-ID" example:"122020255"`
	Message string `json:"message" v:"required" dc:"用户本轮输入内容" example:"寝室 WiFi 坏了，连不上网"`
}

type AgentQueryRes struct {
	AgentResponse
}

type AgentResumeReq struct {
	g.Meta       `path:"/v1/itsm/agent/resume" tags:"ITSM" method:"post" summary:"继续 ITSM 工单对话" dc:"使用 query 返回的 checkpoint_id 和 interrupt_id 继续中断流程。补信息阶段传 answer；确认阶段传 confirmed，可选覆盖 subject/othersDesc。若 confirmed=true 且不传 subject、othersDesc，则表示直接确认当前草稿；若 confirmed=false，则表示取消提交。"`
	UserID       string                   `json:"-" in:"header" param:"X-User-ID" v:"required" dc:"当前登录人工号，请求头 X-User-ID" example:"122020255"`
	CheckpointID string                   `json:"checkpoint_id" v:"required" dc:"query 返回的流程快照 ID" example:"ckpt-b64cb049-85a8-433a-a5b7-fb5ad6d2b0f0"`
	Targets      map[string]*ResumeTarget `json:"targets" v:"required" dc:"以 interrupt_id 为 key 的恢复输入集合，值为 ResumeTarget。补信息场景传 answer；确认场景传 confirmed，可选附带 subject、othersDesc。" example:"{\"6819cf6c-ea98-49d2-82b3-3e7cbcbc90b7\":{\"confirmed\":true,\"subject\":\"道扬书院C1010宿舍 WiFi故障\"}}"`
}

type AgentResumeRes struct {
	AgentResponse
}

var (
	AgentQueryReqExample = g.Map{
		"message": "道扬书院 C1010 WiFi 坏了，连不上网。",
	}
	AgentQueryResExamples = goai.Examples{
		"need_info": {
			Value: &goai.Example{
				Summary: "需要补充信息",
				Value: g.Map{
					"code":    0,
					"message": "",
					"data": g.Map{
						"status":        "need_info",
						"session_id":    "sess-a2c42cb0-d65e-4a44-b0fb-30680020a0b1",
						"checkpoint_id": "ckpt-b64cb049-85a8-433a-a5b7-fb5ad6d2b0f0",
						"interrupts": []g.Map{
							{
								"interrupt_id":   "83120df4-a30d-44a4-b958-98a94689b8c7",
								"type":           "need_info",
								"prompt":         "信息还不完整，请补充：服务级别。补充说明：请提供寝室具体位置（楼号、房间号）及故障现象。",
								"missing_fields": []string{"serviceLevel"},
								"draft": g.Map{
									"userCode":     "122020255",
									"subject":      "寝室WiFi故障",
									"serviceLevel": "3",
									"priority":     "3",
									"othersDesc":   "WiFi无法连接",
								},
							},
						},
					},
				},
			},
		},
		"need_confirm": {
			Value: &goai.Example{
				Summary: "进入确认阶段",
				Value: g.Map{
					"code":    0,
					"message": "",
					"data": g.Map{
						"status":        "need_confirm",
						"session_id":    "sess-4f8e3652-30ff-4d84-99ea-5df7b359af80",
						"checkpoint_id": "ckpt-b64cb049-85a8-433a-a5b7-fb5ad6d2b0f0",
						"interrupts": []g.Map{
							{
								"interrupt_id":    "6819cf6c-ea98-49d2-82b3-3e7cbcbc90b7",
								"type":            "need_confirm",
								"prompt":          "请确认工单信息。你可以编辑 subject 和 othersDesc，确认后将正式提交。",
								"editable_fields": []string{"subject", "othersDesc"},
								"readonly_fields": []string{"userCode", "serviceLevel", "priority"},
								"draft": g.Map{
									"userCode":     "122020255",
									"subject":      "道扬书院C1010 WiFi故障",
									"serviceLevel": "3",
									"priority":     "3",
									"othersDesc":   "WiFi能搜到但连接后无法上网，从昨晚开始",
								},
							},
						},
					},
				},
			},
		},
	}
	AgentResumeReqExamples = goai.Examples{
		"need_info": {
			Value: &goai.Example{
				Summary: "补充信息阶段",
				Value: g.Map{
					"checkpoint_id": "ckpt-b64cb049-85a8-433a-a5b7-fb5ad6d2b0f0",
					"targets": g.Map{
						"83120df4-a30d-44a4-b958-98a94689b8c7": g.Map{
							"answer": "服务级别填3。道扬书院C1010，WiFi能搜到但连接后无法上网，从昨晚开始。",
						},
					},
				},
			},
		},
		"need_confirm": {
			Value: &goai.Example{
				Summary: "确认并修改草稿",
				Value: g.Map{
					"checkpoint_id": "ckpt-b64cb049-85a8-433a-a5b7-fb5ad6d2b0f0",
					"targets": g.Map{
						"6819cf6c-ea98-49d2-82b3-3e7cbcbc90b7": g.Map{
							"confirmed":  true,
							"subject":    "道扬书院C1010宿舍 WiFi故障",
							"othersDesc": "WiFi能搜到但连接后无法上网，从昨晚开始，多台设备均无法使用。",
						},
					},
				},
			},
		},
	}
	AgentResumeResExamples = goai.Examples{
		"done_success": {
			Value: &goai.Example{
				Summary: "提交成功",
				Value: g.Map{
					"code":    0,
					"message": "",
					"data": g.Map{
						"status":     "done",
						"session_id": "sess-d1b6bce5-ec98-488c-8976-25e32801ca28",
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
				Summary: "用户取消提交",
				Value: g.Map{
					"code":    0,
					"message": "",
					"data": g.Map{
						"status":     "done",
						"session_id": "sess-4f8e3652-30ff-4d84-99ea-5df7b359af80",
						"result": g.Map{
							"success": false,
							"message": "用户取消提交工单",
						},
					},
				},
			},
		},
	}
)
