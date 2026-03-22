package v1

import (
	itsmv1 "lakeside/api/itsm/v1"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/goai"
)

// AgentSource 表示知识检索命中的来源片段。
type AgentSource struct {
	KBID     string  `json:"kb_id" dc:"知识来源所属知识库 ID" example:"campus-it-faq"`
	DocID    string  `json:"doc_id" dc:"命中文档 ID" example:"doc-3f26dfc5"`
	NodeID   string  `json:"node_id" dc:"命中片段节点 ID" example:"node-8dbfd2f8"`
	Filename string  `json:"filename,omitempty" dc:"来源文档文件名" example:"vpn-user-guide.md"`
	Snippet  string  `json:"snippet,omitempty" dc:"命中片段摘要，便于前端直接展示引用内容" example:"连接学校 VPN 后，再访问校内系统。如果仍无法访问，请检查是否已安装并登录统一身份认证客户端。"`
	Score    float64 `json:"score,omitempty" dc:"检索命中分数，仅用于调试或展示排序参考" example:"0.92"`
}

// AgentStep 表示本轮顶层 agent 编排过程中的一个子步骤结果。
type AgentStep struct {
	Path       []string                `json:"path,omitempty" dc:"当前步骤对应的 agent 路径，自顶层到叶子 agent" example:"campus,it,itsm"`
	Kind       string                  `json:"kind" dc:"步骤类型，如 knowledge、itsm_interrupt、itsm_done、assistant_message" example:"knowledge"`
	Message    string                  `json:"message,omitempty" dc:"该步骤面向用户展示的结果说明或回答正文" example:"请先连接学校 VPN，再访问校内资源。"`
	Sources    []AgentSource           `json:"sources,omitempty" dc:"该步骤返回的知识来源列表"`
	Interrupts []itsmv1.AgentInterrupt `json:"interrupts,omitempty" dc:"该步骤产生的中断详情列表；通常用于 ITSM interrupt 场景"`
}

// AgentResult 表示本轮对外暴露的最终结果。
type AgentResult struct {
	Success  bool          `json:"success" dc:"本次执行是否成功" example:"true"`
	TicketNo string        `json:"ticket_no,omitempty" dc:"创建成功后的工单单号，仅 ITSM 场景返回" example:"SQ26030001"`
	Message  string        `json:"message" dc:"返回给用户的最终结果说明或知识回答正文" example:"请先连接学校 VPN，再访问校内资源。"`
	Code     int           `json:"code,omitempty" dc:"下游业务系统返回的业务码，不是 HTTP 状态码" example:"500"`
	Sources  []AgentSource `json:"sources,omitempty" dc:"最终结果引用的知识来源列表，仅知识库场景返回"`
}

// AgentRunSnapshot 表示一次 run 的完整快照。
type AgentRunSnapshot struct {
	RunID        string                  `json:"run_id" dc:"当前执行 run ID" example:"run-8f4b6d3b"`
	AssistantKey string                  `json:"assistant_key" dc:"当前使用的顶层助手 key，对应路径参数 assistant_key" example:"campus"`
	RunStatus    string                  `json:"run_status" dc:"run 运行状态：queued、running、waiting_input、done、failed、cancelled" example:"waiting_input"`
	Status       string                  `json:"status,omitempty" dc:"当前流程状态：need_info、need_confirm、done、error" example:"need_info"`
	SessionID    string                  `json:"session_id,omitempty" dc:"当前顶层助手会话 ID" example:"sess-4f8e3652-30ff-4d84-99ea-5df7b359af80"`
	CheckpointID string                  `json:"checkpoint_id,omitempty" dc:"当前顶层 agent 的 checkpoint_id；waiting_input 时可用于调试，resume 不要求前端回传" example:"ckpt-b64cb049-85a8-433a-a5b7-fb5ad6d2b0f0"`
	ActivePath   []string                `json:"active_path,omitempty" dc:"当前活跃 agent 路径，自顶层到最后处理该请求的子代理" example:"campus,it,itsm"`
	Steps        []AgentStep             `json:"steps,omitempty" dc:"本轮编排过程的步骤结果列表；可同时包含 knowledge 回答与 ITSM interrupt"`
	Interrupts   []itsmv1.AgentInterrupt `json:"interrupts,omitempty" dc:"为了兼容前端直接处理 interrupt，保留顶层中断详情列表；通常与最后一个 itsm_interrupt step 对应"`
	Result       *AgentResult            `json:"result,omitempty" dc:"流程结束时的统一执行结果"`
	ErrorMessage string                  `json:"error_message,omitempty" dc:"run 失败或取消时的错误说明" example:"service restarted before run completed"`
	StartedAt    string                  `json:"started_at,omitempty" dc:"run 开始时间" example:"2026-03-11T21:30:00+08:00"`
	FinishedAt   string                  `json:"finished_at,omitempty" dc:"run 结束时间；未结束时为空" example:"2026-03-11T21:30:12+08:00"`
}

