# Lakeside Agent Testing Guide

本文档对应当前 **session-first + render/actions** 主产品协议。

`render/actions` 的 NDJSON 负载直接对齐 **A2UI v0.8 catalog 语义**，但只负责 **中间聊天区**。

## 主产品入口

- `GET /v1/agent/{assistant_key}/render`
- `POST /v1/agent/{assistant_key}/actions`
- `GET /v1/agent/{assistant_key}/sessions`
- `POST /v1/agent/{assistant_key}/sessions`
- `DELETE /v1/agent/{assistant_key}/sessions/{session_id}`

默认约定：

- `assistant_key=campus`
- 请求头 `X-User-ID` 的值是 UPN
- 服务地址 `http://127.0.0.1:8011`

## 1. 前置检查

1. Redis 可连接。
2. `config/config.yaml` 至少配置：
   - `agent.redis.*`
   - `model.*`
   - `agents.roots/domains/leaves`
3. 如需测知识检索，配置 `agents.rag.baseURL` 并确保 RAG 服务可用。

## 2. 启动方式

单进程：

```bash
MODE=all go run main.go
```

分离模式：

```bash
# 终端1：仅 API
MODE=api go run main.go

# 终端2：仅 worker
MODE=worker go run main.go
```

## 3. render 空页面

未选中会话时，请求：

```bash
curl -N -sS 'http://127.0.0.1:8011/v1/agent/campus/render' \
  -H 'X-User-ID: 122020255@link.cuhk.edu.cn'
```

期望：

- 返回 `application/x-ndjson`
- 至少包含：`deleteSurface`、`surfaceUpdate`、`dataModelUpdate`、`beginRendering`
- `surfaceUpdate.components[*]` 使用标准 catalog 组件定义，例如 `Row`、`Column`、`Card`、`Text`、`Button`、`TextField`
- 只渲染聊天区，不包含左侧历史列表或右侧轨迹列
- 中间显示空态，不自动创建 session

## 4. actions 发送消息

直接发送第一条消息：

```bash
curl -N -sS -X POST 'http://127.0.0.1:8011/v1/agent/campus/actions' \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: 122020255@link.cuhk.edu.cn' \
  -d '{
    "userAction": {
      "name": "send_message",
      "surfaceId": "agent-canvas",
      "sourceComponentId": "composer-send",
      "timestamp": "2026-03-21T15:00:00Z",
      "context": {
        "message": "VPN 连不上，顺便告诉我学生群组邮箱地址。"
      }
    }
  }'
```

期望：

- 返回 NDJSON 流
- 早期消息里会写入新的 `meta.sessionId`
- 如果模型正常，会逐步返回助理回答
- 如果顶层或模块判断信息不足，会出现 `need_info` follow-up interrupt
- 如果没有模块高置信命中，会返回带“通用回答”标识的 fallback 内容
- 若触发 ITSM 中断，会出现 interrupt 数据模型和对应组件

## 5. sessions 列表 / 创建 / 删除

历史会话列表：

```bash
curl -sS 'http://127.0.0.1:8011/v1/agent/campus/sessions?limit=30' \
  -H 'X-User-ID: 122020255@link.cuhk.edu.cn'
```

显式创建空会话：

```bash
curl -sS -X POST 'http://127.0.0.1:8011/v1/agent/campus/sessions' \
  -H 'X-User-ID: 122020255@link.cuhk.edu.cn'
```

删除历史会话：

```bash
curl -sS -X DELETE 'http://127.0.0.1:8011/v1/agent/campus/sessions/<SESSION_ID>' \
  -H 'X-User-ID: 122020255@link.cuhk.edu.cn'
```

期望：

- queued / running 的会话不能删除
- waiting_input 会话允许删除，但删除后应无法继续 resume，且该 run 会被标记为 cancelled
- 空会话或已完成会话可删除
- 超过保留期的 deleted 会话应被后台 cleanup worker 物理删除，关联 messages / runs / run_events 也应被清理

## 6. follow_up / approval / cancel

当页面进入 interrupt 状态后，可继续通过 `actions` 提交。

补信息：

```bash
curl -N -sS -X POST 'http://127.0.0.1:8011/v1/agent/campus/actions' \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: 122020255@link.cuhk.edu.cn' \
  -d '{
    "userAction": {
      "name": "follow_up_submit",
      "surfaceId": "agent-canvas",
      "sourceComponentId": "interrupt-submit",
      "timestamp": "2026-03-21T15:10:00Z",
      "context": {
        "sessionId": "<SESSION_ID>",
        "runId": "<RUN_ID>",
        "interruptId": "<INTERRUPT_ID>",
        "answer": "道扬书院 C1010，WiFi 能连上但无法访问外网。"
      }
    }
  }'
```

确认提交：

```bash
curl -N -sS -X POST 'http://127.0.0.1:8011/v1/agent/campus/actions' \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: 122020255@link.cuhk.edu.cn' \
  -d '{
    "userAction": {
      "name": "approval_submit",
      "surfaceId": "agent-canvas",
      "sourceComponentId": "interrupt-approve",
      "timestamp": "2026-03-21T15:11:00Z",
      "context": {
        "sessionId": "<SESSION_ID>",
        "runId": "<RUN_ID>",
        "interruptId": "<INTERRUPT_ID>",
        "approved": true,
        "subject": "宿舍 WiFi 无法上网",
        "othersDesc": "多台设备都无法访问外网。"
      }
    }
  }'
```

取消当前执行：

```bash
curl -N -sS -X POST 'http://127.0.0.1:8011/v1/agent/campus/actions' \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: 122020255@link.cuhk.edu.cn' \
  -d '{
    "userAction": {
      "name": "cancel_turn",
      "surfaceId": "agent-canvas",
      "sourceComponentId": "cancel-run-btn",
      "timestamp": "2026-03-21T15:12:00Z",
      "context": {
        "sessionId": "<SESSION_ID>",
        "runId": "<RUN_ID>"
      }
    }
  }'
```

## 7. 高级轨迹

默认不展示高级轨迹。前端开启高级模式后，应改为单独拉：

```bash
curl -sS 'http://127.0.0.1:8011/v1/agent/campus/sessions/<SESSION_ID>' \
  -H 'X-User-ID: 122020255@link.cuhk.edu.cn'
```

期望：

- 返回该会话的 `messages` 与 `runs`
- `runs[*].events` 可用于渲染右侧时间线
- `render/actions` 本身不再负责整页轨迹列

## 8. 自动化测试

```bash
go test ./...
```

如需 live integration：

```bash
LAKESIDE_RUN_LIVE_TESTS=1 \
LAKESIDE_TEST_ASSISTANT_KEY=campus \
LAKESIDE_TEST_USER_ID=122020255@link.cuhk.edu.cn \
go test ./test/integration -run Live -v
```
