# Lakeside

Lakeside 当前是一个分层 Agent 平台。

已落地结构：

- 顶层助手：`campus`
- 领域助手：`it`
- 叶子 agent：`itsm`、`campus_it_kb`、`campus_it_kb_for_itso_student_assistant`

前端主产品协议已经切到 **session-first + server-driven UI**。

当前 `render/actions` 的 NDJSON 消息对齐 **A2UI v0.8 catalog 语义**，但 A2UI 只负责 **中间对话区 chat surface**。页面壳子、左侧历史会话和右侧高级轨迹仍由前端平台自己渲染。

## 主产品入口

主产品页面统一使用：

- `GET /v1/agent/{assistant_key}/render`
- `POST /v1/agent/{assistant_key}/actions`
- `GET /v1/agent/{assistant_key}/sessions`
- `POST /v1/agent/{assistant_key}/sessions`
- `DELETE /v1/agent/{assistant_key}/sessions/{session_id}`
- `GET /v1/agent/{assistant_key}/memories`
- `POST /v1/agent/{assistant_key}/memories/clear`

当前仍保留：
- 内部 `run + run_events` 调试模型，用于高级轨迹和问题排查

## 页面协议

`render/actions` 返回 `application/x-ndjson`。

消息结构对齐 A2UI v0.8：

当前使用的消息类型：

- `deleteSurface`
- `surfaceUpdate`
- `dataModelUpdate`
- `beginRendering`

后端负责输出聊天区所需的 catalog 组件树和数据模型；前端 agent 模块负责：

- 渲染页面壳子
- 通过 `GET /sessions` 管理左侧历史会话
- 通过 `GET /sessions/{session_id}` 管理右侧高级轨迹
- 在中间聊天区消费 `render/actions` 并回传用户动作

当前动作名包括：

- `send_message`
- `follow_up_submit`
- `approval_submit`
- `cancel_turn`

## 会话模型

主产品以 `session + messages` 为事实来源。

语义约定：

- `session`：一次对话
- `messages`：该会话下的历史消息
- `memories`：长期事实记忆，只作为增强注入，不替代消息历史

当前前端恢复历史页面时：

1. 先通过 `GET /sessions` 获取左侧历史列表
2. 中间聊天区通过 `GET /render` 获取 chat surface 快照
3. 选中会话后，`render?session_id=...` 恢复历史消息和待处理 interrupt
4. 用户动作统一通过 `POST /actions` 回传
5. 如开启高级轨迹，前端再通过 `GET /sessions/{session_id}` 拉取 run 详情

历史会话删除语义：

- `queued` / `running`：不允许删除
- `waiting_input`：允许删除，但会同时放弃 pending interrupt、清空 checkpoint 并把最近一次 run 记为 `cancelled`
- `done` / `failed` / `cancelled`：允许删除
- 用户侧删除先做软删除；后台 cleanup worker 会定期物理清理超过保留期的 `deleted` 会话及其 `messages / runs / run_events`

空白会话可以通过 `POST /sessions` 显式创建，但当前产品也允许用户直接发送第一条消息，让服务端在创建首个 turn 时自动建会话。

## 架构分层

- `internal/service/agentplatform/`
  - 通用平台层：registry、runtime、checkpoint、session、memory、response
- `internal/service/rootassistant/`
  - 顶层助手，例如 `campus`
- `internal/service/domainassistant/`
  - 领域助手，例如 `it`
- `internal/service/leafagent/`
  - 叶子 agent，目前包括 `itsm` 与 `knowledge`
- `internal/infra/`
  - 外部依赖适配层，例如 `ragclient`、`uniauth`、`rediskit`

当前顶层/领域层编排继续优先使用 Eino 官方组件；叶子层保留各自实现。

当前运行时语义：

- `campus` 是薄根节点，只做模块 assessment、fan-out 和结果合并
- `it` 是第一个领域模块，内部执行 `assessment + planner + workflow`
- `knowledge` 作为 read tool 使用
- `itsm` 作为 write / interruptible agent 使用
- 无模块命中时，`campus` 返回明确标注的通用 fallback 回答
- 模块或顶层判断信息不足时，会直接发起 follow-up interrupt

