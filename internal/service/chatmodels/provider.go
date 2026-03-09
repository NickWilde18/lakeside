package chatmodels

import (
	"context"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/gogf/gf/v2/frame/g"
)

func GetChatModel(ctx context.Context) model.ToolCallingChatModel {
	provider := strings.ToLower(g.Cfg().MustGet(ctx, "model.provider", "openrouter").String())
	switch provider {
	case "qwen":
		return getQwenModel(ctx)
	case "openrouter", "open_router":
		return getOpenRouterModel(ctx)
	default:
		return getOpenRouterModel(ctx)
	}
}
