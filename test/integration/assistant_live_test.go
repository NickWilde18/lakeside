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

	assistantv1 "lakeside/api/assistant/v1"

	"github.com/stretchr/testify/require"
)

type responseEnvelope[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

func TestAssistantQueryLive(t *testing.T) {
	t.Helper()
	if os.Getenv("LAKESIDE_RUN_LIVE_TESTS") != "1" {
		t.Skip("set LAKESIDE_RUN_LIVE_TESTS=1 to enable live API tests")
	}

	resp := postAssistantJSON[assistantv1.AssistantQueryRes](
		t,
		"/v1/assistant/query",
		map[string]any{
			"message": envOrDefault("LAKESIDE_LIVE_QUERY_MESSAGE", "宿舍 WiFi 坏了，帮我报修"),
		},
	)

	require.Equal(t, 0, resp.Code)
	require.Empty(t, resp.Message)
	require.NotEmpty(t, resp.Data.SessionID)
	require.NotEmpty(t, resp.Data.ActiveAgent)
	require.NotEmpty(t, resp.Data.Status)
}

func TestAssistantResumeLive(t *testing.T) {
	t.Helper()
	if os.Getenv("LAKESIDE_RUN_LIVE_TESTS") != "1" {
		t.Skip("set LAKESIDE_RUN_LIVE_TESTS=1 to enable live API tests")
	}

	body := strings.TrimSpace(os.Getenv("LAKESIDE_LIVE_RESUME_BODY"))
	if body == "" {
		t.Skip("set LAKESIDE_LIVE_RESUME_BODY to a full JSON request body")
	}

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &payload))

	resp := postAssistantJSON[assistantv1.AssistantResumeRes](t, "/v1/assistant/resume", payload)

	require.Equal(t, 0, resp.Code)
	require.Empty(t, resp.Message)
	require.NotEmpty(t, resp.Data.SessionID)
	require.NotEmpty(t, resp.Data.ActiveAgent)
	require.NotEmpty(t, resp.Data.Status)
}

func postAssistantJSON[T any](t *testing.T, path string, payload map[string]any) responseEnvelope[T] {
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

func baseURL() string {
	return envOrDefault("LAKESIDE_BASE_URL", "http://127.0.0.1:8011")
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