type AgentClientEvent struct {
	UserAction *AgentUserAction `json:"userAction,omitempty" dc:"A2UI client event；当前仅支持 userAction"`
	Error      map[string]any   `json:"error,omitempty" dc:"客户端错误上报；当前仅透传记录"`
}

type AgentUserAction struct {
	Name              string         `json:"name" dc:"动作名称，例如 send_message、follow_up_submit、approval_submit、cancel_turn；历史会话切换和删除默认由前端通过独立 sessions JSON API 处理" example:"send_message"`
	SurfaceID         string         `json:"surfaceId,omitempty" dc:"动作来源 surface ID" example:"agent-canvas-campus"`
	SourceComponentID string         `json:"sourceComponentId,omitempty" dc:"触发动作的组件 ID" example:"composer-send-btn"`
	Timestamp         string         `json:"timestamp,omitempty" dc:"客户端动作时间戳" example:"2026-03-21T14:00:00Z"`
	Context           map[string]any `json:"context,omitempty" dc:"动作附带上下文，由客户端解析 action.context 后回传"`
}

type AgentRenderReq struct {
	g.Meta        `path:"/v1/agent/{assistant_key}/render" tags:"Agent" method:"get" summary:"渲染 agent 聊天区 A2UI surface" dc:"返回 application/x-ndjson 的 A2UI 消息流，只负责中间对话区的动态内容。页面壳子、历史会话列表和高级轨迹面板由前端单独管理。若传 session_id，则恢复该会话的聊天 surface；不传则渲染未选中会话的空态。"`
	AssistantKey  string `json:"-" in:"path" param:"assistant_key" v:"required" dc:"顶层助手 key" example:"campus"`
	UserID        string `json:"-" in:"header" param:"X-User-ID" v:"required" dc:"当前登录用户 UPN，请求头 X-User-ID" example:"122020255@link.cuhk.edu.cn"`
	SessionID     string `json:"session_id,omitempty" in:"query" dc:"可选，会话 ID；用于恢复指定会话页面" example:"sess-a925e3c0-8f4b-4daf-bbe3-1885afd915c5"`
	AdvancedTrace bool   `json:"advanced_trace,omitempty" in:"query" dc:"保留调试开关；不影响页面壳子，执行轨迹默认由前端通过独立 sessions 详情接口拉取" example:"false"`
}

type AgentRenderRes struct {
	Placeholder string `json:"placeholder,omitempty" dc:"render 接口返回 application/x-ndjson，本结构仅用于满足 GoFrame XxxRes 命名要求"`
}

type AgentActionsReq struct {
	g.Meta        `path:"/v1/agent/{assistant_key}/actions" tags:"Agent" method:"post" summary:"提交聊天区 A2UI 用户动作并返回增量 UI 消息流" dc:"接收 A2UI client event（当前主要是 userAction），返回 application/x-ndjson 的 A2UI 消息流。该接口只驱动聊天区中的消息、表单和 interrupt 卡片，不负责整页壳子。复杂动作如 send_message / follow_up_submit / approval_submit 会在同一个响应流里逐步返回聊天区增量更新。"`
	AssistantKey  string           `json:"-" in:"path" param:"assistant_key" v:"required" dc:"顶层助手 key" example:"campus"`
	UserID        string           `json:"-" in:"header" param:"X-User-ID" v:"required" dc:"当前登录用户 UPN，请求头 X-User-ID" example:"122020255@link.cuhk.edu.cn"`
	AdvancedTrace bool             `json:"advanced_trace,omitempty" in:"query" dc:"保留调试开关；高级轨迹默认由前端通过 sessions 详情接口单独展示" example:"false"`
	UserAction    *AgentUserAction `json:"userAction,omitempty" dc:"用户动作"`
	Error         map[string]any   `json:"error,omitempty" dc:"客户端错误上报"`
}

