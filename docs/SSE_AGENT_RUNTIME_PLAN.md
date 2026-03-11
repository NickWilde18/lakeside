# SSE Agent Runtime Plan

## 目标

为 Lakeside 的多层 Agent 平台提供一个可观测、可恢复、适合长任务的前后端交互协议，避免当前阻塞式 `query` / `resume` 在长耗时场景下“超时且看不到内部状态”。

适用场景：

- 顶层 / 领域 Supervisor 路由
- knowledge 叶子 agent 多次检索、改写检索
- ITSM interrupt / resume
- 后续 coding、HR、OSA 等其他助手

## 核心设计

不要把现有接口直接改成“纯 token 流式输出”。

推荐使用：

- 同步创建 run
- 异步执行
- SSE 推送结构化事件
- 单独查询最终快照

## 推荐接口

### 1. 发起运行

`POST /v1/agent/{assistant_key}/runs`

请求体：

```json
{
  "message": "宿舍 WiFi 坏了，先告诉我怎么排查，再帮我报修。"
}
```

响应：

```json
{
  "assistant_key": "campus",
  "run_id": "run-8f4b6d3b",
  "session_id": "sess-a925e3c0-8f4b-4daf-bbe3-1885afd915c5",
  "status": "running"
}
```

### 2. 订阅事件流

`GET /v1/agent/{assistant_key}/runs/{run_id}/events`

返回类型：

- `Content-Type: text/event-stream`

### 3. 查询快照

`GET /v1/agent/{assistant_key}/runs/{run_id}`

用途：

- 首次进入详情页时拉一次快照
- SSE 断开后补状态
- 前端刷新页面时恢复状态

### 4. 继续中断

`POST /v1/agent/{assistant_key}/runs/{run_id}/resume`

请求体与当前 `resume` 基本一致，只是把 `run_id` 纳入资源路径。

### 5. 取消运行

`POST /v1/agent/{assistant_key}/runs/{run_id}/cancel`

适用于长时间知识检索、后续 coding agent、批量任务。

## 事件模型

不要只推文本，统一推结构化事件。

推荐字段：

```json
{
  "event_id": "evt-001",
  "run_id": "run-8f4b6d3b",
  "assistant_key": "campus",
  "session_id": "sess-a925e3c0-8f4b-4daf-bbe3-1885afd915c5",
  "path": ["campus", "it", "campus_it_kb"],
  "event_type": "knowledge_retrieve_started",
  "message": "开始检索知识库",
  "payload": {
    "kb_id": "69ecf4b8-e875-421f-9bfa-e2600056261a",
    "query": "VPN连接方法"
  },
  "created_at": "2026-03-11T11:41:59+08:00"
}
```

## 推荐事件类型

- `run_started`
- `agent_entered`
- `agent_completed`
- `knowledge_retrieve_started`
- `knowledge_retrieve_finished`
- `knowledge_answer_ready`
- `itsm_interrupt_emitted`
- `itsm_submitted`
- `run_completed`
- `run_failed`
- `run_cancelled`

## 前端展示建议

前端以时间线渲染：

1. 顶层进入 `campus`
2. 路由到 `it`
3. knowledge 检索第 1 次查询
4. knowledge 检索第 2 次查询
5. knowledge 回答就绪
6. 进入 `itsm`
7. 产生 `need_info`

这比“等一个长请求返回最终 JSON”更适合 agent 平台。

## 与当前响应结构的关系

当前已有：

- `steps[]`
- `interrupts[]`
- `result`

建议保留它们作为最终快照结构。

SSE 负责推送增量事件，最终 `GET /runs/{run_id}` 或 `run_completed` 事件里仍然返回完整快照。

## 后端落地建议

### 1. 新增运行实体

建议新增：

- `agent_runs`
- `agent_run_events`

其中：

- `agent_sessions` 仍表示长期会话
- `agent_runs` 表示一次 query / resume 触发的具体执行
- `agent_run_events` 表示 SSE 的事件源

### 2. 事件来源

优先复用当前 `agentplatform callback` 与现有 step 聚合点。

当前代码里已经有：

- agent callback
- knowledge retrieve debug 日志
- ITSM interrupt 聚合

下一步应把这些状态直接落为结构化 event，而不是只写日志。

### 3. SSE 实现方式

服务端实现：

- 每个 `run_id` 对应一个 event channel
- 新事件写 DB
- 同时推送给在线 SSE 订阅者
- 订阅中断后可通过 `Last-Event-ID` 或快照补齐

### 4. 超时与断线

SSE 连接断开不应影响 run 本身。

也就是说：

- run 生命周期独立于 SSE 连接
- 前端重连后继续看事件
- 前端刷新后从快照恢复

## 为什么不是只做流式文本

纯 token streaming 只能解决“回答字一个个出来”，不能解决：

- 路由到哪个 subagent
- 触发了几次 knowledge retrieve
- 是否进入 ITSM interrupt
- 当前卡在哪个阶段

Lakeside 的问题不是“模型输出太慢”这么简单，而是“平台内部状态不可见”。

## 迁移建议

分三步：

1. 新增 `runs` 与 `events`，保留现有 `query` / `resume`
2. 前端先接 SSE
3. 稳定后再把主入口从阻塞式 `query` 迁到 `runs`

## 当前已验证的必要性

当前联调已确认：

- knowledge 叶子 agent 会在单次请求内发起多次真实检索
- 顶层 / 领域 Supervisor 可能经历多次进入 / 返回
- 阻塞式请求在复杂问题上存在长时间无响应、用户不可见内部状态的问题

因此，SSE 不是“锦上添花”，而是后续产品化的必要能力。
