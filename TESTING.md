# Testing

## Baseline Checks

后端最小检查：

```bash
go test ./...
```

如果同时联调 research UI，前端至少跑一次：

```bash
cd ../UI
pnpm exec tsc -b --pretty false
```

## Local Services

research / DeerFlow 联调至少需要这些服务可用：

1. Redis
2. `../ChatGPT-Azure`
3. 当前仓库 `lakeside`，默认 `:8011`
4. `../UI`，默认 `:5173`
5. DeerFlow LangGraph API，按 `agents.deerflow.baseURL` 配置

UI 本地开发建议在 `../UI/.env` 里设置：

```bash
X_USER_ID=<你的 UPN>
```

这样 Vite proxy 会自动注入 `X-User-ID`，并通过 `X-Service: lakeside` 转发到 `:8011`。

## Research Manual Check

页面入口：

- `http://localhost:5173/chat/agents/research`

建议问题：

- `请对比 Kimi K2.5 和 GLM-5，重点比较代码能力、Agent 能力、价格、上下文长度，给我结论并引用来源。优先使用官方资料和 benchmark，如果资料冲突请明确说明。`

期望行为：

1. 发送问题后，research 会话应创建成功，并进入 `running`。
2. 进入对应会话页后，聊天区应切换到 DeerFlow 专用 renderer，而不是通用气泡列表。
3. 页面应显示 DeerFlow `title / run_status / thread_id / run_id`。
4. 页面应显示 DeerFlow processing 卡片，包括 thinking、tool call、搜索结果、todo、来源标签。
5. 来源卡片应显示可信度标签，如 `高可信 / 中可信 / 低可信`，以及来源类型标签，如 `官方 / 基准 / 提供方 / 社区`。
6. 如果存在低可信来源或工具失败，应显示 research quality warning。
7. research 专用视图下不应再叠一层通用消息泡泡；只保留必要操作按钮，例如 `取消当前执行`。

## Failure Check

如果 DeerFlow 终态没有返回可见文本，或 DeerFlow / LangGraph 中途失败，期望：

1. Lakeside run 状态为 `failed`，而不是“完成但空白”。
2. `deerflow.traceJson` 或 session detail 中能看到：
   - `thread_id`
   - `run_id`
   - `run_status`
   - `state_tail`
3. research 页面应把这些诊断信息显示在 quality warning / failed trace 里，便于回溯 DeerFlow 线程。

## API Spot Check

通过 Vite proxy 直接查看 render：

```bash
curl -s -N \
  -H 'X-Service: lakeside' \
  -H 'X-User-ID: <你的 UPN>' \
  'http://localhost:5173/v1/agent/research/render?session_id=<session_id>&advanced_trace=0'
```

关键检查点：

- `dataModel.deerflow.traceJson` 不应为 `null`
- `traceJson` 里应包含 `messages / sources / thread_id / run_id`

查看 session detail：

```bash
curl -s \
  -H 'X-Service: lakeside' \
  -H 'X-User-ID: <你的 UPN>' \
  'http://localhost:5173/v1/agent/research/sessions/<session_id>'
```

关键检查点：

- 最新 run 的 `events` 中应包含 `deerflow_trace_updated`
- render 恢复时，即使 snapshot 未固化 DeerFlow trace，也应能从最新事件回填并渲染出来
