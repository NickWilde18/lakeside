package itsmagent

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/stretchr/testify/require"
)

func TestTicketCreateAgentResumeCollectCanCancel(t *testing.T) {
	t.Parallel()

	agent := NewTicketCreateAgent(nil, nil, nil, nil, nil, serviceConfig{})
	iter := agent.Resume(context.Background(), &adk.ResumeInfo{
		WasInterrupted: true,
		InterruptState: &TicketAgentState{
			Stage:    stageCollect,
			Language: "zh",
			Pending: TicketInterruptInfo{
				Type:   statusNeedInfo,
				Prompt: "请补充更多信息。",
			},
		},
		IsResumeTarget: true,
		ResumeData: &ResumeConfirmData{
			Confirmed: false,
		},
	})

	event, ok := iter.Next()
	require.True(t, ok)
	require.NotNil(t, event)
	require.NotNil(t, event.Output)

	msg, _, err := adk.GetMessage(event)
	require.NoError(t, err)
	require.NotNil(t, msg)
	require.Equal(t, "已取消创建工单。", msg.Content)

	result := ExecutionResultFromAny(event.Output.CustomizedOutput)
	require.NotNil(t, result)
	require.False(t, result.Success)
	require.Equal(t, "用户取消提交工单", result.Message)
}
