package agentplatform

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDeleteSessionAllowsWaitingInputAndCancelsRun(t *testing.T) {
	repo := newTestAgentRepository(t)
	svc := &Service{
		repo:        repo,
		subscribers: make(map[string]map[chan RunEventRecord]struct{}),
		eventDedup:  make(map[string]map[string]agentEventDedupState),
	}
	now := time.Now().Add(-time.Minute)
	require.NoError(t, repo.SaveSession(context.Background(), SessionRecord{
		AssistantKey:     "campus",
		SessionID:        "sess-waiting",
		UserUPN:          "user@example.edu",
		ActivePathJSON:   `["campus","it"]`,
		ActiveCheckpoint: "ckpt-1",
		Status:           statusActive,
		Language:         "zh",
		CreatedAt:        now,
		UpdatedAt:        now,
	}))
	require.NoError(t, repo.CreateRun(context.Background(), RunRecord{
		RunID:        "run-waiting",
		AssistantKey: "campus",
		SessionID:    "sess-waiting",
		UserUPN:      "user@example.edu",
		Kind:         runKindQuery,
		Status:       runStatusWaitingInput,
		CheckpointID: "ckpt-1",
		RequestJSON:  `{"message":"请帮我报修"}`,
		ResponseJSON: `{}`,
		StartedAt:    now,
		FinishedAt:   now,
	}))

	err := svc.DeleteSession(context.Background(), &DeleteSessionRequest{
		AssistantKey: "campus",
		SessionID:    "sess-waiting",
		UserUPN:      "user@example.edu",
	})
	require.NoError(t, err)

	session, err := repo.GetSession(context.Background(), "sess-waiting")
	require.NoError(t, err)
	require.NotNil(t, session)
	require.Equal(t, statusDeleted, session.Status)

	run, err := repo.GetRun(context.Background(), "run-waiting")
	require.NoError(t, err)
	require.NotNil(t, run)
	require.Equal(t, runStatusCancelled, run.Status)
	require.Contains(t, run.ErrorMessage, "流程已放弃")

	events, err := repo.ListRunEventsAfter(context.Background(), "run-waiting", 0)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, eventTypeRunCancelled, events[0].EventType)
}

func TestPurgeDeletedSessionsRemovesOnlyExpiredDeletedHistory(t *testing.T) {
	repo := newTestAgentRepository(t)
	ctx := context.Background()
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	activeTime := time.Now().Add(-time.Hour)

	require.NoError(t, repo.SaveSession(ctx, SessionRecord{
		AssistantKey:   "campus",
		SessionID:      "sess-deleted-old",
		UserUPN:        "user@example.edu",
		ActivePathJSON: `["campus","it"]`,
		Status:         statusDeleted,
		Language:       "zh",
		CreatedAt:      oldTime,
		UpdatedAt:      oldTime,
	}))
	require.NoError(t, appendRunHistoryForTest(ctx, repo, "sess-deleted-old", "run-deleted-old", oldTime))

	require.NoError(t, repo.SaveSession(ctx, SessionRecord{
		AssistantKey:   "campus",
		SessionID:      "sess-active",
		UserUPN:        "user@example.edu",
		ActivePathJSON: `["campus","it"]`,
		Status:         statusActive,
		Language:       "zh",
		CreatedAt:      activeTime,
		UpdatedAt:      activeTime,
	}))
	require.NoError(t, appendRunHistoryForTest(ctx, repo, "sess-active", "run-active", activeTime))

	purged, err := repo.PurgeDeletedSessions(ctx, time.Now().Add(-7*24*time.Hour), 100)
	require.NoError(t, err)
	require.Equal(t, 1, purged)

	deletedSession, err := repo.GetSession(ctx, "sess-deleted-old")
	require.NoError(t, err)
	require.Nil(t, deletedSession)
	deletedMessages, err := repo.ListMessages(ctx, "sess-deleted-old")
	require.NoError(t, err)
	require.Empty(t, deletedMessages)
	deletedRun, err := repo.GetRun(ctx, "run-deleted-old")
	require.NoError(t, err)
	require.Nil(t, deletedRun)
	deletedEvents, err := repo.ListRunEventsAfter(ctx, "run-deleted-old", 0)
	require.NoError(t, err)
	require.Empty(t, deletedEvents)

	activeSession, err := repo.GetSession(ctx, "sess-active")
	require.NoError(t, err)
	require.NotNil(t, activeSession)
	activeMessages, err := repo.ListMessages(ctx, "sess-active")
	require.NoError(t, err)
	require.Len(t, activeMessages, 1)
	activeRun, err := repo.GetRun(ctx, "run-active")
	require.NoError(t, err)
	require.NotNil(t, activeRun)
	activeEvents, err := repo.ListRunEventsAfter(ctx, "run-active", 0)
	require.NoError(t, err)
	require.Len(t, activeEvents, 1)
}

func newTestAgentRepository(t *testing.T) *sqliteRepository {
	t.Helper()
	cfg := &config{}
	cfg.Storage.Provider = "sqlite"
	cfg.Storage.SQLitePath = filepath.Join(t.TempDir(), "agent.db")
	repo, err := newRepository(context.Background(), cfg)
	require.NoError(t, err)
	typed, ok := repo.(*sqliteRepository)
	require.True(t, ok)
	return typed
}

func appendRunHistoryForTest(ctx context.Context, repo *sqliteRepository, sessionID, runID string, ts time.Time) error {
	if _, err := repo.AppendMessage(ctx, MessageRecord{
		AssistantKey:   "campus",
		SessionID:      sessionID,
		UserUPN:        "user@example.edu",
		Role:           "assistant",
		Content:        "test message",
		ActivePathJSON: `["campus","it"]`,
		Language:       "zh",
		CreatedAt:      ts,
	}); err != nil {
		return err
	}
	if err := repo.CreateRun(ctx, RunRecord{
		RunID:        runID,
		AssistantKey: "campus",
		SessionID:    sessionID,
		UserUPN:      "user@example.edu",
		Kind:         runKindQuery,
		Status:       runStatusDone,
		RequestJSON:  `{"message":"hello"}`,
		ResponseJSON: `{"status":"done"}`,
		StartedAt:    ts,
		FinishedAt:   ts,
	}); err != nil {
		return err
	}
	_, err := repo.AppendRunEvent(ctx, RunEventRecord{
		RunID:        runID,
		AssistantKey: "campus",
		SessionID:    sessionID,
		EventType:    eventTypeRunCompleted,
		PathJSON:     `["campus","it"]`,
		Message:      "completed",
		PayloadJSON:  `{"run_status":"done"}`,
		CreatedAt:    ts,
	})
	return err
}
