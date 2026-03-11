# Lakeside Agent Testing Guide (Current Runtime)

本文档对应当前 `run + worker + SSE` 架构。

主入口：

- `POST /v1/agent/{assistant_key}/runs`
- `GET /v1/agent/{assistant_key}/runs/{run_id}`
- `GET /v1/agent/{assistant_key}/runs/{run_id}/events`
- `POST /v1/agent/{assistant_key}/runs/{run_id}/resume`
- `POST /v1/agent/{assistant_key}/runs/{run_id}/cancel`

默认约定：

- `assistant_key=campus`
- 请求头 `X-User-ID` 的值为 UPN
- 服务地址 `http://127.0.0.1:8011`

## 1. 前置检查

1. Redis 可连接（本项目必需依赖）。
2. `config/config.yaml` 至少配置：
   - `agent.redis.*`
   - `model.*`
   - `agents.roots/domains/leaves`
3. 可选：若要测知识检索，配置 `agents.rag.baseURL` 并确保 RAG 服务可用。

## 2. 启动方式

方式 A：单进程（推荐联调）

```bash
MODE=all go run main.go
```

方式 B：多进程（生产形态）

```bash
# 终端1：仅 API
MODE=api go run main.go

# 终端2：仅 worker
MODE=worker go run main.go
```

## 3. 基础流程（create -> events -> snapshot）

创建 run：

```bash
curl -sS -X POST 'http://127.0.0.1:8011/v1/agent/campus/runs' \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: 122020255@link.cuhk.edu.cn' \
  -d '{"message":"宿舍 WiFi 连接后无法上网，先给我排查建议，再帮我报修。"}'
```

记录返回中的 `run_id`。

订阅事件：

```bash
curl -N -sS 'http://127.0.0.1:8011/v1/agent/campus/runs/<RUN_ID>/events' \
  -H 'X-User-ID: 122020255@link.cuhk.edu.cn'
```

查询快照：

```bash
curl -sS 'http://127.0.0.1:8011/v1/agent/campus/runs/<RUN_ID>' \
  -H 'X-User-ID: 122020255@link.cuhk.edu.cn'
```

期望：

- `run_status` 进入 `queued -> running -> waiting_input|done|failed|cancelled`
- `waiting_input` 时有 `interrupts`
- `done` 时有 `result`
- SSE 终态事件与 `run_status` 对应：`run_waiting_input` / `run_completed` / `run_failed` / `run_cancelled`

## 4. resume 流程

当 `run_status=waiting_input` 后，取 `interrupt_id` 调用：

```bash
curl -sS -X POST 'http://127.0.0.1:8011/v1/agent/campus/runs/<RUN_ID>/resume' \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: 122020255@link.cuhk.edu.cn' \
  -d '{"targets":{"<INTERRUPT_ID>":{"confirmed":true}}}'
```

期望：

- 返回新的 `run_id`
- 新 run 可继续 `events/snapshot`

## 5. cancel 流程

取消接口：

```bash
curl -sS -X POST 'http://127.0.0.1:8011/v1/agent/campus/runs/<RUN_ID>/cancel' \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: 122020255@link.cuhk.edu.cn' \
  -d '{}'
```

期望：

- 返回 `data.result.cancelled=true`
- 快照最终 `run_status=cancelled`
- SSE 出现 `run_cancelled`

说明：

- `queued` 可直接取消（数据库原子更新）
- `running` 通过 Redis cancel 广播跨实例取消

## 6. SSE 断线重连

记录上次收到的事件 `id`，重连时可以传 `Last-Event-ID`（或 query 参数 `last_event_id`）：

```bash
curl -N -sS 'http://127.0.0.1:8011/v1/agent/campus/runs/<RUN_ID>/events' \
  -H 'X-User-ID: 122020255@link.cuhk.edu.cn' \
  -H 'Last-Event-ID: 12'
```

期望：服务会先补发 `id>12` 的历史事件，再继续实时推送。

也可用 query 方式（便于某些网关或调试工具）：

```bash
curl -N -sS 'http://127.0.0.1:8011/v1/agent/campus/runs/<RUN_ID>/events?last_event_id=12' \
  -H 'X-User-ID: 122020255@link.cuhk.edu.cn'
```

## 7. Redis Streams 可靠性检查

查看队列与 pending：

```bash
redis-cli XINFO GROUPS lakeside:interactive:runtime:agent:runs:v1
redis-cli XPENDING lakeside:interactive:runtime:agent:runs:v1 lakeside-agent-workers
```

期望：

- 正常运行时，`pending` 会被 worker 持续清理
- worker 重启后，pending 会被 `XAUTOCLAIM` 回收再处理

语义说明：

- 仅 handler 成功才 `XACK`
- handler 失败不 `XACK`，消息保留 pending，后续可 reclaim

## 8. 自动化测试

```bash
go test ./...
```

如需 live integration（依赖本地已启动服务与可用外部依赖）：

```bash
LAKESIDE_RUN_LIVE_TESTS=1 \
LAKESIDE_TEST_ASSISTANT_KEY=campus \
LAKESIDE_TEST_USER_ID=122020255@link.cuhk.edu.cn \
go test ./test/integration -run Live -v
```