type AgentActionsRes struct {
	Placeholder string `json:"placeholder,omitempty" dc:"actions 接口返回 application/x-ndjson，本结构仅用于满足 GoFrame XxxRes 命名要求"`
}

// AgentRunEvent 表示 run 事件流中的一条结构化事件。
type AgentRunEvent struct {
	EventID      int64    `json:"event_id" dc:"事件 ID，可用于 SSE 断线重连" example:"568"`
	RunID        string   `json:"run_id" dc:"所属 run ID" example:"run-659d3bc2-3db5-4dbe-9d8d-125b85a398e8"`
	AssistantKey string   `json:"assistant_key" dc:"顶层助手 key" example:"campus"`
	SessionID    string   `json:"session_id" dc:"所属会话 ID" example:"sess-5e7ae0ce-46ee-4344-8b99-c2da4c83df32"`
	Path         []string `json:"path,omitempty" dc:"当前事件对应的 agent 路径" example:"campus,it,itsm"`
	EventType    string   `json:"event_type" dc:"事件类型。常见值：run_started、run_waiting_input、run_completed、run_failed、run_cancelled、agent_entered、agent_completed、domain_plan_started、domain_plan_ready、domain_execute_started、domain_supervisor_fallback、knowledge_run_started、knowledge_retrieve_started、knowledge_retrieve_finished、knowledge_answer_chunk、knowledge_answer_generation_started、knowledge_answer_generation_finished、knowledge_answer_ready、knowledge_run_completed、itsm_interrupt_emitted、itsm_done" example:"knowledge_answer_chunk"`
	Message      string   `json:"message,omitempty" dc:"事件说明文案" example:"开始检索知识库"`
	Payload      any      `json:"payload,omitempty" dc:"事件附带的结构化载荷"`
	CreatedAt    string   `json:"created_at,omitempty" dc:"事件创建时间" example:"2026-03-12T01:14:49+08:00"`
}

// AgentSessionSummary 表示一个顶层助手会话摘要。
type AgentSessionSummary struct {
	AssistantKey  string   `json:"assistant_key" dc:"当前顶层助手 key" example:"campus"`
	SessionID     string   `json:"session_id" dc:"当前会话 ID" example:"sess-a925e3c0-8f4b-4daf-bbe3-1885afd915c5"`
	Title         string   `json:"title" dc:"会话标题，通常由首条用户消息裁剪得到" example:"VPN 连不上，顺便告诉我学生群组邮箱地址"`
	Status        string   `json:"status" dc:"当前会话状态，如 active、done" example:"active"`
	ActivePath    []string `json:"active_path,omitempty" dc:"当前会话最后一次活跃的 agent 路径" example:"campus,it,itsm"`
	LastRunID     string   `json:"last_run_id,omitempty" dc:"当前会话最近一次 run ID" example:"run-8f4b6d3b"`
	LastRunStatus string   `json:"last_run_status,omitempty" dc:"当前会话最近一次 run 状态" example:"waiting_input"`
	CreatedAt     string   `json:"created_at,omitempty" dc:"会话创建时间" example:"2026-03-12T00:15:00+08:00"`
	UpdatedAt     string   `json:"updated_at,omitempty" dc:"会话更新时间" example:"2026-03-12T00:18:00+08:00"`
}

type AgentSessionCreateReq struct {
	g.Meta       `path:"/v1/agent/{assistant_key}/sessions" tags:"Agent" method:"post" summary:"创建一个空白 agent 会话" dc:"返回一个新的会话 ID，供前端显式新建对话时使用。当前仅创建空白会话，不触发执行。"`
	AssistantKey string `json:"-" in:"path" param:"assistant_key" v:"required" dc:"顶层助手 key" example:"campus"`
	UserID       string `json:"-" in:"header" param:"X-User-ID" v:"required" dc:"当前登录用户 UPN，请求头 X-User-ID" example:"122020255@link.cuhk.edu.cn"`
}

