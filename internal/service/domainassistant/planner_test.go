package domainassistant

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

type fakeToolCallingModel struct {
	content string
	err     error
}

func (f *fakeToolCallingModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &schema.Message{Content: f.content}, nil
}

func (f *fakeToolCallingModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}

func (f *fakeToolCallingModel) WithTools(_ []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return f, nil
}

func TestPlannerPlanKnowledgeBeforeInterruptingLeaf(t *testing.T) {
	p := &planner{
		domainKey:   "it",
		instruction: "测试领域规则",
		leaves: []LeafBinding{
			{Key: "itsm", Kind: "itsm", Description: "正式工单", Interruptible: true},
			{Key: "campus_it_kb", Kind: "knowledge", Description: "知识问答", Interruptible: false},
		},
		model: &fakeToolCallingModel{
			content: `{"mode":"sequential","steps":[{"agent_key":"campus_it_kb","reason":"先给排查建议"},{"agent_key":"itsm","reason":"再建单"}]}`,
		},
	}

	plan, err := p.Plan(context.Background(), "VPN 连不上怎么办？另外帮我报修。")
	require.NoError(t, err)
	require.Equal(t, planModeSequential, plan.Mode)
	require.Len(t, plan.Steps, 2)
	require.Equal(t, "campus_it_kb", plan.Steps[0].AgentKey)
	require.Equal(t, "itsm", plan.Steps[1].AgentKey)
}

func TestPlannerNormalizeDropsUnknownAndDuplicateSteps(t *testing.T) {
	p := &planner{
		leaves: []LeafBinding{
			{Key: "itsm", Kind: "itsm", Description: "正式工单", Interruptible: true},
			{Key: "campus_it_kb", Kind: "knowledge", Description: "知识问答", Interruptible: false},
		},
	}

	plan := p.normalizePlan(domainExecutionPlan{
		Mode: planModeSequential,
		Steps: []domainPlanStep{
			{AgentKey: "campus_it_kb"},
			{AgentKey: "unknown"},
			{AgentKey: "campus_it_kb"},
			{AgentKey: "itsm"},
		},
	})

	require.Equal(t, planModeSequential, plan.Mode)
	require.Len(t, plan.Steps, 2)
	require.Equal(t, "campus_it_kb", plan.Steps[0].AgentKey)
	require.Equal(t, "itsm", plan.Steps[1].AgentKey)
}

func TestPlannerNormalizeFallsBackToSupervisorWhenNoValidSteps(t *testing.T) {
	p := &planner{
		leaves: []LeafBinding{
			{Key: "itsm", Kind: "itsm", Description: "正式工单", Interruptible: true},
		},
	}

	plan := p.normalizePlan(domainExecutionPlan{
		Mode:  planModeSequential,
		Steps: []domainPlanStep{{AgentKey: "missing"}},
	})

	require.Equal(t, planModeSupervisor, plan.Mode)
	require.Empty(t, plan.Steps)
}

func TestPlannerHeuristicHowToReportUsesKnowledgeOnly(t *testing.T) {
	p := &planner{
		leaves: []LeafBinding{
			{Key: "itsm", Kind: "itsm", Description: "正式工单", Interruptible: true},
			{Key: "campus_it_kb", Kind: "knowledge", Description: "知识问答", Interruptible: false},
		},
		model: &fakeToolCallingModel{
			content: `{"mode":"supervisor","steps":[]}`,
		},
	}

	plan, err := p.Plan(context.Background(), "VPN 连不上，先帮我排查，再告诉我如果还不行该怎么报修。")
	require.NoError(t, err)
	require.Equal(t, planModeSequential, plan.Mode)
	require.Len(t, plan.Steps, 1)
	require.Equal(t, "campus_it_kb", plan.Steps[0].AgentKey)
}

func TestPlannerHeuristicSubmitAndTroubleshootUsesKnowledgeThenITSM(t *testing.T) {
	p := &planner{
		leaves: []LeafBinding{
			{Key: "itsm", Kind: "itsm", Description: "正式工单", Interruptible: true},
			{Key: "campus_it_kb", Kind: "knowledge", Description: "知识问答", Interruptible: false},
		},
		model: &fakeToolCallingModel{
			content: `{"mode":"supervisor","steps":[]}`,
		},
	}

	plan, err := p.Plan(context.Background(), "VPN 连不上，先帮我排查，然后直接帮我提交工单。")
	require.NoError(t, err)
	require.Equal(t, planModeSequential, plan.Mode)
	require.Len(t, plan.Steps, 2)
	require.Equal(t, "campus_it_kb", plan.Steps[0].AgentKey)
	require.Equal(t, "itsm", plan.Steps[1].AgentKey)
}

func TestPlannerHeuristicSubmitAfterTriedUsesITSMOnly(t *testing.T) {
	p := &planner{
		leaves: []LeafBinding{
			{Key: "itsm", Kind: "itsm", Description: "正式工单", Interruptible: true},
			{Key: "campus_it_kb", Kind: "knowledge", Description: "知识问答", Interruptible: false},
		},
		model: &fakeToolCallingModel{
			content: `{"mode":"supervisor","steps":[]}`,
		},
	}

	plan, err := p.Plan(context.Background(), "我尝试之后还是不行，提个工单吧。")
	require.NoError(t, err)
	require.Equal(t, planModeSequential, plan.Mode)
	require.Len(t, plan.Steps, 1)
	require.Equal(t, "itsm", plan.Steps[0].AgentKey)
}

func TestPlannerHeuristicExplicitSubmitUsesITSMOnlyWithoutGuidanceIntent(t *testing.T) {
	p := &planner{
		leaves: []LeafBinding{
			{Key: "itsm", Kind: "itsm", Description: "正式工单", Interruptible: true},
			{Key: "campus_it_kb", Kind: "knowledge", Description: "知识问答", Interruptible: false},
		},
		model: &fakeToolCallingModel{
			content: `{"mode":"supervisor","steps":[]}`,
		},
	}

	plan, err := p.Plan(context.Background(), "VPN 连不上，直接帮我提个工单。")
	require.NoError(t, err)
	require.Equal(t, planModeSequential, plan.Mode)
	require.Len(t, plan.Steps, 1)
	require.Equal(t, "itsm", plan.Steps[0].AgentKey)
}
