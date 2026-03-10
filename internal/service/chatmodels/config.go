package chatmodels

import (
	"context"

	openai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino-ext/components/model/qwen"
	"github.com/gogf/gf/v2/frame/g"
)

const (
	qwenBaseURL       = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	openRouterBaseURL = "https://openrouter.ai/api/v1"
)

func isThinkingEnabled(ctx context.Context) bool {
	return g.Cfg().MustGet(ctx, "model.enableThinking", true).Bool()
}

func buildQwenChatModelConfig(apiKey, modelName string, enableThinking bool) *qwen.ChatModelConfig {
	// Qwen 原生支持 enable_thinking，直接透传即可。
	return &qwen.ChatModelConfig{
		BaseURL:        qwenBaseURL,
		APIKey:         apiKey,
		Model:          modelName,
		Timeout:        0,
		MaxTokens:      of(2048),
		Temperature:    of(float32(0.2)),
		TopP:           of(float32(0.8)),
		EnableThinking: of(enableThinking),
	}
}

func buildOpenRouterChatModelConfig(apiKey, modelName string, enableThinking bool) *openai.ChatModelConfig {
	cfg := &openai.ChatModelConfig{
		BaseURL:     openRouterBaseURL,
		APIKey:      apiKey,
		Model:       modelName,
		Timeout:     0,
		MaxTokens:   of(2048),
		Temperature: of(float32(0.2)),
		TopP:        of(float32(0.8)),
	}
	if !enableThinking {
		// OpenRouter 走 OpenAI 兼容接口，这里用 reasoning.effort=none 关闭推理。
		cfg.ExtraFields = g.Map{
			"reasoning": g.Map{
				"effort": "none",
			},
		}
	}
	return cfg
}
