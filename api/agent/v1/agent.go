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

// AgentRunCreateReq 发起一次新的 agent run。
type AgentRunCreateReq struct {
	g.Meta       `path:"/v1/agent/{assistant_key}/runs" tags:"Agent" method:"post" summary:"发起新的 agent run" dc:"按 assistant_key 选择顶层助手入口，创建一次异步执行 run。若传 session_id，则表示在该历史会话里继续追问；不传则新建会话。前端应随后通过 snapshot 或 SSE 事件流跟踪执行过程。" example:"{\"message\":\"VPN 连不上，顺便告诉我学生群组邮箱地址\"}"`
	AssistantKey string `json:"-" in:"path" param:"assistant_key" v:"required" dc:"顶层助手 key，对应路由路径参数 assistant_key" example:"campus"`
	UserID       string `json:"-" in:"header" param:"X-User-ID" v:"required" dc:"当前登录用户 UPN，请求头 X-User-ID；Lakeside 会在服务端内部转换下游系统所需身份字段" example:"122020255@link.cuhk.edu.cn"`
	SessionID    string `json:"session_id,omitempty" dc:"可选。已有会话 ID；传入后会在该会话内继续创建新的 query run，而不是新建 session" example:"sess-a925e3c0-8f4b-4daf-bbe3-1885afd915c5"`
	Message      string `json:"message" v:"required" dc:"用户本轮输入内容" example:"VPN 连不上，顺便告诉我学生群组邮箱地址"`
}

// AgentRunCreateRes 表示 run 创建成功后的最小返回。
type AgentRunCreateRes struct {
	AssistantKey string `json:"assistant_key" dc:"当前顶层助手 key" example:"campus"`
	RunID        string `json:"run_id" dc:"新创建的 run ID" example:"run-8f4b6d3b"`
	SessionID    string `json:"session_id" dc:"本次 run 所属会话 ID；query 新建会话，resume 复用原会话" example:"sess-a925e3c0-8f4b-4daf-bbe3-1885afd915c5"`
	RunStatus    string `json:"run_status" dc:"当前 run 状态，初始一般为 queued 或 running" example:"queued"`
}

// AgentRunGetReq 获取一次 run 的快照。
type AgentRunGetReq struct {
	g.Meta       `path:"/v1/agent/{assistant_key}/runs/{run_id}" tags:"Agent" method:"get" summary:"查看 agent run 快照" dc:"返回当前 run 的完整快照，包括步骤、interrupt 和最终结果。"`
	AssistantKey string `json:"-" in:"path" param:"assistant_key" v:"required" dc:"顶层助手 key" example:"campus"`
	RunID        string `json:"-" in:"path" param:"run_id" v:"required" dc:"run ID" example:"run-8f4b6d3b"`
	UserID       string `json:"-" in:"header" param:"X-User-ID" v:"required" dc:"当前登录用户 UPN，请求头 X-User-ID" example:"122020255@link.cuhk.edu.cn"`
}

// AgentRunGetRes 返回单个 run 的完整快照。
type AgentRunGetRes struct {
	AgentRunSnapshot
}

// AgentRunResumeReq 继续一个 waiting_input 的 run。
type AgentRunResumeReq struct {
	g.Meta       `path:"/v1/agent/{assistant_key}/runs/{run_id}/resume" tags:"Agent" method:"post" summary:"继续一个 waiting_input 的 agent run" dc:"当前典型场景是继续 ITSM 子代理产生的 interrupt。前端只需要传 targets；session_id 和 checkpoint_id 由服务端根据 run_id 找回。"`
	AssistantKey string                          `json:"-" in:"path" param:"assistant_key" v:"required" dc:"顶层助手 key" example:"campus"`
	RunID        string                          `json:"-" in:"path" param:"run_id" v:"required" dc:"waiting_input 的 run ID" example:"run-8f4b6d3b"`
	UserID       string                          `json:"-" in:"header" param:"X-User-ID" v:"required" dc:"当前登录用户 UPN，请求头 X-User-ID" example:"122020255@link.cuhk.edu.cn"`
	Targets      map[string]*itsmv1.ResumeTarget `json:"targets" v:"required" dc:"继续当前流程所需的恢复输入集合，值类型为 lakeside.api.itsm.v1.ResumeTarget" example:"{\"6819cf6c-ea98-49d2-82b3-3e7cbcbc90b7\":{\"confirmed\":true}}"`
}

