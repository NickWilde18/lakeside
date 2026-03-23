package deerflowclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const deerFlowRunStatusPollInterval = 2 * time.Second

func (c *Client) streamRun(ctx context.Context, threadID string, payload map[string]any, onTraceUpdate func(*Trace)) (string, map[string]any, string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", nil, "", fmt.Errorf("marshal deerflow stream request failed: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/threads/"+threadID+"/runs/stream", bytes.NewReader(body))
	if err != nil {
		return "", nil, "", fmt.Errorf("create deerflow stream request failed: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return "", nil, "", fmt.Errorf("call deerflow stream failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(response.Body)
		return "", nil, "", fmt.Errorf("deerflow returned %d: %s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	reader := bufio.NewReader(response.Body)
	var (
		currentEvent      string
		dataLines         []string
		runID             string
		lastRunStatus     string
		latestState       map[string]any
		lastTraceSnapshot string
		lastPollAt        time.Time
	)
	checkTerminal := func(force bool) (bool, error) {
		if !force && time.Since(lastPollAt) < deerFlowRunStatusPollInterval {
			return false, nil
		}
		lastPollAt = time.Now()
		run, err := c.latestRun(ctx, threadID)
		if err != nil || run == nil {
			return false, nil
		}
		if runID == "" {
			runID = strings.TrimSpace(run.RunID)
		}
		lastRunStatus = strings.TrimSpace(run.Status)
		return isDeerFlowRunTerminal(lastRunStatus), nil
	}
	flushEvent := func() error {
		eventName := strings.TrimSpace(currentEvent)
		payloadText := strings.TrimSpace(strings.Join(dataLines, "\n"))
		currentEvent = ""
		dataLines = nil
		if eventName == "" || payloadText == "" {
			return nil
		}
		switch eventName {
		case "metadata":
			var metadata struct {
				RunID string `json:"run_id"`
			}
			if err := json.Unmarshal([]byte(payloadText), &metadata); err == nil {
				runID = strings.TrimSpace(metadata.RunID)
			}
		case "values":
			var state map[string]any
			if err := json.Unmarshal([]byte(payloadText), &state); err != nil {
				return fmt.Errorf("decode deerflow values event failed: %w", err)
			}
			latestState = state
			if onTraceUpdate != nil {
				trace := buildTrace(state, threadID, runID, lastRunStatus, "")
				if trace != nil {
					serialized := traceSignature(trace)
					if serialized != lastTraceSnapshot {
						lastTraceSnapshot = serialized
						onTraceUpdate(trace)
					}
				}
			}
		}
		return nil
	}

	for {
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			trimmed := strings.TrimRight(line, "\r\n")
			switch {
			case strings.HasPrefix(trimmed, "event:"):
				currentEvent = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
			case strings.HasPrefix(trimmed, "data:"):
				dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
			case strings.HasPrefix(trimmed, ":"):
				// Heartbeat/comment; terminal run status is polled on the following blank line.
			case trimmed == "":
				if err := flushEvent(); err != nil {
					return runID, latestState, lastRunStatus, err
				}
				terminal, err := checkTerminal(false)
				if err != nil {
					return runID, latestState, lastRunStatus, err
				}
				if terminal {
					return runID, latestState, lastRunStatus, nil
				}
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				if err := flushEvent(); err != nil {
					return runID, latestState, lastRunStatus, err
				}
				return runID, latestState, lastRunStatus, nil
			}
			if ctx.Err() != nil {
				return runID, latestState, lastRunStatus, ctx.Err()
			}
			return runID, latestState, lastRunStatus, fmt.Errorf("read deerflow stream failed: %w", readErr)
		}
	}
}

func buildRequestMessages(message, preferredLanguage string) []map[string]any {
	messages := make([]map[string]any, 0, 2)
	if policy := researchQualityPolicy(preferredLanguage); policy != "" {
		messages = append(messages, map[string]any{
			"role":    "system",
			"content": policy,
		})
	}
	messages = append(messages, map[string]any{
		"role":    "human",
		"content": message,
	})
	return messages
}

func researchQualityPolicy(preferredLanguage string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(preferredLanguage)), "zh") {
		return strings.TrimSpace(`
你是 Lakeside 集成的 DeerFlow 研究执行器。执行研究任务时必须遵守以下约束：
- 优先官方文档、厂商 API 定价页、论文、benchmark 官方榜单和权威技术文档；对于模型对比，优先一手来源。
- 只有在高质量来源不足时，才使用论坛、自媒体、聚合站或二手转载，并在结论中明确说明证据较弱。
- 不要发起空查询、单个符号查询、明显重复的低信息量查询；搜索词必须具体、可验证。
- 不要把 URL、片段 ID、tool_call 占位符或类似 ":9"、"://example.com" 的字符串当作搜索词；需要打开具体链接时，先从搜索结果或已有来源里提取完整 URL，再调用抓取工具。
- 当抓取失败、页面无正文或不同来源冲突时，必须保留不确定性，不要把猜测写成事实。
- 最终回答必须保留可追溯来源，并尽量把结论建立在多源交叉验证之上。
- 回答语言跟随用户。`)
	}
	return strings.TrimSpace(`
You are the DeerFlow research executor behind Lakeside. Follow these constraints:
- Prefer official documentation, vendor pricing pages, papers, benchmark leaderboards, and primary technical sources.
- Only use community posts, forums, aggregators, or republished articles when stronger sources are unavailable, and explicitly note weaker evidence.
- Never issue empty, punctuation-only, or obviously low-information search queries; every search must be specific and verifiable.
- Never use raw URLs, fragment IDs, tool-call placeholders, or strings like ":9" / "://example.com" as search queries; when you need a specific page, extract a full valid URL from prior results and then use the fetch tool.
- If fetches fail, pages have no extractable content, or sources conflict, preserve the uncertainty instead of turning guesses into facts.
- Keep answers source-traceable and prefer conclusions backed by cross-checked sources.
- Reply in the user's language.`)
}

func traceSignature(trace *Trace) string {
	if trace == nil {
		return ""
	}
	data, err := json.Marshal(trace)
	if err != nil {
		return ""
	}
	return string(data)
}
