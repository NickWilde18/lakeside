package assistant

import (
	"context"
	"strings"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/gogf/gf/v2/frame/g"
)

type callbackStartedAtKey struct{}

func newAgentCallbackHandler() callbacks.Handler {
	return callbacks.NewHandlerBuilder().
		OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, _ callbacks.CallbackInput) context.Context {
			if !shouldLogCallback(info) {
				return ctx
			}
			g.Log().Infof(ctx, "assistant agent callback start, name=%s type=%s", info.Name, info.Type)
			return context.WithValue(ctx, callbackStartedAtKey{}, time.Now())
		}).
		OnEndFn(func(ctx context.Context, info *callbacks.RunInfo, _ callbacks.CallbackOutput) context.Context {
			if !shouldLogCallback(info) {
				return ctx
			}
			startedAt, _ := ctx.Value(callbackStartedAtKey{}).(time.Time)
			if startedAt.IsZero() {
				g.Log().Infof(ctx, "assistant agent callback end, name=%s type=%s", info.Name, info.Type)
				return ctx
			}
			g.Log().Infof(ctx, "assistant agent callback end, name=%s type=%s duration_ms=%d", info.Name, info.Type, time.Since(startedAt).Milliseconds())
			return ctx
		}).
		OnErrorFn(func(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
			if !shouldLogCallback(info) {
				return ctx
			}
			startedAt, _ := ctx.Value(callbackStartedAtKey{}).(time.Time)
			if err != nil && isExpectedCallbackSignal(err) {
				if startedAt.IsZero() {
					g.Log().Infof(ctx, "assistant agent callback expected stop, name=%s type=%s", info.Name, info.Type)
					return ctx
				}
				g.Log().Infof(ctx, "assistant agent callback expected stop, name=%s type=%s duration_ms=%d", info.Name, info.Type, time.Since(startedAt).Milliseconds())
				return ctx
			}
			if startedAt.IsZero() {
				g.Log().Errorf(ctx, "assistant agent callback error, name=%s type=%s err=%v", info.Name, info.Type, err)
				return ctx
			}
			g.Log().Errorf(ctx, "assistant agent callback error, name=%s type=%s duration_ms=%d err=%v", info.Name, info.Type, time.Since(startedAt).Milliseconds(), err)
			return ctx
		}).
		Build()
}

func shouldLogCallback(info *callbacks.RunInfo) bool {
	if info == nil {
		return false
	}
	return info.Name == assistantAgentName || info.Name == "itsm_ticket_create_agent"
}

func isExpectedCallbackSignal(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "interrupt") || strings.Contains(text, "context canceled")
}