// AgentRunResumeRes 表示 resume 创建的新 run。
type AgentRunResumeRes struct {
	AssistantKey string `json:"assistant_key" dc:"当前顶层助手 key" example:"campus"`
	RunID        string `json:"run_id" dc:"新创建的 resume run ID" example:"run-01234567"`
	SessionID    string `json:"session_id" dc:"resume 复用的会话 ID" example:"sess-4f8e3652-30ff-4d84-99ea-5df7b359af80"`
	RunStatus    string `json:"run_status" dc:"当前 run 状态，初始一般为 queued 或 running" example:"queued"`
}

// AgentRunCancelReq 取消当前运行中的 run。
type AgentRunCancelReq struct {
	g.Meta       `path:"/v1/agent/{assistant_key}/runs/{run_id}/cancel" tags:"Agent" method:"post" summary:"取消当前运行中的 run" dc:"仅适用于 queued 或 running 的 run；已进入 waiting_input 的 run 不能通过此接口取消。"`
	AssistantKey string `json:"-" in:"path" param:"assistant_key" v:"required" dc:"顶层助手 key" example:"campus"`
	RunID        string `json:"-" in:"path" param:"run_id" v:"required" dc:"run ID" example:"run-8f4b6d3b"`
	UserID       string `json:"-" in:"header" param:"X-User-ID" v:"required" dc:"当前登录用户 UPN，请求头 X-User-ID" example:"122020255@link.cuhk.edu.cn"`
}

// AgentRunCancelResult 表示 run 取消结果。
type AgentRunCancelResult struct {
	Cancelled bool `json:"cancelled" dc:"是否已发出取消请求" example:"true"`
}

// AgentRunCancelRes 表示 run 取消接口返回。
type AgentRunCancelRes struct {
	AssistantKey string               `json:"assistant_key" dc:"当前顶层助手 key" example:"campus"`
	RunID        string               `json:"run_id" dc:"被取消的 run ID" example:"run-8f4b6d3b"`
	Result       AgentRunCancelResult `json:"result" dc:"取消结果"`
}

// AgentRunEventsReq 订阅一次 run 的 SSE 事件流。
type AgentRunEventsReq struct {
	g.Meta       `path:"/v1/agent/{assistant_key}/runs/{run_id}/events" tags:"Agent" method:"get" summary:"订阅 run 的 SSE 事件流" dc:"返回 text/event-stream。支持浏览器 Last-Event-ID 断线重连；事件流会先回放数据库中已有事件，再实时推送后续事件。事件覆盖 run 生命周期、domain 规划阶段、knowledge 检索/生成阶段和 ITSM 中断阶段。"`
	AssistantKey string `json:"-" in:"path" param:"assistant_key" v:"required" dc:"顶层助手 key" example:"campus"`
	RunID        string `json:"-" in:"path" param:"run_id" v:"required" dc:"run ID" example:"run-8f4b6d3b"`
	UserID       string `json:"-" in:"header" param:"X-User-ID" v:"required" dc:"当前登录用户 UPN，请求头 X-User-ID" example:"122020255@link.cuhk.edu.cn"`
	LastEventID  int64  `json:"-" in:"header" param:"Last-Event-ID" dc:"可选。SSE 断线重连时从该事件 ID 之后继续回放；浏览器 EventSource 重连时会自动携带。"`
	LastEventQID int64  `json:"last_event_id,omitempty" in:"query" dc:"可选。与 Last-Event-ID 语义一致，便于 curl 或网关转发场景手动传参。" example:"12"`
}

