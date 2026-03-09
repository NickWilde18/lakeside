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

		m, err := qwen.NewChatModel(ctx, &qwen.ChatModelConfig{
			BaseURL:     "https://dashscope.aliyuncs.com/compatible-mode/v1",
			APIKey:      apiKey,
			Model:       modelName,
			Timeout:     0,
			MaxTokens:   of(2048),
			Temperature: of(float32(0.2)),
			TopP:        of(float32(0.8)),
		})
		if err != nil {
			g.Log().Fatalf(ctx, "init qwen model failed: %v", err)
		}
		qwenModel = m
	})
	return qwenModel
}
