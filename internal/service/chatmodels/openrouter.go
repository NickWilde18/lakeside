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

		m, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
			BaseURL:     "https://openrouter.ai/api/v1",
			APIKey:      apiKey,
			Model:       modelName,
			Timeout:     0,
			MaxTokens:   of(2048),
			Temperature: of(float32(0.2)),
			TopP:        of(float32(0.8)),
		})
		if err != nil {
			g.Log().Fatalf(ctx, "init openrouter model failed: %v", err)
		}
		openRouterModel = m
	})
	return openRouterModel
}
