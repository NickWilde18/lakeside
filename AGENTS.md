# Repository Guidelines

## Project Structure & Module Organization
- `main.go`: application entry.
- `config/`: runtime config (`config.yaml`).
- `api/`: GoFrame API contracts (Req/Res + route metadata). Versioned under `api/*/v1`.
- `internal/controller/`: HTTP handlers. Follow project rule: **one handler per file**.
- `internal/service/agentplatform/`: 通用 agent 平台层（registry/runtime/session/response/memory）。
- `internal/service/rootassistant/`: 顶层助手，例如 `campus`、后续的 `coding`。
- `internal/service/domainassistant/`: 领域助手，例如 `it`、后续的 `hr`、`osa`。
- `internal/service/leafagent/`: 叶子 agent，例如 `itsm`、`knowledge`。
- `internal/service/`: 其他业务或基础服务，例如 `itsmagent`, `itsmclient`, `chatmodels`。
- `internal/cmd/`: server bootstrap and route binding.
- `manifest/`, `hack/`, `resource/`: deployment scripts, codegen/build helpers, static assets.

## Build, Test, and Development Commands
- `make build`: build binary via GoFrame CLI (`gf build -ew`).
- `make ctrl`: generate controller/interface files from `api/` (`gf gen ctrl`).
- `go test ./...`: run all unit tests.
- `go run main.go`: start server locally.
- `make image` / `make image.push`: build and optionally push Docker image.

## Coding Style & Naming Conventions
- Language: Go `1.26.0`, format with `gofmt` (or `go fmt ./...`).
- Keep packages small and focused; prefer clear service boundaries.
- API definitions must use paired names: `XxxReq` and `XxxRes`.
- In `api/*/v1`, keep each `Req`/`Res` as a paired declaration. If a `Res` depends on custom structs, declare those structs immediately before that `Res`.
- `XxxRes` must be a concrete struct type (not a type alias) to satisfy GoFrame runtime response naming checks.
- When config semantics are documented in code, put them on the config struct fields or config type definition, not as ad-hoc comment blocks at individual read sites.
- For exported Go symbols, prefer standard Go doc style comments over ad-hoc parameter bullet lists.
- Do not hand-write controller skeletons. Define API first, then run `gf gen ctrl`.
- Controller methods should match generated interface names (for example `AgentQuery`, `AgentResume`).
- During collaboration, if new global requirements or coding conventions are agreed in chat, update `AGENTS.md` immediately so rules stay source-of-truth.
- 顶层 IT 助手下的各类子代理（例如 `itsm`、knowledge subagents）是同级关系；不要把知识检索能力嵌套进 ITSM 子代理内部。
- 对外请求头 `X-User-ID` 保持不变，但其值语义统一为 UPN；如某个下游系统要求员工编号等其他身份字段，应在服务端内部转换，不要要求前端改传别的 header。
- 顶层/领域助手的路由提示词模板应通过 `agents.roots[].instructionTemplate`、`agents.domains[].instructionTemplate` 配置，不要把这类路由策略硬编码在 Go 代码里；字段说明写在 `config/config.example.yaml`。
- 顶层/领域层编排优先使用 Eino 官方组件，当前统一使用 `adk/prebuilt/supervisor`；只有官方没有现成能力时才加薄适配层。
- knowledge 叶子 agent 不得靠硬编码“一轮只检索一次”来限制检索；如需扩召回，优先使用 Eino 官方检索组件，例如 `flow/retriever/multiquery`。
- 未来 agent 体系按分层组织：`顶层助手 -> 领域助手 -> 叶子 agent`。目录设计应优先体现这三层结构，不再继续把所有 agent 平铺在 `internal/service` 根下。
- 主产品入口统一使用 `/v1/agent/{assistant_key}/*`；旧 `/v1/assistant/*` 不再保留。

## Testing Guidelines
- Use Go `testing` package; `testify/require` is allowed and already used in this repo for concise assertions.
- Test files end with `_test.go`; functions use `TestXxx` naming.
- Place tests next to implementation (for example `internal/service/itsmclient/client_test.go`).
- Minimum check before pushing: `go test ./...`.

## Commit & Pull Request Guidelines
- Keep commits small, focused, and buildable.
- Commit message style in this repo is short and direct (single-line summary).
  - Example: `itsm: add ADK resume flow`
- PRs should include:
  - what changed and why,
  - key API/config impacts,
  - test evidence (`go test ./...` output),
  - sample request/response for API changes.

## Security & Configuration Tips
- Never commit real secrets (`model.*.apiKey`, `itsm.appSecret`, Redis password).
- Use environment-specific config overrides for production.
- For multi-instance ADK resume, configure shared Redis checkpoint storage.
- Keep `config/config.yaml` lean for real runtime values, and put human-facing field explanations/examples in `config/config.example.yaml`.
- Prefer code constants over runtime config for fixed Redis key prefixes or other internal namespace conventions unless they truly need environment-level override.
- Redis is a required infrastructure dependency for checkpoint/idempotency persistence in this repo; do not add in-memory fallback paths for those stores.
