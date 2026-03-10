package assistant

import (
	"testing"

	itsmv1 "lakeside/api/itsm/v1"

	"github.com/stretchr/testify/require"
)

func TestSummarizeTargets(t *testing.T) {
	confirmed := true
	text := summarizeTargets(map[string]*itsmv1.ResumeTarget{
		"a": {
			Confirmed:  &confirmed,
			Subject:    "Dorm WiFi issue",
			OthersDesc: "Connected but no internet",
		},
	}, "en")
	require.NotEmpty(t, text)
}

func TestResponseActiveAgent(t *testing.T) {
	require.Equal(t, activeAgentAssistant, responseActiveAgent(false, activeAgentAssistant, nil))
	require.Equal(t, activeAgentITSM, responseActiveAgent(true, activeAgentAssistant, nil))
}

func TestDetectLanguage(t *testing.T) {
	require.Equal(t, "zh", detectLanguage("宿舍网络坏了"))
	require.Equal(t, "en", detectLanguage("Dorm WiFi is broken"))
}
