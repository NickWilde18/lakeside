package agentplatform

import (
	"context"
	"strings"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/gogf/gf/v2/frame/g"

	"lakeside/internal/service/agentplatform/eventctx"
)

type callbackStartedAtKey struct{}

func newAgentCallbackHandler(names ...string) callbacks.Handler {
	allowedNames := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		allowedNames[name] = struct{}{}
	}
	return callbacks.NewHandlerBuilder().
		OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, _ callbacks.CallbackInput) context.Context {
			if !shouldEmitCallback(info, allowedNames) {
				return ctx
			}
			g.Log().Infof(ctx, "agentplatform callback start, name=%s type=%s", info.Name, info.Type)
			eventctx.EmitForNode(ctx, eventTypeAgentEntered, info.Name, info.Name, map[string]any{
				"agent_name": info.Name,
				"agent_type": info.Type,
			})
			return context.WithValue(ctx, callbackStartedAtKey{}, time.Now())
		}).
		OnEndFn(func(ctx context.Context, info *callbacks.RunInfo, _ callbacks.CallbackOutput) context.Context {
			if !shouldEmitCallback(info, allowedNames) {
				return ctx
			}
			startedAt, _ := ctx.Value(callbackStartedAtKey{}).(time.Time)
			if startedAt.IsZero() {
				// 过滤缺少起始时间的结束回调，避免重复的 0ms 事件污染前端时间线。
				return ctx
			}
			g.Log().Infof(ctx, "agentplatform callback end, name=%s type=%s duration_ms=%d", info.Name, info.Type, time.Since(startedAt).Milliseconds())
			eventctx.EmitForNode(ctx, eventTypeAgentCompleted, info.Name, info.Name, map[string]any{
				"agent_name":  info.Name,
				"agent_type":  info.Type,
				"duration_ms": time.Since(startedAt).Milliseconds(),
			})
			return ctx
		}).
		OnErrorFn(func(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
			if !shouldEmitCallback(info, allowedNames) {
				return ctx
			}
			startedAt, _ := ctx.Value(callbackStartedAtKey{}).(time.Time)
			if err != nil && isExpectedCallbackSignal(err) {
				if startedAt.IsZero() {
					g.Log().Infof(ctx, "agentplatform callback expected stop, name=%s type=%s", info.Name, info.Type)
					return ctx
				}
				g.Log().Infof(ctx, "agentplatform callback expected stop, name=%s type=%s duration_ms=%d", info.Name, info.Type, time.Since(startedAt).Milliseconds())
				return ctx
			}
			if startedAt.IsZero() {
				g.Log().Errorf(ctx, "agentplatform callback error, name=%s type=%s err=%v", info.Name, info.Type, err)
				return ctx
			}
			g.Log().Errorf(ctx, "agentplatform callback error, name=%s type=%s duration_ms=%d err=%v", info.Name, info.Type, time.Since(startedAt).Milliseconds(), err)
			return ctx
		}).
		Build()
}

func shouldEmitCallback(info *callbacks.RunInfo, allowedNames map[string]struct{}) bool {
	if info == nil || len(allowedNames) == 0 {
		return false
	}
	name := strings.TrimSpace(info.Name)
	if name == "" {
		return false
	}
	if _, ok := allowedNames[name]; !ok {
		return false
	}
	switch strings.TrimSpace(info.Type) {
	case "Supervisor", "Agent":
		return true
	default:
		return false
	}
}

func isExpectedCallbackSignal(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "interrupt") || strings.Contains(text, "context canceled")
}