// AgentRunEventsRes 是 SSE 事件流接口的占位响应结构。
type AgentRunEventsRes struct {
	Placeholder string `json:"placeholder,omitempty" dc:"SSE 接口返回 text/event-stream，本结构仅用于满足 GoFrame XxxRes 命名要求"`
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
	g.Meta       `path:"/v1/agent/{assistant_key}/sessions/{session_id}" tags:"Agent" method:"delete" summary:"删除一个 agent 历史会话" dc:"默认做软删除，只从当前用户历史列表里移除该会话；为了避免破坏当前流程，不允许删除 queued、running 或 waiting_input 状态的会话。"`
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
	AgentRunCreateReqExample = g.Map{
		"message": "VPN 连不上，顺便告诉我学生群组邮箱地址。",
	}
	AgentRunCreateResExamples = goai.Examples{
		"created": {
			Value: &goai.Example{
				Summary: "成功创建一个新的 run",
				Value: g.Map{
					"code":    0,
					"message": "",
					"data": g.Map{
						"assistant_key": "campus",
						"run_id":        "run-8f4b6d3b",
						"session_id":    "sess-a925e3c0-8f4b-4daf-bbe3-1885afd915c5",
						"run_status":    "queued",
					},
				},
			},
		},
	}
	AgentRunGetResExamples = goai.Examples{
		"waiting_input": {
			Value: &goai.Example{
				Summary: "knowledge + ITSM interrupt 的 run 快照",
				Value: g.Map{
					"code":    0,
					"message": "",
					"data": g.Map{
						"run_id":        "run-8f4b6d3b",
						"assistant_key": "campus",
						"run_status":    "waiting_input",
						"status":        "need_info",
						"session_id":    "sess-a925e3c0-8f4b-4daf-bbe3-1885afd915c5",
						"checkpoint_id": "ckpt-b64cb049-85a8-433a-a5b7-fb5ad6d2b0f0",
						"active_path":   []string{"campus", "it", "itsm"},
						"started_at":    "2026-03-11T21:30:00+08:00",
						"finished_at":   "2026-03-11T21:30:12+08:00",
						"steps": []g.Map{{
							"path":    []string{"campus", "it", "campus_it_kb"},
							"kind":    "knowledge",
							"message": "如果是宿舍 WiFi 无法访问校内资源，可先确认是否是设备问题或局部故障。若需要继续报修，请补充地点与故障现象。",
						}, {
							"path": []string{"campus", "it", "itsm"},
							"kind": "itsm_interrupt",
							"interrupts": []g.Map{{
								"interrupt_id":   "83120df4-a30d-44a4-b958-98a94689b8c7",
								"type":           "need_info",
								"prompt":         "信息还不完整，请补充：问题描述。补充说明：请提供寝室具体位置（楼号、房间号）及故障现象。",
								"missing_fields": []string{"othersDesc"},
							}},
						}},
					},
				},
			},
		},
	}
	AgentRunResumeReqExamples = goai.Examples{
		"itsm_need_info": {
			Value: &goai.Example{
				Summary: "继续 ITSM 补信息阶段",
				Value: g.Map{
					"targets": g.Map{
						"83120df4-a30d-44a4-b958-98a94689b8c7": g.Map{
							"answer": "道扬书院C1010，WiFi能搜到但连接后无法上网，宿舍里多台设备都受影响。",
						},
					},
				},
			},
		},
	}
	AgentRunResumeResExamples = goai.Examples{
		"created": {
			Value: &goai.Example{
				Summary: "成功创建 resume run",
				Value: g.Map{
					"code":    0,
					"message": "",
					"data": g.Map{
						"assistant_key": "campus",
						"run_id":        "run-01234567",
						"session_id":    "sess-4f8e3652-30ff-4d84-99ea-5df7b359af80",
						"run_status":    "queued",
					},
				},
			},
		},
	}
	AgentRunCancelResExamples = goai.Examples{
		"cancelled": {
			Value: &goai.Example{
				Summary: "成功发出取消请求",
				Value: g.Map{
					"code":    0,
					"message": "",
					"data": g.Map{
						"assistant_key": "campus",
						"run_id":        "run-8f4b6d3b",
						"result": g.Map{
							"cancelled": true,
						},
					},
				},
			},
		},
	}
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
