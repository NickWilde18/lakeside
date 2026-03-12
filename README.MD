# Lakeside

Lakeside 现在是一个多层 Agent 平台，而不是单一的 IT 助手服务。

当前已落地的树形结构：

- 顶层助手：`campus`
- 领域助手：`it`
- 叶子 agent：`itsm`、`campus_it_kb`、`campus_it_kb_for_itso_student_assistant`

主入口统一为：

- `POST /v1/agent/{assistant_key}/runs`
- `GET /v1/agent/{assistant_key}/runs/{run_id}`
- `GET /v1/agent/{assistant_key}/runs/{run_id}/events`
- `POST /v1/agent/{assistant_key}/runs/{run_id}/resume`
- `POST /v1/agent/{assistant_key}/runs/{run_id}/cancel`
- `GET /v1/agent/{assistant_key}/memories`
- `POST /v1/agent/{assistant_key}/memories/clear`

当前仍保留 `/v1/itsm/agent/*` 作为叶子 agent 调试入口，不再作为主产品入口。

`POST /v1/agent/{assistant_key}/runs` 请求体当前约定：

- `message`：必填，用户本轮输入
- `session_id`：可选；传入表示复用现有会话，不传表示新建会话

## 架构分层

- `internal/service/agentplatform/`
  - 通用平台层：registry、runtime、checkpoint、session、memory、response
- `internal/service/rootassistant/`
  - 顶层助手，例如 `campus`；当前统一使用 Eino 官方 `adk/prebuilt/supervisor`
- `internal/service/domainassistant/`
  - 领域助手，例如 `it`；当前统一使用 Eino 官方 `adk/prebuilt/supervisor`
- `internal/service/leafagent/`
  - 叶子 agent，目前包括 `itsm` 与 `knowledge`
- `internal/infra/`
  - 外部依赖适配层，例如 `ragclient`、`uniauth`、`rediskit`

## 当前行为

一次 run 允许出现多个 step。例如：

1. 先调用 knowledge 叶子 agent 返回排查建议
2. 再调用 `itsm` 进入 `need_info` 或 `need_confirm`

因此顶层响应不再只表达“命中了哪个 agent”，而是表达：

- `assistant_key`
- `active_path`
- `steps[]`
- `interrupts[]`
- `result`
- `run_status`

其中：

- `steps[]` 用于展示本轮依次发生了什么
- `result.sources[]` 用于返回知识来源，包含 `filename`、`doc_id`、`node_id`、`snippet`、`score`
- `interrupts[]` 当前主要来自 `itsm` 叶子 agent
- `run_status` 用于表达运行态，例如 `queued`、`running`、`waiting_input`、`done`、`failed`、`cancelled`

运行态约定：

- 仅 `queued`、`running` 视为处理中
- `waiting_input`、`done`、`failed`、`cancelled` 均视为非运行态

## 会话续聊与上下文

同一对话里继续提问，前端应复用已有 `session_id` 调用 `POST /runs`，后端会在同一个会话下创建新 run。

当前上下文注入由两部分组成：

- 长期记忆：`agent_memories`
- 近程会话上下文：同会话最近若干条消息（当前默认最近 12 条并按长度截断）

只有在用户显式开启新对话时，才不传 `session_id` 新建会话。

## 配置

运行时读取：

- `config/config.yaml`

注释示例：

- `config/config.example.yaml`

当前配置已经切到树形：

- `agents.roots[]`
- `agents.domains[]`
- `agents.leaves[]`

根/领域助手的路由提示词模板也都在配置里：

- `agents.roots[].instructionTemplate`
- `agents.domains[].instructionTemplate`

运行时会把这些占位符替换成当前会话值：

- `{assistant_key}`
- `{user_upn}`
- `{assistant_context}`
- `{latest_user_message}`
- `{child_catalog}`

## 持久化

SQLite：

- `agent_sessions`
- `agent_messages`
- `agent_memories`
- `agent_runs`
- `agent_run_events`

Redis：

- 顶层助手 checkpoint：`lakeside:interactive:rootassistant:checkpoint:v1:`
- ITSM checkpoint：`lakeside:interactive:it:subagent:itsm:checkpoint:v1:`
- ITSM 幂等：`lakeside:interactive:it:subagent:itsm:idempotency:v1:`
- 共享 embedding cache：`lakeside:shared:embedding:cache:v1:`
- run 队列（Streams）：`agents.runtime.streamKey`（默认 `lakeside:interactive:runtime:agent:runs:v1`）
- run 事件唤醒（Pub/Sub）：`agents.runtime.pubsubChannel`
- run 取消广播（Pub/Sub）：`agents.runtime.pubsubChannel + ":cancel"`

