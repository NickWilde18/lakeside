package eventctx

import "context"

type EmitFunc func(ctx context.Context, eventType string, path []string, message string, payload any)

type ResolvePathFunc func(nodeName string) []string

type meta struct {
	RunID        string
	AssistantKey string
	SessionID    string
}

type (
	metaKey       struct{}
	emitFuncKey   struct{}
	resolvePathKey struct{}
)

func WithRun(ctx context.Context, runID, assistantKey, sessionID string, resolve ResolvePathFunc, emit EmitFunc) context.Context {
	ctx = context.WithValue(ctx, metaKey{}, meta{
		RunID:        runID,
		AssistantKey: assistantKey,
		SessionID:    sessionID,
	})
	if resolve != nil {
		ctx = context.WithValue(ctx, resolvePathKey{}, resolve)
	}
	if emit != nil {
		ctx = context.WithValue(ctx, emitFuncKey{}, emit)
	}
	return ctx
}

func RunID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	item, _ := ctx.Value(metaKey{}).(meta)
	return item.RunID
}

func AssistantKey(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	item, _ := ctx.Value(metaKey{}).(meta)
	return item.AssistantKey
}

func SessionID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	item, _ := ctx.Value(metaKey{}).(meta)
	return item.SessionID
}

func ResolvePath(ctx context.Context, nodeName string) []string {
	if ctx == nil {
		return nil
	}
	fn, _ := ctx.Value(resolvePathKey{}).(ResolvePathFunc)
	if fn == nil {
		return nil
	}
	return fn(nodeName)
}

func Emit(ctx context.Context, eventType string, path []string, message string, payload any) {
	if ctx == nil {
		return
	}
	fn, _ := ctx.Value(emitFuncKey{}).(EmitFunc)
	if fn == nil {
		return
	}
	fn(ctx, eventType, path, message, payload)
}

func EmitForNode(ctx context.Context, eventType, nodeName, message string, payload any) {
	Emit(ctx, eventType, ResolvePath(ctx, nodeName), message, payload)
}
