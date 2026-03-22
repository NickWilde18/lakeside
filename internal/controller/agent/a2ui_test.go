package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"lakeside/internal/service/agentplatform"
)

func TestBuildA2UIRenderMessages(t *testing.T) {
	messages := buildA2UIRenderMessages(agentPageState{
		AssistantKey:         "campus",
		AssistantTitle:       "校园助理",
		AssistantDescription: "desc",
		EmptyTitle:           "开始新对话",
		EmptyBody:            "发送第一条消息后自动创建会话。",
	})

	require.Len(t, messages, 4)
	require.NotNil(t, messages[0].DeleteSurface)
	require.NotNil(t, messages[1].SurfaceUpdate)
	require.NotNil(t, messages[2].DataModel)
	require.NotNil(t, messages[3].BeginRendering)
	require.Equal(t, a2uiSurfaceID, messages[0].DeleteSurface.SurfaceID)
	require.Equal(t, "chat-root", messages[3].BeginRendering.Root)

	components := messages[1].SurfaceUpdate.Components
	require.NotEmpty(t, components)
	ids := make([]string, 0, len(components))
	for _, component := range components {
		ids = append(ids, component.ID)
	}
	require.Contains(t, ids, "chat-root")
	require.Contains(t, ids, "messages-col")
	require.Contains(t, ids, "composer-card")
	require.NotContains(t, ids, "sidebar-col")
	require.NotContains(t, ids, "trace-col")
	require.NotNil(t, components[0].Component["Column"])

	contents := messages[2].DataModel.Contents
	require.Len(t, contents, 4)
	require.Equal(t, "meta", contents[0].Key)
	require.Equal(t, "composer", contents[1].Key)
	require.NotEmpty(t, contents[0].ValueMap)
	require.Equal(t, "assistantKey", contents[0].ValueMap[0].Key)
}

func TestParseActionHelpers(t *testing.T) {
	ctx := map[string]any{
		"sessionId": " sess-1 ",
		"approved":  true,
		"disabled":  "false",
		"missing":   nil,
	}

	require.Equal(t, "sess-1", parseActionString(ctx, "sessionId"))
	require.Equal(t, "", parseActionString(ctx, "unknown"))

	approved := parseActionBool(ctx, "approved")
	require.NotNil(t, approved)
	require.True(t, *approved)

	disabled := parseActionBool(ctx, "disabled")
	require.NotNil(t, disabled)
	require.False(t, *disabled)

	require.Nil(t, parseActionBool(ctx, "missing"))
}

func TestBuildPageMessagesAddsRunningPlaceholder(t *testing.T) {
	now := time.Date(2026, 3, 21, 15, 0, 0, 0, time.FixedZone("CST", 8*3600))
	detail := &agentplatform.SessionDetail{
		Messages: []agentplatform.SessionMessage{{
			ID:        1,
			Role:      "user",
			Content:   "VPN 连不上",
			CreatedAt: now,
		}},
		Runs: []agentplatform.RunTrace{{
			Snapshot: &agentplatform.RunSnapshot{
				RunID:        "run-1",
				AssistantKey: "campus",
				RunStatus:    "running",
				SessionID:    "sess-1",
			},
		}},
	}

	messages, latest := buildPageMessages(detail)
	require.NotNil(t, latest)
	require.Len(t, messages, 2)
	require.Equal(t, "user", messages[0].Role)
	require.Equal(t, "assistant", messages[1].Role)
	require.Equal(t, "running", messages[1].Status)
	require.Equal(t, "latest-running", messages[1].ID)
}
