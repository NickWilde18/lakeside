package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	agentv1 "lakeside/api/agent/v1"

	"github.com/stretchr/testify/require"
)

type responseEnvelope[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

func TestAgentRunCreateLive(t *testing.T) {
	t.Helper()
	if os.Getenv("LAKESIDE_RUN_LIVE_TESTS") != "1" {
		t.Skip("set LAKESIDE_RUN_LIVE_TESTS=1 to enable live API tests")
	}

	resp := postAgentJSON[agentv1.AgentRunCreateRes](
		t,
		"/v1/agent/"+assistantKey()+"/runs",
		map[string]any{
			"message": envOrDefault("LAKESIDE_LIVE_QUERY_MESSAGE", "宿舍 WiFi 坏了，帮我报修"),
		},
	)

	require.Equal(t, 0, resp.Code)
	require.Equal(t, "OK", resp.Message)
	require.NotEmpty(t, resp.Data.RunID)
	require.NotEmpty(t, resp.Data.SessionID)
	require.NotEmpty(t, resp.Data.RunStatus)
}

func TestAgentRunGetLive(t *testing.T) {
	t.Helper()
	if os.Getenv("LAKESIDE_RUN_LIVE_TESTS") != "1" {
		t.Skip("set LAKESIDE_RUN_LIVE_TESTS=1 to enable live API tests")
	}
	runID := strings.TrimSpace(os.Getenv("LAKESIDE_LIVE_RUN_ID"))
	if runID == "" {
		t.Skip("set LAKESIDE_LIVE_RUN_ID to an existing run id")
	}
	resp := getAgentJSON[agentv1.AgentRunGetRes](t, "/v1/agent/"+assistantKey()+"/runs/"+runID)
	require.Equal(t, 0, resp.Code)
	require.Equal(t, "OK", resp.Message)
	require.Equal(t, runID, resp.Data.RunID)
	require.NotEmpty(t, resp.Data.RunStatus)
}

func TestAgentRunResumeLive(t *testing.T) {
	t.Helper()
	if os.Getenv("LAKESIDE_RUN_LIVE_TESTS") != "1" {
		t.Skip("set LAKESIDE_RUN_LIVE_TESTS=1 to enable live API tests")
	}
	runID := strings.TrimSpace(os.Getenv("LAKESIDE_LIVE_RESUME_RUN_ID"))
	if runID == "" {
		t.Skip("set LAKESIDE_LIVE_RESUME_RUN_ID to a waiting_input run id")
	}
	body := strings.TrimSpace(os.Getenv("LAKESIDE_LIVE_RESUME_BODY"))
	if body == "" {
		t.Skip("set LAKESIDE_LIVE_RESUME_BODY to a full JSON request body containing targets")
	}
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &payload))

	resp := postAgentJSON[agentv1.AgentRunResumeRes](t, "/v1/agent/"+assistantKey()+"/runs/"+runID+"/resume", payload)
	require.Equal(t, 0, resp.Code)
	require.Equal(t, "OK", resp.Message)
	require.NotEmpty(t, resp.Data.RunID)
	require.NotEmpty(t, resp.Data.SessionID)
	require.NotEmpty(t, resp.Data.RunStatus)
}

func postAgentJSON[T any](t *testing.T, path string, payload map[string]any) responseEnvelope[T] {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL()+path, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", envOrDefault("LAKESIDE_TEST_USER_ID", "122020255"))

	client := &http.Client{Timeout: liveHTTPTimeout()}
	httpResp, err := client.Do(req)
	require.NoError(t, err)
	defer httpResp.Body.Close()

	require.Equal(t, http.StatusOK, httpResp.StatusCode)

	var out responseEnvelope[T]
	require.NoError(t, json.NewDecoder(httpResp.Body).Decode(&out))
	return out
}

func getAgentJSON[T any](t *testing.T, path string) responseEnvelope[T] {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, baseURL()+path, nil)
	require.NoError(t, err)
	req.Header.Set("X-User-ID", envOrDefault("LAKESIDE_TEST_USER_ID", "122020255"))

	client := &http.Client{Timeout: liveHTTPTimeout()}
	httpResp, err := client.Do(req)
	require.NoError(t, err)
	defer httpResp.Body.Close()

	require.Equal(t, http.StatusOK, httpResp.StatusCode)

	var out responseEnvelope[T]
	require.NoError(t, json.NewDecoder(httpResp.Body).Decode(&out))
	return out
}

func baseURL() string {
	return envOrDefault("LAKESIDE_BASE_URL", "http://127.0.0.1:8011")
}

func assistantKey() string {
	return envOrDefault("LAKESIDE_TEST_ASSISTANT_KEY", "campus")
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func liveHTTPTimeout() time.Duration {
	secondsText := strings.TrimSpace(os.Getenv("LAKESIDE_LIVE_TIMEOUT_SECONDS"))
	if secondsText == "" {
		return 120 * time.Second
	}
	seconds, err := strconv.Atoi(secondsText)
	if err != nil || seconds <= 0 {
		return 120 * time.Second
	}
	return time.Duration(seconds) * time.Second
}

func TestLiveEnvExamples(t *testing.T) {
	t.Skip(fmt.Sprintf("example: LAKESIDE_RUN_LIVE_TESTS=1 LAKESIDE_TEST_USER_ID=122020255 go test ./test/integration -run Live"))
}
