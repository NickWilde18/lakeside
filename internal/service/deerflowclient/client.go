package deerflowclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultDeerFlowRunRecursionLimit = 100

type Config struct {
	BaseURL         string
	AssistantID     string
	DefaultModel    string
	Timeout         time.Duration
	ThinkingEnabled bool
	PlanMode        bool
}

type RunWaitRequest struct {
	ThreadID          string
	Message           string
	SessionID         string
	UserUPN           string
	PreferredLanguage string
	OnTraceUpdate     func(*Trace)
}

type RunWaitResult struct {
	ThreadID  string
	RunID     string
	RunStatus string
	Message   string
	Artifacts []string
	RawState  map[string]any
	Trace     *Trace
}

type RunDiagnosticError struct {
	ThreadID  string
	RunID     string
	Status    string
	StateTail string
	Cause     string
}

func (e *RunDiagnosticError) Error() string {
	if e == nil {
		return "deerflow run failed"
	}
	parts := make([]string, 0, 4)
	if cause := strings.TrimSpace(e.Cause); cause != "" {
		parts = append(parts, cause)
	} else {
		parts = append(parts, "deerflow run failed")
	}
	if status := strings.TrimSpace(e.Status); status != "" {
		parts = append(parts, fmt.Sprintf("status=%s", status))
	}
	if threadID := strings.TrimSpace(e.ThreadID); threadID != "" {
		parts = append(parts, fmt.Sprintf("thread_id=%s", threadID))
	}
	if runID := strings.TrimSpace(e.RunID); runID != "" {
		parts = append(parts, fmt.Sprintf("run_id=%s", runID))
	}
	message := strings.Join(parts, ", ")
	if tail := strings.TrimSpace(e.StateTail); tail != "" {
		message += fmt.Sprintf(", state_tail=%s", tail)
	}
	return message
}

func (e *RunDiagnosticError) ProviderData() map[string]any {
	if e == nil {
		return nil
	}
	data := make(map[string]any)
	if threadID := strings.TrimSpace(e.ThreadID); threadID != "" {
		data["deerflow_thread_id"] = threadID
	}
	if runID := strings.TrimSpace(e.RunID); runID != "" {
		data["deerflow_run_id"] = runID
	}
	if status := strings.TrimSpace(e.Status); status != "" {
		data["deerflow_run_status"] = status
	}
	if tail := strings.TrimSpace(e.StateTail); tail != "" {
		data["deerflow_state_tail"] = tail
	}
	if len(data) == 0 {
		return nil
	}
	return data
}

type Client struct {
	baseURL         string
	assistantID     string
	defaultModel    string
	thinkingEnabled bool
	planMode        bool
	httpClient      *http.Client
}

func New(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	return &Client{
		baseURL:         strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		assistantID:     strings.TrimSpace(cfg.AssistantID),
		defaultModel:    strings.TrimSpace(cfg.DefaultModel),
		thinkingEnabled: cfg.ThinkingEnabled,
		planMode:        cfg.PlanMode,
		httpClient:      &http.Client{Timeout: timeout},
	}
}