## 当前行为

一次用户动作可能触发一个新的后台 run，但这个 run 已经不是主产品页面直接消费的对象。

当前主页面看到的是：

- 左侧前端历史列表
- 中间聊天区历史消息与流式回复
- 中间聊天区 interrupt 卡片
- 可选的右侧高级轨迹

对 `campus` 来说，一次执行通常仍可能包含多个后端步骤，例如：

1. 先调用 knowledge 叶子 agent 返回排查建议
2. 再调用 `itsm` 进入 `need_info` 或 `need_confirm`

当未来接入多个领域模块时，`campus` 会先做模块 assessment，再按 `read` 并行、`write` 串行的方式执行。

只是这些步骤默认不再直接暴露成前端的主渲染协议。

## 运行态约定

前端显示处理中时只认：

- `queued`
- `running`

以下状态均视为非运行态：

- `waiting_input`
- `done`
- `failed`
- `cancelled`

## 身份语义

对外请求头仍然叫：

- `X-User-ID`

但其值语义统一为：

- `UPN`

当前落地方式：

- 调知识库 RAG 时，把 `X-User-ID` 透传为 `x-user-upn`
- 调 ITSM 前，通过 `UniAuth` 把 UPN 换成下游需要的 `employeeId`

## 持久化

SQLite：

- `agent_sessions`
- `agent_messages`
- `agent_memories`
- `agent_runs`
- `agent_run_events`

其中：

- `agent_sessions + agent_messages` 是主产品恢复历史页面的事实来源
- `agent_memories` 是长期记忆增强层
- `agent_runs + agent_run_events` 主要服务调试和高级轨迹

Redis：

- 顶层助手 checkpoint：`lakeside:interactive:rootassistant:checkpoint:v1:`
- ITSM checkpoint：`lakeside:interactive:it:subagent:itsm:checkpoint:v1:`
- ITSM 幂等：`lakeside:interactive:it:subagent:itsm:idempotency:v1:`
- 共享 embedding cache：`lakeside:shared:embedding:cache:v1:`
- run 队列（Streams）：`agents.runtime.streamKey`
- run 事件唤醒（Pub/Sub）：`agents.runtime.pubsubChannel`
- run 取消广播（Pub/Sub）：`agents.runtime.pubsubChannel + ":cancel"`

Redis 在本仓库中是必需依赖。

## 高级轨迹

高级轨迹默认关闭。

当前产品策略：

- 默认聊天页面不显示执行轨迹
- 用户在前端 settings 中打开开关后，前端会给 `render/actions` 传 `advanced_trace=1`
- 后端才会把 `agent_run_events` 映射成高级轨迹内容

高级轨迹内容由内部 `agent_run_events` 映射生成，前端通过 `GET /sessions/{session_id}` 获取，不再单独暴露公开 `/runs*` HTTP 接口。

## 知识库接入

Lakeside 不负责知识库管理，只复用独立 RAG 服务：

- `POST /v1/retrieve`
- `POST /v1/documents/batch_get`

knowledge 叶子 agent 负责：

1. 按固定 `kb_id` 或 `kb_id[]` 检索
2. 使用 Eino 官方检索组件扩召回
3. 补齐文件名
4. 基于检索片段生成回答
5. 返回来源与 `snippet`

## ITSM 接入

`itsm` 叶子 agent 当前保留自定义可中断状态机：

1. 抽取草稿
2. 缺信息时 interrupt
3. 二次确认时 interrupt
4. 用户确认后提交下游 ITSM

在主产品页面里，这些中断会通过 `render/actions` 映射成表单和确认卡片。

## 启动

```bash
go run main.go
```

运行模式：

- `all`：HTTP + worker（默认）
- `api`：仅 HTTP
- `worker`：仅 worker

默认地址：

- `http://127.0.0.1:8011`
- OpenAPI: `http://127.0.0.1:8011/api.json`
- Swagger: `http://127.0.0.1:8011/swagger/`

## 测试

```bash
go test ./...
```

联调流程见：[TESTING.md](./TESTING.md)
