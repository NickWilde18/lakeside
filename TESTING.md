# Assistant Testing Guide

本文档整理了顶层 `IT 小助手` 的测试方法。测试时优先走：

- `POST /v1/assistant/query`
- `POST /v1/assistant/resume`

旧的 `/v1/itsm/agent/*` 只作为子代理兼容入口，不再作为主测试路径。

## 启动服务

```bash
gf run main.go
```

默认服务地址：

- `http://127.0.0.1:8011`
- OpenAPI: `http://127.0.0.1:8011/api.json`
- Swagger: `http://127.0.0.1:8011/swagger/`

## 1. 发起建单对话

```bash
curl -sS -m 240 -X POST http://127.0.0.1:8011/v1/assistant/query \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: 122020255' \
  -d '{"message":"道扬书院C1010 WiFi坏了，帮我报修。"}'
```

预期：

- `data.active_agent = "itsm"`
- `data.status = "need_info"` 或 `need_confirm`
- 返回 `session_id`
- 返回 `checkpoint_id`
- 返回 `interrupts[0].interrupt_id`

## 2. 补充信息

把上一步返回的 `session_id`、`checkpoint_id`、`interrupt_id` 带回去：

```bash
curl -sS -m 240 -X POST http://127.0.0.1:8011/v1/assistant/resume \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: 122020255' \
  -d '{
    "session_id":"sess-xxx",
    "checkpoint_id":"ckpt-xxx",
    "targets":{
      "interrupt-id-xxx":{
        "answer":"服务级别填3。WiFi能连上但无法上网，宿舍里所有设备都受影响。"
      }
    }
  }'
```

预期：

- `data.active_agent = "itsm"`
- `data.status = "need_confirm"` 或 `done`

## 3. 确认提交

```bash
curl -sS -m 240 -X POST http://127.0.0.1:8011/v1/assistant/resume \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: 122020255' \
  -d '{
    "session_id":"sess-xxx",
    "checkpoint_id":"ckpt-xxx",
    "targets":{
      "interrupt-id-xxx":{
        "confirmed":true
      }
    }
  }'
```

预期：

- `data.status = "done"`
- `data.result.success = true`
- `data.result.ticket_no` 有值

## 4. 取消提交

```bash
curl -sS -m 240 -X POST http://127.0.0.1:8011/v1/assistant/resume \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: 122020255' \
  -d '{
    "session_id":"sess-xxx",
    "checkpoint_id":"ckpt-xxx",
    "targets":{
      "interrupt-id-xxx":{
        "confirmed":false
      }
    }
  }'
```

预期：

- `data.status = "done"`
- `data.result.success = false`

## 5. 测顶层 assistant 自己的回答

```bash
curl -sS -m 120 -X POST http://127.0.0.1:8011/v1/assistant/query \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: 122020255' \
  -d '{"message":"你现在支持哪些能力？"}'
```

预期：

- `data.active_agent = "assistant"`
- `data.status = "done"`

## 6. 自动化测试

普通回归：

```bash
go test ./...
```

如果本地服务已经常驻在 `8011`，可以跑 live integration test：

```bash
LAKESIDE_RUN_LIVE_TESTS=1 \
LAKESIDE_TEST_USER_ID=122020255 \
go test ./test/integration -run Live -v
```

如果要测 `resume`：

```bash
LAKESIDE_RUN_LIVE_TESTS=1 \
LAKESIDE_TEST_USER_ID=122020255 \
LAKESIDE_LIVE_RESUME_BODY='{"session_id":"sess-xxx","checkpoint_id":"ckpt-xxx","targets":{"interrupt-id":{"confirmed":true}}}' \
go test ./test/integration -run Resume -v
```

## 7. 查看和清除长期记忆

查看当前用户长期记忆：

```bash
curl -sS http://127.0.0.1:8011/v1/assistant/memories \
  -H 'X-User-ID: 122020255'
```

按分类和键定向清除：

```bash
curl -sS -X POST http://127.0.0.1:8011/v1/assistant/memories/clear \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: 122020255' \
  -d '{"category":"location","canonical_key":"dormitory_location"}'
```

清空当前用户全部长期记忆：

```bash
curl -sS -X POST http://127.0.0.1:8011/v1/assistant/memories/clear \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: 122020255' \
  -d '{}'
```

如果要直接查 SQLite：

```bash
sqlite3 runtime/assistant.db "select id,user_code,category,canonical_key,content,confidence,status from assistant_memories order by id desc;"
```

## 8. 日志观察点

出现慢响应时，优先看这些日志：

- `assistant query started`
- `assistant resume started`
- `assistant agent callback start/end`
- `itsm extractor started`
- `assistant query completed`

如果 `curl` 自己超时，服务端通常会看到 `context canceled`，这表示客户端先断开了，不一定是服务端崩了。