func (c *Client) EnsureThread(ctx context.Context, threadID string, metadata map[string]any) (string, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID != "" {
		return threadID, nil
	}
	if c == nil || c.baseURL == "" {
		return "", fmt.Errorf("deerflow client is not configured")
	}
	payload := map[string]any{
		"metadata": metadata,
	}
	body, err := c.doJSON(ctx, http.MethodPost, c.baseURL+"/threads", payload)
	if err != nil {
		return "", err
	}
	var result struct {
		ThreadID string `json:"thread_id"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("decode deerflow thread response failed: %w", err)
	}
	if strings.TrimSpace(result.ThreadID) == "" {
		return "", fmt.Errorf("deerflow thread_id is empty")
	}
	return strings.TrimSpace(result.ThreadID), nil
}

func (c *Client) RunWait(ctx context.Context, req RunWaitRequest) (*RunWaitResult, error) {
	if c == nil || c.baseURL == "" {
		return nil, fmt.Errorf("deerflow client is not configured")
	}
	message := strings.TrimSpace(req.Message)
	if message == "" {
		return nil, fmt.Errorf("deerflow request message is empty")
	}
	threadID, err := c.EnsureThread(ctx, req.ThreadID, map[string]any{
		"user_upn":   strings.TrimSpace(req.UserUPN),
		"session_id": strings.TrimSpace(req.SessionID),
		"source":     "lakeside",
	})
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"assistant_id": c.assistantID,
		"input": map[string]any{
			"messages": buildRequestMessages(message, req.PreferredLanguage),
		},
		"context": map[string]any{
			"thread_id":          threadID,
			"user_upn":           strings.TrimSpace(req.UserUPN),
			"session_id":         strings.TrimSpace(req.SessionID),
			"preferred_language": strings.TrimSpace(req.PreferredLanguage),
			"thinking_enabled":   c.thinkingEnabled,
			"is_plan_mode":       c.planMode,
		},
		"config": map[string]any{
			"recursion_limit": defaultDeerFlowRunRecursionLimit,
		},
	}
	if c.defaultModel != "" {
		payloadContext := payload["context"].(map[string]any)
		payloadContext["model_name"] = c.defaultModel
	}
	runID, streamedState, streamedStatus, err := c.streamRun(ctx, threadID, payload, req.OnTraceUpdate)
	if err != nil {
		return nil, err
	}
	state := streamedState
	if latestState, stateErr := c.threadState(ctx, threadID); stateErr == nil && len(latestState) > 0 {
		state = latestState
	}
	result := &RunWaitResult{
		ThreadID:  threadID,
		RunID:     strings.TrimSpace(runID),
		RunStatus: strings.TrimSpace(streamedStatus),
		Message:   extractResponseText(state),
		Artifacts: extractArtifacts(state),
		RawState:  state,
	}
	run, err := c.latestRun(ctx, threadID)
	if err == nil && run != nil {
		if result.RunID == "" {
			result.RunID = strings.TrimSpace(run.RunID)
		}
		result.RunStatus = strings.TrimSpace(run.Status)
	}
	result.Trace = buildTrace(state, result.ThreadID, result.RunID, result.RunStatus, summarizeStateTail(state))
	if req.OnTraceUpdate != nil && result.Trace != nil {
		req.OnTraceUpdate(result.Trace)
	}
	if !isDeerFlowRunSuccess(result.RunStatus) {
		diagState := state
		if latestState, stateErr := c.threadState(ctx, threadID); stateErr == nil && len(latestState) > 0 {
			diagState = latestState
		}
		return nil, &RunDiagnosticError{
			ThreadID:  threadID,
			RunID:     result.RunID,
			Status:    result.RunStatus,
			StateTail: summarizeStateTail(diagState),
			Cause:     fmt.Sprintf("deerflow run failed with status %s", chooseDiagnosticStatus(result.RunStatus)),
		}
	}
	if strings.TrimSpace(result.Message) == "" {
		diagState := state
		if latestState, stateErr := c.threadState(ctx, threadID); stateErr == nil && len(latestState) > 0 {
			diagState = latestState
		}
		return nil, &RunDiagnosticError{
			ThreadID:  threadID,
			RunID:     result.RunID,
			Status:    result.RunStatus,
			StateTail: summarizeStateTail(diagState),
			Cause:     "deerflow completed without visible text",
		}
	}
	return result, nil
}

func (c *Client) doJSON(ctx context.Context, method, url string, payload any) ([]byte, error) {
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal deerflow request failed: %w", err)
		}
	}
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, fmt.Errorf("create deerflow request failed: %w", err)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Accept", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call deerflow failed: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read deerflow response failed: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("deerflow returned %d: %s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	return responseBody, nil
}

type deerflowRunInfo struct {
	RunID  string `json:"run_id"`
	Status string `json:"status"`
}

func (c *Client) latestRun(ctx context.Context, threadID string) (*deerflowRunInfo, error) {
	body, err := c.doJSON(ctx, http.MethodGet, c.baseURL+"/threads/"+threadID+"/runs", nil)
	if err != nil {
		return nil, err
	}
	var runs []deerflowRunInfo
	if err := json.Unmarshal(body, &runs); err != nil {
		return nil, fmt.Errorf("decode deerflow run list failed: %w", err)
	}
	if len(runs) == 0 {
		return nil, fmt.Errorf("deerflow run list is empty")
	}
	copied := runs[0]
	return &copied, nil
}

func (c *Client) threadState(ctx context.Context, threadID string) (map[string]any, error) {
	body, err := c.doJSON(ctx, http.MethodGet, c.baseURL+"/threads/"+threadID+"/state", nil)
	if err != nil {
		return nil, err
	}
	var state map[string]any
	if err := json.Unmarshal(body, &state); err != nil {
		return nil, fmt.Errorf("decode deerflow thread state failed: %w", err)
	}
	return state, nil
}

func extractResponseText(result map[string]any) string {
	messages := stateMessages(result)
	for i := len(messages) - 1; i >= 0; i-- {
		msg, ok := messages[i].(map[string]any)
		if !ok {
			continue
		}
		msgType := strings.TrimSpace(strings.ToLower(toString(msg["type"])))
		if msgType == "human" {
			break
		}
		if msgType == "tool" && strings.TrimSpace(toString(msg["name"])) == "ask_clarification" {
			if text := messageVisibleText(msg); text != "" {
				return text
			}
		}
		if msgType == "ai" {
			if text := messageVisibleText(msg); text != "" {
				return text
			}
		}
	}
	return ""
}

func extractArtifacts(result map[string]any) []string {
	messages := stateMessages(result)
	artifacts := make([]string, 0)
	seen := make(map[string]struct{})
	for i := len(messages) - 1; i >= 0; i-- {
		msg, ok := messages[i].(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(strings.ToLower(toString(msg["type"]))) == "human" {
			break
		}
		if strings.TrimSpace(strings.ToLower(toString(msg["type"]))) != "ai" {
			continue
		}
		toolCalls, _ := msg["tool_calls"].([]any)
		for _, item := range toolCalls {
			call, ok := item.(map[string]any)
			if !ok || strings.TrimSpace(toString(call["name"])) != "present_files" {
				continue
			}
			args, _ := call["args"].(map[string]any)
			files, _ := args["files"].([]any)
			for _, file := range files {
				path := strings.TrimSpace(toString(file))
				if path == "" {
					continue
				}
				if _, ok := seen[path]; ok {
					continue
				}
				seen[path] = struct{}{}
				artifacts = append(artifacts, path)
			}
		}
	}
	return artifacts
}

func stateMessages(result map[string]any) []any {
	if result == nil {
		return nil
	}
	if messages, ok := result["messages"].([]any); ok {
		return messages
	}
	values, _ := result["values"].(map[string]any)
	messages, _ := values["messages"].([]any)
	return messages
}

func contentToText(content any) string {
	switch value := content.(type) {
	case string:
		return strings.TrimSpace(value)
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			switch block := item.(type) {
			case string:
				text := strings.TrimSpace(block)
				if text != "" {
					parts = append(parts, text)
				}
			case map[string]any:
				if text := strings.TrimSpace(toString(block["text"])); text != "" {
					parts = append(parts, text)
				} else if nested := strings.TrimSpace(toString(block["content"])); nested != "" {
					parts = append(parts, nested)
				}
			}
		}
		return strings.TrimSpace(strings.Join(parts, ""))
	case map[string]any:
		if text := strings.TrimSpace(toString(value["text"])); text != "" {
			return text
		}
		return strings.TrimSpace(toString(value["content"]))
	default:
		return ""
	}
}

func toString(value any) string {
	switch item := value.(type) {
	case string:
		return item
	case fmt.Stringer:
		return item.String()
	default:
		return ""
	}
}

func isDeerFlowRunSuccess(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "success", "succeeded", "done", "completed":
		return true
	default:
		return false
	}
}

func isDeerFlowRunTerminal(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success", "succeeded", "done", "completed", "error", "failed", "cancelled", "canceled":
		return true
	default:
		return false
	}
}

func chooseDiagnosticStatus(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return "unknown"
	}
	return status
}

func summarizeStateTail(state map[string]any) string {
	if len(state) == 0 {
		return ""
	}
	parts := make([]string, 0, 6)
	if metadata, ok := state["metadata"].(map[string]any); ok {
		if step, ok := metadata["step"]; ok {
			parts = append(parts, fmt.Sprintf("step=%v", step))
		}
	}
	messages := stateMessages(state)
	if len(messages) > 4 {
		messages = messages[len(messages)-4:]
	}
	for _, item := range messages {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		msgType := strings.TrimSpace(strings.ToLower(toString(msg["type"])))
		summary := msgType
		if msgType == "tool" {
			if name := strings.TrimSpace(toString(msg["name"])); name != "" {
				summary += ":" + name
			}
			if status := strings.TrimSpace(strings.ToLower(toString(msg["status"]))); status != "" {
				summary += "[" + status + "]"
			}
		}
		if msgType == "ai" {
			toolCalls, _ := msg["tool_calls"].([]any)
			if len(toolCalls) > 0 {
				summary += fmt.Sprintf("[tool_calls=%d]", len(toolCalls))
			}
			if responseMetadata, ok := msg["response_metadata"].(map[string]any); ok {
				if finish := strings.TrimSpace(toString(responseMetadata["finish_reason"])); finish != "" {
					summary += fmt.Sprintf("[finish=%s]", finish)
				}
			}
		}
		if snippet := truncateDiagnosticText(contentToText(msg["content"])); snippet != "" {
			summary += fmt.Sprintf("{%q}", snippet)
		}
		parts = append(parts, summary)
	}
	return strings.Join(parts, " | ")
}

func truncateDiagnosticText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.Join(strings.Fields(text), " ")
	const limit = 96
	if len(text) <= limit {
		return text
	}
	return strings.TrimSpace(text[:limit-3]) + "..."
}
