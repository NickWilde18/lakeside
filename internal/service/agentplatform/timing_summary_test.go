package agentplatform

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSummarizeRunTiming(t *testing.T) {
	startedAt := time.Date(2026, 3, 12, 2, 1, 0, 0, time.Local)
	finishedAt := startedAt.Add(55 * time.Second)
	events := []RunEventRecord{
		{
			EventType:   eventTypeKnowledgeRetrieveStart,
			PathJSON:    `["campus","it","campus_it_kb_for_itso_student_assistant"]`,
			PayloadJSON: `{"query":"wifi"}`,
			CreatedAt:   startedAt.Add(2 * time.Second),
		},
		{
			EventType:   eventTypeKnowledgeRetrieveEnd,
			PathJSON:    `["campus","it","campus_it_kb_for_itso_student_assistant"]`,
			PayloadJSON: `{"duration_ms":700}`,
			CreatedAt:   startedAt.Add(3 * time.Second),
		},
		{
			EventType:   eventTypeKnowledgeAnswerReady,
			PathJSON:    `["campus","it","campus_it_kb_for_itso_student_assistant"]`,
			PayloadJSON: `{}`,
			CreatedAt:   startedAt.Add(15 * time.Second),
		},
		{
			EventType:   eventTypeAgentCompleted,
			PathJSON:    `["campus","it","itsm"]`,
			PayloadJSON: `{"agent_name":"itsm","agent_type":"Agent","duration_ms":52000}`,
			CreatedAt:   startedAt.Add(55 * time.Second),
		},
	}

	summary := summarizeRunTiming(events, startedAt, finishedAt)
	require.Contains(t, summary, "knowledge[campus / it / campus_it_kb_for_itso_student_assistant]{queries=1,retrieve_total_ms=700,retrieve_max_ms=700,total_ms=13000}")
	require.Contains(t, summary, "agent[campus / it / itsm|itsm|Agent]=52000ms")
}
