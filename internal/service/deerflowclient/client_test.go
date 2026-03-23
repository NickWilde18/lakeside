package deerflowclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunWaitSetsRecursionLimitAndReturnsVisibleText(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/threads/thread-1/runs/stream":
			require.Equal(t, http.MethodPost, r.Method)
			var payload map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			config, ok := payload["config"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, float64(defaultDeerFlowRunRecursionLimit), config["recursion_limit"])
			input, ok := payload["input"].(map[string]any)
			require.True(t, ok)
			requestMessages, ok := input["messages"].([]any)
			require.True(t, ok)
			require.Len(t, requestMessages, 2)
			first, ok := requestMessages[0].(map[string]any)
			require.True(t, ok)
			second, ok := requestMessages[1].(map[string]any)
			require.True(t, ok)
			require.Equal(t, "system", first["role"])
			require.Equal(t, "human", second["role"])
			require.Equal(t, "hi", second["content"])
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "event: metadata\n")
			fmt.Fprint(w, "data: {\"run_id\":\"run-1\",\"attempt\":1}\n\n")
			stateJSON, err := json.Marshal(map[string]any{
				"messages": []map[string]any{
					{"type": "human", "content": "hi"},
					{"type": "ai", "content": "visible answer"},
				},
			})
			require.NoError(t, err)
			fmt.Fprint(w, "event: values\n")
			fmt.Fprintf(w, "data: %s\n\n", stateJSON)
		case "/api/threads/thread-1/runs":
			require.Equal(t, http.MethodGet, r.Method)
			require.NoError(t, json.NewEncoder(w).Encode([]map[string]any{
				{"run_id": "run-1", "status": "success"},
			}))
		case "/api/threads/thread-1/state":
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"values": map[string]any{
					"messages": []map[string]any{
						{"type": "human", "content": "hi"},
						{"type": "ai", "content": "visible answer"},
					},
				},
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(Config{
		BaseURL:     server.URL + "/api",
		AssistantID: "lead_agent",
		Timeout:     5 * time.Second,
	})

	result, err := client.RunWait(context.Background(), RunWaitRequest{
		ThreadID: "thread-1",
		Message:  "hi",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "thread-1", result.ThreadID)
	require.Equal(t, "run-1", result.RunID)
	require.Equal(t, "success", result.RunStatus)
	require.Equal(t, "visible answer", result.Message)
}

func TestRunWaitReturnsDiagnosticErrorWhenRunStatusIsError(t *testing.T) {
	t.Parallel()

	waitState := map[string]any{
		"messages": []map[string]any{
			{"type": "human", "content": "hi"},
			{
				"type":              "ai",
				"content":           "",
				"tool_calls":        []map[string]any{{"name": "web_search", "id": "web_search:1"}},
				"response_metadata": map[string]any{"finish_reason": "tool_calls"},
			},
			{"type": "tool", "name": "web_search", "status": "success", "content": "search output"},
		},
	}
	stateDetail := map[string]any{
		"metadata": map[string]any{"step": 25},
		"values":   waitState,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/threads/thread-err/runs/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "event: metadata\n")
			fmt.Fprint(w, "data: {\"run_id\":\"run-err\",\"attempt\":1}\n\n")
			stateJSON, err := json.Marshal(waitState)
			require.NoError(t, err)
			fmt.Fprint(w, "event: values\n")
			fmt.Fprintf(w, "data: %s\n\n", stateJSON)
		case "/api/threads/thread-err/runs":
			require.NoError(t, json.NewEncoder(w).Encode([]map[string]any{
				{"run_id": "run-err", "status": "error"},
			}))
		case "/api/threads/thread-err/state":
			require.NoError(t, json.NewEncoder(w).Encode(stateDetail))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(Config{
		BaseURL:     server.URL + "/api",
		AssistantID: "lead_agent",
		Timeout:     5 * time.Second,
	})

	result, err := client.RunWait(context.Background(), RunWaitRequest{
		ThreadID: "thread-err",
		Message:  "hi",
	})
	require.Nil(t, result)
	require.Error(t, err)

	var diag *RunDiagnosticError
	require.True(t, errors.As(err, &diag))
	require.Equal(t, "thread-err", diag.ThreadID)
	require.Equal(t, "run-err", diag.RunID)
	require.Equal(t, "error", diag.Status)
	require.Contains(t, diag.StateTail, "step=25")
	require.Contains(t, diag.StateTail, `tool:web_search[success]{"search output"}`)
	require.Contains(t, err.Error(), "thread_id=thread-err")
}

func TestRunWaitReturnsDiagnosticErrorWhenVisibleTextIsMissing(t *testing.T) {
	t.Parallel()

	waitState := map[string]any{
		"messages": []map[string]any{
			{"type": "human", "content": "hi"},
			{
				"type":              "ai",
				"content":           "",
				"tool_calls":        []map[string]any{{"name": "web_fetch", "id": "web_fetch:1"}},
				"response_metadata": map[string]any{"finish_reason": "tool_calls"},
			},
			{"type": "tool", "name": "web_fetch", "status": "success", "content": "fetched page"},
		},
	}
	stateDetail := map[string]any{
		"metadata": map[string]any{"step": 31},
		"values":   waitState,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/threads/thread-empty/runs/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "event: metadata\n")
			fmt.Fprint(w, "data: {\"run_id\":\"run-empty\",\"attempt\":1}\n\n")
			stateJSON, err := json.Marshal(waitState)
			require.NoError(t, err)
			fmt.Fprint(w, "event: values\n")
			fmt.Fprintf(w, "data: %s\n\n", stateJSON)
		case "/api/threads/thread-empty/runs":
			require.NoError(t, json.NewEncoder(w).Encode([]map[string]any{
				{"run_id": "run-empty", "status": "success"},
			}))
		case "/api/threads/thread-empty/state":
			require.NoError(t, json.NewEncoder(w).Encode(stateDetail))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(Config{
		BaseURL:     server.URL + "/api",
		AssistantID: "lead_agent",
		Timeout:     5 * time.Second,
	})

	result, err := client.RunWait(context.Background(), RunWaitRequest{
		ThreadID: "thread-empty",
		Message:  "hi",
	})
	require.Nil(t, result)
	require.Error(t, err)

	var diag *RunDiagnosticError
	require.True(t, errors.As(err, &diag))
	require.Equal(t, "thread-empty", diag.ThreadID)
	require.Equal(t, "run-empty", diag.RunID)
	require.Equal(t, "success", diag.Status)
	require.Contains(t, diag.StateTail, "step=31")
	require.Contains(t, err.Error(), "without visible text")
	require.Equal(t, map[string]any{
		"deerflow_thread_id":  "thread-empty",
		"deerflow_run_id":     "run-empty",
		"deerflow_run_status": "success",
		"deerflow_state_tail": diag.StateTail,
	}, diag.ProviderData())
}
