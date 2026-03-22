package rootassistant

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/stretchr/testify/require"

	"lakeside/internal/service/moduleapi"
)

type stubModule struct {
	name       string
	desc       string
	assessment moduleapi.Assessment
}

func (m *stubModule) Name(_ context.Context) string { return m.name }

func (m *stubModule) Description(_ context.Context) string { return m.desc }

func (m *stubModule) Run(_ context.Context, _ *adk.AgentInput, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go gen.Close()
	return iter
}

func (m *stubModule) Resume(_ context.Context, _ *adk.ResumeInfo, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go gen.Close()
	return iter
}

func (m *stubModule) Assess(_ context.Context, _ string) (moduleapi.Assessment, error) {
	return m.assessment, nil
}

func TestCampusChooseRouteSplitsReadAndWriteModules(t *testing.T) {
	agent := &campusAgent{}
	plan, clarify := agent.chooseRoute([]moduleapi.Assessment{
		{ModuleKey: "it", Status: moduleapi.AssessmentReady, Phase: moduleapi.PhaseRead, Score: 0.9},
		{ModuleKey: "osa", Status: moduleapi.AssessmentReady, Phase: moduleapi.PhaseWrite, Score: 0.8},
	})
	require.Nil(t, clarify)
	require.Equal(t, []string{"it"}, plan.ReadModules)
	require.Equal(t, []string{"osa"}, plan.WriteModules)
}

func TestCampusChooseRouteFallsBackToClarifyWhenNoReadyModule(t *testing.T) {
	agent := &campusAgent{}
	plan, clarify := agent.chooseRoute([]moduleapi.Assessment{
		{ModuleKey: "it", Status: moduleapi.AssessmentNeedClarify, Phase: moduleapi.PhaseRead, Score: 0.6, FollowUpPrompt: "请补充系统名称。"},
		{ModuleKey: "osa", Status: moduleapi.AssessmentReject, Phase: moduleapi.PhaseRead, Score: 0.1},
	})
	require.True(t, plan.isEmpty())
	require.NotNil(t, clarify)
	require.Equal(t, "it", clarify.ModuleKey)
	require.Equal(t, "请补充系统名称。", clarify.FollowUpPrompt)
}
