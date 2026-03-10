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

// responseEnvelope 对应 GoFrame 统一响应包裹结构。
// live test 不直接关心中间件细节，只校验最外层 code/message/data。
type responseEnvelope[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

// TestAssistantQueryLive 用于联调本地常驻服务上的顶层 assistant query 接口。
// 这类测试默认关闭，只有显式设置 LAKESIDE_RUN_LIVE_TESTS=1 才会执行，
// 避免 CI 或日常 go test 因依赖本地服务而变得不稳定。
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

// TestAssistantResumeLive 用于联调 resume 接口。
// resume 依赖已有 session/checkpoint/interrupt，因此请求体由环境变量整包传入，
// 这样测试代码本身保持稳定，不把某个具体中断 ID 写死在仓库里。
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

// postAssistantJSON 是 live test 的最小 HTTP 调用封装。
// 这里故意不抽成复杂测试框架，只统一处理：
// 1. JSON 编码
// 2. X-User-ID 注入
// 3. 超时控制
// 4. GoFrame 标准响应解码
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

// baseURL 支持把 live test 指向其它环境，默认仍是本地 8011。
func baseURL() string {
	return envOrDefault("LAKESIDE_BASE_URL", "http://127.0.0.1:8011")
}

// envOrDefault 统一读取环境变量，避免每个测试重复写 trim + fallback。
func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

// liveHTTPTimeout 控制 live 调用超时。
// 大模型响应波动较大，所以这里做成环境变量可调，而不是把超时时间写死。
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

// TestLiveEnvExamples 仅用于给出最短运行示例，不参与实际断言。
func TestLiveEnvExamples(t *testing.T) {
	t.Skip(fmt.Sprintf("example: LAKESIDE_RUN_LIVE_TESTS=1 LAKESIDE_TEST_USER_ID=122020255 go test ./test/integration -run Live"))
}
