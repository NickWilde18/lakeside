package chatmodels

import (
	"context"
	"sync"

	openai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/gogf/gf/v2/frame/g"
)

var (
	openRouterModel     *openai.ChatModel
	openRouterModelOnce sync.Once
)

func getOpenRouterModel(ctx context.Context) *openai.ChatModel {
	openRouterModelOnce.Do(func() {
		apiKey := g.Cfg().MustGet(ctx, "model.openrouter.apiKey").String()
		modelName := g.Cfg().MustGet(ctx, "model.openrouter.modelName", "moonshotai/kimi-k2").String()
		enableThinking := isThinkingEnabled(ctx)

		m, err := openai.NewChatModel(ctx, buildOpenRouterChatModelConfig(apiKey, modelName, enableThinking))
		if err != nil {
			g.Log().Fatalf(ctx, "init openrouter model failed: %v", err)
		}
		g.Log().Infof(ctx, "init openrouter model success, model=%s enable_thinking=%t", modelName, enableThinking)
		openRouterModel = m
	})
	return openRouterModel
}