Redis 在本仓库中是必需依赖。

## SSE 运行时

顶层入口已经切到 `run + snapshot + SSE events`：

1. `POST /v1/agent/{assistant_key}/runs`
   - 立即返回 `run_id`、`session_id`
   - 请求体支持可选 `session_id`：续聊复用会话，不传则新建会话
   - run 会进入 Redis Streams 队列，由 worker 异步消费
2. `GET /v1/agent/{assistant_key}/runs/{run_id}/events`
   - 返回 `text/event-stream`
   - 支持 `Last-Event-ID`
   - 先回放 DB 事件，再结合本机订阅 + Redis Pub/Sub 推送后续事件
3. `GET /v1/agent/{assistant_key}/runs/{run_id}`
   - 返回当前快照
4. `POST /v1/agent/{assistant_key}/runs/{run_id}/resume`
   - 继续 `waiting_input` 的运行
5. `POST /v1/agent/{assistant_key}/runs/{run_id}/cancel`
   - 取消长任务
   - `queued` 可直接取消
   - `running` 通过 Redis cancel 广播跨实例通知 worker 取消

当前已实现的结构化事件包括：

- `run_started`
- `domain_plan_started`
- `domain_plan_ready`
- `domain_execute_started`
- `domain_supervisor_fallback`
- `run_waiting_input`
- `agent_entered`
- `agent_completed`
- `knowledge_retrieve_started`
- `knowledge_retrieve_finished`
- `knowledge_answer_chunk`
- `knowledge_answer_ready`
- `itsm_interrupt_emitted`
- `itsm_done`
- `run_completed`
- `run_failed`
- `run_cancelled`

其中领域计划相关事件的 payload 会携带：

- `steps`：执行顺序
- `step_details[]`：每个 step 的 `agent_key` 与 `reason`，用于解释路由决策原因

队列可靠性语义：

- worker 仅在 handler 成功后 `XACK`，失败消息会保留 pending
- worker 启动后会通过 `XAUTOCLAIM` 回收 pending，避免崩溃后消息遗失
- `TryStartRun` 保证同一个 run 只会被一个 worker 原子抢占执行

断线重连约定：

- 支持标准 `Last-Event-ID` 请求头
- 同时支持 query 参数 `last_event_id`（便于网关或调试工具）

## 身份语义

对外请求头仍然叫：

- `X-User-ID`

但其值语义统一为：

- `UPN`

当前落地方式：

- 调知识库 RAG 时，把 `X-User-ID` 透传为 `x-user-upn`
- 调 ITSM 前，通过 `UniAuth` 把 UPN 换成下游需要的 `employeeId`

## 知识库接入

Lakeside 不负责知识库管理，只复用独立 RAG 服务的检索接口：

- `POST /v1/retrieve`
- `POST /v1/documents/batch_get`

knowledge 叶子 agent 负责：

1. 按固定 `kb_id` 或 `kb_id[]` 检索
2. 使用 Eino 官方 `flow/retriever/multiquery` 做查询改写与扩召回
3. 补齐文件名
4. 基于检索片段生成回答
5. 返回来源与 `snippet`

知识库上传、重建索引、权限同步仍然在独立 RAG 服务里完成，不在 Lakeside 内实现。

## ITSM 接入

`itsm` 叶子 agent 保留当前自定义可中断状态机：

1. 抽取草稿
2. 缺信息时 interrupt
3. 二次确认时 interrupt
4. 用户确认后提交下游 ITSM

当前也保留：

- 提交幂等保护
- 相似问题聚合与服务级别升级

中断取消约定：

- `need_info` 与 `need_confirm` 阶段都支持通过 `resume.targets[interrupt_id].confirmed=false` 取消当前工单流程

## 启动

```bash
go run main.go
```

运行模式（可通过环境变量 `MODE` 或 `server.mode` 指定）：

- `all`：HTTP + worker（默认）
- `api`：仅 HTTP
- `worker`：仅 worker

默认地址：

- `http://127.0.0.1:8011`
- OpenAPI: `http://127.0.0.1:8011/api.json`
- Swagger: `http://127.0.0.1:8011/swagger/`

## 测试

单元测试：

```bash
go test ./...
```

联调说明见：

- [TESTING.md](/home/nickw/lakeside/TESTING.md)

设计文档见：

- [SSE_AGENT_RUNTIME_PLAN.md](/home/nickw/lakeside/docs/SSE_AGENT_RUNTIME_PLAN.md)
- [KNOWLEDGE_AGENT_DESIGN_GUIDELINES.md](/home/nickw/lakeside/docs/KNOWLEDGE_AGENT_DESIGN_GUIDELINES.md)