type AgentSessionCreateRes struct {
	AssistantKey string `json:"assistant_key" dc:"当前顶层助手 key" example:"campus"`
	SessionID    string `json:"session_id" dc:"新创建的会话 ID" example:"sess-a925e3c0-8f4b-4daf-bbe3-1885afd915c5"`
}

// AgentSessionsReq 查看某个顶层助手的历史会话列表。
type AgentSessionsReq struct {
	g.Meta       `path:"/v1/agent/{assistant_key}/sessions" tags:"Agent" method:"get" summary:"查看当前用户的 agent 历史会话列表" dc:"返回当前 assistant_key 下、当前 X-User-ID 对应的历史会话摘要列表，供前端展示历史记录。"`
	AssistantKey string `json:"-" in:"path" param:"assistant_key" v:"required" dc:"顶层助手 key" example:"campus"`
	UserID       string `json:"-" in:"header" param:"X-User-ID" v:"required" dc:"当前登录用户 UPN，请求头 X-User-ID" example:"122020255@link.cuhk.edu.cn"`
	Limit        int    `json:"limit" in:"query" dc:"返回条数上限，默认 20" example:"20"`
}

// AgentSessionsRes 返回当前用户在某个顶层助手下的历史会话摘要列表。
type AgentSessionsRes struct {
	AssistantKey string                `json:"assistant_key" dc:"当前顶层助手 key" example:"campus"`
	Items        []AgentSessionSummary `json:"items" dc:"历史会话摘要列表"`
}

// AgentSessionMessage 表示历史会话中的一条消息。
type AgentSessionMessage struct {
	ID           int64    `json:"id" dc:"消息记录 ID" example:"10"`
	Role         string   `json:"role" dc:"消息角色，user 或 assistant" example:"assistant"`
	Content      string   `json:"content" dc:"消息正文" example:"针对宿舍 WiFi 已连接但无法打开网页的情况，请按以下步骤排查。"`
	ActivePath   []string `json:"active_path,omitempty" dc:"写入该消息时的 agent 路径" example:"campus,it,campus_it_kb_for_itso_student_assistant"`
	CheckpointID string   `json:"checkpoint_id,omitempty" dc:"写入该消息时对应的 checkpoint_id；调试用" example:"ckpt-b64cb049-85a8-433a-a5b7-fb5ad6d2b0f0"`
	CreatedAt    string   `json:"created_at,omitempty" dc:"消息创建时间" example:"2026-03-12T00:15:12+08:00"`
}

// AgentSessionRunTrace 表示某个历史 run 的快照与完整事件流。
type AgentSessionRunTrace struct {
	Snapshot *AgentRunSnapshot `json:"snapshot,omitempty" dc:"该 run 的最终快照"`
	Events   []AgentRunEvent   `json:"events,omitempty" dc:"该 run 的完整事件列表，按时间顺序排列"`
}

// AgentSessionDetail 表示某个历史会话的完整详情。
type AgentSessionDetail struct {
	Session  AgentSessionSummary    `json:"session" dc:"会话摘要"`
	Messages []AgentSessionMessage  `json:"messages,omitempty" dc:"该会话的消息列表，按时间顺序排列"`
	Runs     []AgentSessionRunTrace `json:"runs,omitempty" dc:"该会话下的 run 列表及其事件轨迹"`
}

// AgentSessionDetailReq 查看一个历史会话的完整详情。
type AgentSessionDetailReq struct {
	g.Meta       `path:"/v1/agent/{assistant_key}/sessions/{session_id}" tags:"Agent" method:"get" summary:"查看一个 agent 历史会话详情" dc:"返回该会话的消息列表、run 快照和完整 run 事件，供前端恢复历史对话与执行轨迹。"`
	AssistantKey string `json:"-" in:"path" param:"assistant_key" v:"required" dc:"顶层助手 key" example:"campus"`
	SessionID    string `json:"-" in:"path" param:"session_id" v:"required" dc:"历史会话 ID" example:"sess-a925e3c0-8f4b-4daf-bbe3-1885afd915c5"`
	UserID       string `json:"-" in:"header" param:"X-User-ID" v:"required" dc:"当前登录用户 UPN，请求头 X-User-ID" example:"122020255@link.cuhk.edu.cn"`
}

