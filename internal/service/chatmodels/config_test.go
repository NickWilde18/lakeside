package chatmodels

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildQwenChatModelConfig_DisableThinking(t *testing.T) {
	cfg := buildQwenChatModelConfig("test-key", "qwen-plus", false)

	require.NotNil(t, cfg.EnableThinking)
	require.False(t, *cfg.EnableThinking)
	require.Equal(t, qwenBaseURL, cfg.BaseURL)
	require.Equal(t, "qwen-plus", cfg.Model)
}

func TestBuildOpenRouterChatModelConfig_DisableThinking(t *testing.T) {
	cfg := buildOpenRouterChatModelConfig("test-key", "moonshotai/kimi-k2.5", false)

	require.Equal(t, openRouterBaseURL, cfg.BaseURL)
	require.Equal(t, "moonshotai/kimi-k2.5", cfg.Model)
	require.Equal(t, "none", cfg.ExtraFields["reasoning"].(map[string]any)["effort"])
}

func TestBuildOpenRouterChatModelConfig_EnableThinking(t *testing.T) {
	cfg := buildOpenRouterChatModelConfig("test-key", "moonshotai/kimi-k2.5", true)

	require.Nil(t, cfg.ExtraFields)
}
