package agentplatform

import (
	"bytes"
	"context"
	"encoding/gob"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/stretchr/testify/require"

	"lakeside/internal/service/agentplatform/eventctx"
	legacyknowledge "lakeside/internal/service/knowledgeagent"
)

func TestRunPathStringsSkipsInternalWorkflowNodes(t *testing.T) {
	path := []adk.RunStep{
		newRunStepForTest("campus"),
		newRunStepForTest("it"),
		newRunStepForTest("__it_workflow"),
		newRunStepForTest("campus_it_kb_for_itso_student_assistant"),
		newRunStepForTest("itsm"),
	}

	got := runPathStrings(path)
	require.Equal(t, []string{"campus", "it", "campus_it_kb_for_itso_student_assistant", "itsm"}, got)
}

func TestRunPathStringsNormalizesAncestorReturn(t *testing.T) {
	path := []adk.RunStep{
		newRunStepForTest("campus"),
		newRunStepForTest("it"),
		newRunStepForTest("campus_it_kb"),
		newRunStepForTest("it"),
	}

	got := runPathStrings(path)
	require.Equal(t, []string{"campus", "it"}, got)
}

func TestResolveNodePathPrefersRegistryPathOverWorkflowHistory(t *testing.T) {
	svc := &Service{
		registry: &runtimeRegistry{
			paths: nodePathIndex{
				"campus": {
					"campus": {"campus"},
					"it":     {"campus", "it"},
					"campus_it_kb_for_itso_student_assistant": {"campus", "it", "campus_it_kb_for_itso_student_assistant"},
					"itsm": {"campus", "it", "itsm"},
				},
			},
		},
	}

	currentPath := []string{"campus", "it", "__it_workflow", "campus_it_kb_for_itso_student_assistant", "itsm"}
	got := svc.resolveNodePath("campus", currentPath, "itsm")
	require.Equal(t, []string{"campus", "it", "itsm"}, got)
}

func TestResolveNodePathNormalizesReturnedChildPath(t *testing.T) {
	svc := &Service{
		registry: &runtimeRegistry{
			paths: nodePathIndex{
				"campus": {
					"campus":       {"campus"},
					"it":           {"campus", "it"},
					"campus_it_kb": {"campus", "it", "campus_it_kb"},
				},
			},
		},
	}

	currentPath := []string{"campus", "it", "campus_it_kb", "it"}
	got := svc.resolveNodePath("campus", currentPath, "campus_it_kb", "it")
	require.Equal(t, []string{"campus", "it", "campus_it_kb"}, got)
}

func TestConsumeIteratorEmitsKnowledgeAgentCompletedEvent(t *testing.T) {
	svc := &Service{
		registry: &runtimeRegistry{
			paths: nodePathIndex{
				"campus": {
					"campus": {"campus"},
					"it":     {"campus", "it"},
					"campus_it_kb_for_itso_student_assistant": {"campus", "it", "campus_it_kb_for_itso_student_assistant"},
				},
			},
		},
	}

	var events []struct {
		eventType string
		path      []string
		message   string
		payload   any
	}
	ctx := eventctx.WithRun(context.Background(), "run-1", "campus", "sess-1", nil, func(_ context.Context, eventType string, path []string, message string, payload any) {
		events = append(events, struct {
			eventType string
			path      []string
			message   string
			payload   any
		}{
			eventType: eventType,
			path:      append([]string(nil), path...),
			message:   message,
			payload:   payload,
		})
	})

	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		gen.Send(&adk.AgentEvent{
			Output: &adk.AgentOutput{
				CustomizedOutput: legacyknowledge.Result{
					AgentName: "campus_it_kb_for_itso_student_assistant",
					Success:   true,
					Message:   "knowledge answer",
				},
			},
		})
		gen.Close()
	}()

	resp := svc.consumeIterator(ctx, "campus", iter, "sess-1", "", "zh")
	require.NotNil(t, resp)
	require.Len(t, resp.Steps, 1)
	require.Len(t, events, 2)
	require.Equal(t, eventTypeKnowledgeAnswerReady, events[0].eventType)
	require.Equal(t, eventTypeAgentCompleted, events[1].eventType)
	require.Equal(t, []string{"campus", "it", "campus_it_kb_for_itso_student_assistant"}, events[1].path)

	payload, ok := events[1].payload.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "campus_it_kb_for_itso_student_assistant", payload["agent_name"])
	require.Equal(t, "Knowledge", payload["agent_type"])
}

func newRunStepForTest(name string) adk.RunStep {
	var step adk.RunStep
	_ = (&step).GobDecode(mustGobEncodeRunStep(name))
	return step
}

func mustGobEncodeRunStep(name string) []byte {
	encoded, err := gobEncode(struct {
		AgentName string
	}{AgentName: name})
	if err != nil {
		panic(err)
	}
	return encoded
}

func gobEncode(v any) ([]byte, error) {
	var buf bytes.Buffer
	err := gob.NewEncoder(&buf).Encode(v)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
