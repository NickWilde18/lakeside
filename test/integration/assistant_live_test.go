package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

func TestAgentSessionsLive(t *testing.T) {
	t.Helper()
	if os.Getenv("LAKESIDE_RUN_LIVE_TESTS") != "1" {
		t.Skip("set LAKESIDE_RUN_LIVE_TESTS=1 to enable live API tests")
	}

	resp := getAgentJSON[agentv1.AgentSessionsRes](t, "/v1/agent/"+assistantKey()+"/sessions?limit=5")
	require.Equal(t, 0, resp.Code)
	require.Equal(t, "OK", resp.Message)
	require.Equal(t, assistantKey(), resp.Data.AssistantKey)
}

func TestAgentRenderLive(t *testing.T) {
	t.Helper()
	if os.Getenv("LAKESIDE_RUN_LIVE_TESTS") != "1" {
		t.Skip("set LAKESIDE_RUN_LIVE_TESTS=1 to enable live API tests")
	}

	body := getAgentStreamBody(t, "/v1/agent/"+assistantKey()+"/render")
	require.Contains(t, body, "\"beginRendering\"")
	require.Contains(t, body, "\"surfaceUpdate\"")
}

func TestAgentActionsLive(t *testing.T) {
	t.Helper()
	if os.Getenv("LAKESIDE_RUN_LIVE_TESTS") != "1" {
		t.Skip("set LAKESIDE_RUN_LIVE_TESTS=1 to enable live API tests")
	}
	body := postAgentStreamBody(
		t,
		"/v1/agent/"+assistantKey()+"/actions",
		map[string]any{
			"userAction": map[string]any{
				"name":              "send_message",
				"surfaceId":         "agent-canvas",
				"sourceComponentId": "composer-send",
				"timestamp":         time.Now().UTC().Format(time.RFC3339),
				"context": map[string]any{
					"message": envOrDefault("LAKESIDE_LIVE_QUERY_MESSAGE", "学生群组邮箱地址是什么？"),
				},
			},
		},
	)
	require.Contains(t, body, "\"beginRendering\"")
	require.True(t, strings.Contains(body, "\"dataModelUpdate\"") || strings.Contains(body, "\"surfaceUpdate\""))
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

func getAgentStreamBody(t *testing.T, path string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, baseURL()+path, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/x-ndjson")
	req.Header.Set("X-User-ID", envOrDefault("LAKESIDE_TEST_USER_ID", "122020255@link.cuhk.edu.cn"))

	client := &http.Client{Timeout: liveHTTPTimeout()}
	httpResp, err := client.Do(req)
	require.NoError(t, err)
	defer httpResp.Body.Close()

	require.Equal(t, http.StatusOK, httpResp.StatusCode)
	require.Contains(t, httpResp.Header.Get("Content-Type"), "application/x-ndjson")
	body, err := io.ReadAll(httpResp.Body)
	require.NoError(t, err)
	return string(body)
}

func postAgentStreamBody(t *testing.T, path string, payload map[string]any) string {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL()+path, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Accept", "application/x-ndjson")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", envOrDefault("LAKESIDE_TEST_USER_ID", "122020255@link.cuhk.edu.cn"))

	client := &http.Client{Timeout: liveHTTPTimeout()}
	httpResp, err := client.Do(req)
	require.NoError(t, err)
	defer httpResp.Body.Close()

	require.Equal(t, http.StatusOK, httpResp.StatusCode)
	require.Contains(t, httpResp.Header.Get("Content-Type"), "application/x-ndjson")
	responseBody, err := io.ReadAll(httpResp.Body)
	require.NoError(t, err)
	return string(responseBody)
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
	t.Skip(fmt.Sprintf("example: LAKESIDE_RUN_LIVE_TESTS=1 LAKESIDE_TEST_USER_ID=122020255@link.cuhk.edu.cn go test ./test/integration -run Live"))
}
