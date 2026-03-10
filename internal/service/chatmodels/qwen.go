package chatmodels

import (
	"context"
	"sync"

	"github.com/cloudwego/eino-ext/components/model/qwen"
	"github.com/gogf/gf/v2/frame/g"
)

var (
	qwenModel     *qwen.ChatModel
	qwenModelOnce sync.Once
)

func getQwenModel(ctx context.Context) *qwen.ChatModel {
	qwenModelOnce.Do(func() {
		apiKey := g.Cfg().MustGet(ctx, "model.qwen.apiKey").String()
		modelName := g.Cfg().MustGet(ctx, "model.qwen.modelName", "qwen-plus").String()
		enableThinking := isThinkingEnabled(ctx)

		m, err := qwen.NewChatModel(ctx, buildQwenChatModelConfig(apiKey, modelName, enableThinking))
		if err != nil {
			g.Log().Fatalf(ctx, "init qwen model failed: %v", err)
		}
		g.Log().Infof(ctx, "init qwen model success, model=%s enable_thinking=%t", modelName, enableThinking)
		qwenModel = m
	})
	return qwenModel
}