// AgentSessionDetailRes 返回一个历史会话的完整详情。
type AgentSessionDetailRes struct {
	Detail AgentSessionDetail `json:"detail" dc:"历史会话详情"`
}

// AgentSessionDeleteReq 删除一个历史会话。
type AgentSessionDeleteReq struct {
	g.Meta       `path:"/v1/agent/{assistant_key}/sessions/{session_id}" tags:"Agent" method:"delete" summary:"删除一个 agent 历史会话" dc:"默认做软删除，只从当前用户历史列表里移除该会话；queued、running 状态不允许删除；waiting_input 状态允许删除，但会同时放弃当前 pending interrupt、使 checkpoint 不可恢复，并把最近一次 run 记为 cancelled。后台 cleanup worker 会在保留期后物理清理 deleted 会话及其消息、run、run event。"`
	AssistantKey string `json:"-" in:"path" param:"assistant_key" v:"required" dc:"顶层助手 key" example:"campus"`
	SessionID    string `json:"-" in:"path" param:"session_id" v:"required" dc:"历史会话 ID" example:"sess-a925e3c0-8f4b-4daf-bbe3-1885afd915c5"`
	UserID       string `json:"-" in:"header" param:"X-User-ID" v:"required" dc:"当前登录用户 UPN，请求头 X-User-ID" example:"122020255@link.cuhk.edu.cn"`
}

// AgentSessionDeleteResult 表示历史会话删除结果。
type AgentSessionDeleteResult struct {
	Deleted bool `json:"deleted" dc:"是否已成功删除当前历史会话" example:"true"`
}

// AgentSessionDeleteRes 返回历史会话删除结果。
type AgentSessionDeleteRes struct {
	AssistantKey string                   `json:"assistant_key" dc:"顶层助手 key" example:"campus"`
	SessionID    string                   `json:"session_id" dc:"已删除的历史会话 ID" example:"sess-a925e3c0-8f4b-4daf-bbe3-1885afd915c5"`
	Result       AgentSessionDeleteResult `json:"result" dc:"删除结果"`
}

// AgentMemory 表示一个长期记忆条目。
type AgentMemory struct {
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

type AgentMemoriesReq struct {
	g.Meta       `path:"/v1/agent/{assistant_key}/memories" tags:"Agent" method:"get" summary:"查看当前用户长期记忆" dc:"返回当前 assistant_key 下、当前 X-User-ID 对应的长期记忆列表。顶层助手会在每次新建 run 或 resume run 前按需把这些长期记忆注入上下文。"`
	AssistantKey string `json:"-" in:"path" param:"assistant_key" v:"required" dc:"顶层助手 key，对应路由路径参数 assistant_key" example:"campus"`
	UserID       string `json:"-" in:"header" param:"X-User-ID" v:"required" dc:"当前登录用户 UPN，请求头 X-User-ID" example:"122020255@link.cuhk.edu.cn"`
	Limit        int    `json:"limit" in:"query" dc:"返回条数上限，默认 20" example:"20"`
}

type AgentMemoriesRes struct {
	AssistantKey string        `json:"assistant_key" dc:"当前顶层助手 key" example:"campus"`
	Items        []AgentMemory `json:"items" dc:"当前用户的长期记忆列表"`
}

type AgentMemoriesClearReq struct {
	g.Meta       `path:"/v1/agent/{assistant_key}/memories/clear" tags:"Agent" method:"post" summary:"清除当前用户长期记忆" dc:"默认清空当前用户全部长期记忆；如果传 category，则清空该分类；如果同时传 category 和 canonical_key，则只删除该条记忆。"`
	AssistantKey string `json:"-" in:"path" param:"assistant_key" v:"required" dc:"顶层助手 key，对应路由路径参数 assistant_key" example:"campus"`
	UserID       string `json:"-" in:"header" param:"X-User-ID" v:"required" dc:"当前登录用户 UPN，请求头 X-User-ID" example:"122020255@link.cuhk.edu.cn"`
	Category     string `json:"category,omitempty" dc:"可选，限定要清除的长期记忆分类" example:"location"`
	CanonicalKey string `json:"canonical_key,omitempty" dc:"可选，限定要清除的长期记忆稳定键；通常与 category 搭配使用" example:"dormitory_location"`
}

type AgentMemoriesClearResult struct {
	DeletedCount int64 `json:"deleted_count" dc:"本次删除的长期记忆条数" example:"1"`
}

type AgentMemoriesClearRes struct {
	AssistantKey string                   `json:"assistant_key" dc:"当前顶层助手 key" example:"campus"`
	Result       AgentMemoriesClearResult `json:"result" dc:"清理结果"`
}

var (
	AgentSessionsResExamples = goai.Examples{
		"list": {
			Value: &goai.Example{
				Summary: "查看顶层助手历史会话列表",
				Value: g.Map{
					"code":    0,
					"message": "",
					"data": g.Map{
						"assistant_key": "campus",
						"items": []g.Map{{
							"assistant_key":   "campus",
							"session_id":      "sess-a925e3c0-8f4b-4daf-bbe3-1885afd915c5",
							"title":           "VPN 连不上，顺便告诉我学生群组邮箱地址",
							"status":          "done",
							"active_path":     []string{"campus", "it", "campus_it_kb"},
							"last_run_id":     "run-8f4b6d3b",
							"last_run_status": "done",
							"created_at":      "2026-03-12T00:15:00+08:00",
							"updated_at":      "2026-03-12T00:15:12+08:00",
						}},
					},
				},
			},
		},
	}
	AgentSessionDetailResExamples = goai.Examples{
		"detail": {
			Value: &goai.Example{
				Summary: "查看某个历史会话的完整详情",
				Value: g.Map{
					"code":    0,
					"message": "",
					"data": g.Map{
						"detail": g.Map{
							"session": g.Map{
								"assistant_key":   "campus",
								"session_id":      "sess-a925e3c0-8f4b-4daf-bbe3-1885afd915c5",
								"title":           "VPN 连不上，顺便告诉我学生群组邮箱地址",
								"status":          "done",
								"last_run_id":     "run-8f4b6d3b",
								"last_run_status": "done",
							},
							"messages": []g.Map{{
								"id":         1,
								"role":       "user",
								"content":    "VPN 连不上，顺便告诉我学生群组邮箱地址。",
								"created_at": "2026-03-12T00:15:00+08:00",
							}, {
								"id":         2,
								"role":       "assistant",
								"content":    "针对 VPN 连接问题，请先确认统一身份认证客户端和 VPN 软件配置。",
								"created_at": "2026-03-12T00:15:12+08:00",
							}},
							"runs": []g.Map{{
								"snapshot": g.Map{
									"run_id":        "run-8f4b6d3b",
									"assistant_key": "campus",
									"run_status":    "done",
								},
								"events": []g.Map{{
									"event_id":   10,
									"event_type": "knowledge_retrieve_started",
									"message":    "开始检索知识库",
								}},
							}},
						},
					},
				},
			},
		},
	}
	AgentSessionDeleteResExamples = goai.Examples{
		"deleted": {
			Value: &goai.Example{
				Summary: "成功删除历史会话",
				Value: g.Map{
					"code":    0,
					"message": "",
					"data": g.Map{
						"assistant_key": "campus",
						"session_id":    "sess-a925e3c0-8f4b-4daf-bbe3-1885afd915c5",
						"result": g.Map{
							"deleted": true,
						},
					},
				},
			},
		},
	}
	AgentMemoriesResExamples = goai.Examples{
		"list": {
			Value: &goai.Example{
				Summary: "查看当前用户长期记忆",
				Value: g.Map{
					"code":    0,
					"message": "",
					"data": g.Map{
						"assistant_key": "campus",
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
	AgentMemoriesClearReqExample = g.Map{
		"category":      "location",
		"canonical_key": "dormitory_location",
	}
	AgentMemoriesClearResExamples = goai.Examples{
		"clear_specific": {
			Value: &goai.Example{
				Summary: "定向清除一条长期记忆",
				Value: g.Map{
					"code":    0,
					"message": "",
					"data": g.Map{
						"assistant_key": "campus",
						"result": g.Map{
							"deleted_count": 1,
						},
					},
				},
			},
		},
	}
)
