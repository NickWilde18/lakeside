package deerflowclient

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

type Trace struct {
	ThreadID  string         `json:"thread_id,omitempty"`
	RunID     string         `json:"run_id,omitempty"`
	RunStatus string         `json:"run_status,omitempty"`
	Title     string         `json:"title,omitempty"`
	StateTail string         `json:"state_tail,omitempty"`
	Messages  []TraceMessage `json:"messages,omitempty"`
	Todos     []TraceTodo    `json:"todos,omitempty"`
	Artifacts []string       `json:"artifacts,omitempty"`
	Sources   []TraceSource  `json:"sources,omitempty"`
}

type TraceMessage struct {
	ID         string          `json:"id,omitempty"`
	Type       string          `json:"type,omitempty"`
	Name       string          `json:"name,omitempty"`
	Content    string          `json:"content,omitempty"`
	Reasoning  string          `json:"reasoning,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolCalls  []TraceToolCall `json:"tool_calls,omitempty"`
	Status     string          `json:"status,omitempty"`
}

type TraceToolCall struct {
	ID      string         `json:"id,omitempty"`
	Name    string         `json:"name,omitempty"`
	Args    map[string]any `json:"args,omitempty"`
	Result  any            `json:"result,omitempty"`
	Status  string         `json:"status,omitempty"`
	Error   string         `json:"error,omitempty"`
	Sources []TraceSource  `json:"sources,omitempty"`
}

type TraceTodo struct {
	Content string `json:"content,omitempty"`
	Status  string `json:"status,omitempty"`
}

type TraceSource struct {
	Title         string `json:"title,omitempty"`
	URL           string `json:"url,omitempty"`
	Domain        string `json:"domain,omitempty"`
	Snippet       string `json:"snippet,omitempty"`
	Query         string `json:"query,omitempty"`
	ToolName      string `json:"tool_name,omitempty"`
	ToolCallID    string `json:"tool_call_id,omitempty"`
	SourceType    string `json:"source_type,omitempty"`
	Quality       string `json:"quality,omitempty"`
	LowConfidence bool   `json:"low_confidence,omitempty"`
}

var thinkTagRE = regexp.MustCompile(`(?s)<think>\s*(.*?)\s*</think>`)

func buildTrace(state map[string]any, threadID, runID, runStatus, stateTail string) *Trace {
	values := stateValueMap(state)
	messages := stateMessages(state)
	todos := extractTodos(values)
	artifacts := extractArtifacts(state)
	title := strings.TrimSpace(toString(values["title"]))

	if len(messages) == 0 && len(todos) == 0 && len(artifacts) == 0 && title == "" {
		return nil
	}

	toolMessages := make(map[string]map[string]any)
	for _, item := range messages {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(strings.ToLower(toString(msg["type"]))) != "tool" {
			continue
		}
		toolCallID := strings.TrimSpace(toString(msg["tool_call_id"]))
		if toolCallID == "" {
			continue
		}
		toolMessages[toolCallID] = msg
	}

	trace := &Trace{
		ThreadID:  strings.TrimSpace(threadID),
		RunID:     strings.TrimSpace(runID),
		RunStatus: strings.TrimSpace(runStatus),
		Title:     title,
		StateTail: strings.TrimSpace(stateTail),
		Todos:     todos,
		Artifacts: artifacts,
	}
	sourceSeen := make(map[string]struct{})
	for _, item := range messages {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		msgType := strings.TrimSpace(strings.ToLower(toString(msg["type"])))
		if msgType == "" || msgType == "system" {
			continue
		}
		traceMessage := TraceMessage{
			ID:         strings.TrimSpace(toString(msg["id"])),
			Type:       msgType,
			Name:       strings.TrimSpace(toString(msg["name"])),
			ToolCallID: strings.TrimSpace(toString(msg["tool_call_id"])),
			Status:     strings.TrimSpace(strings.ToLower(toString(msg["status"]))),
		}
		switch msgType {
		case "ai":
			traceMessage.Content = messageVisibleText(msg)
			traceMessage.Reasoning = messageReasoningText(msg)
			toolCalls, _ := msg["tool_calls"].([]any)
			for _, rawCall := range toolCalls {
				callMap, ok := rawCall.(map[string]any)
				if !ok {
					continue
				}
				traceCall := normalizeToolCall(callMap, toolMessages[strings.TrimSpace(toString(callMap["id"]))])
				traceMessage.ToolCalls = append(traceMessage.ToolCalls, traceCall)
				for _, source := range traceCall.Sources {
					key := dedupeSourceKey(source)
					if key == "" {
						continue
					}
					if _, exists := sourceSeen[key]; exists {
						continue
					}
					sourceSeen[key] = struct{}{}
					trace.Sources = append(trace.Sources, source)
				}
			}
		case "tool":
			traceMessage.Content = previewText(contentToText(msg["content"]), 360)
		default:
			traceMessage.Content = messageVisibleText(msg)
		}
		if traceMessage.Content == "" && traceMessage.Reasoning == "" && len(traceMessage.ToolCalls) == 0 && msgType != "tool" {
			continue
		}
		trace.Messages = append(trace.Messages, traceMessage)
	}
	if len(trace.Messages) == 0 && len(trace.Todos) == 0 && len(trace.Artifacts) == 0 && trace.Title == "" {
		return nil
	}
	return trace
}

func normalizeToolCall(rawCall map[string]any, toolMessage map[string]any) TraceToolCall {
	callID := strings.TrimSpace(toString(rawCall["id"]))
	name := strings.TrimSpace(toString(rawCall["name"]))
	args, _ := rawCall["args"].(map[string]any)
	status := strings.TrimSpace(strings.ToLower(toString(toolMessage["status"])))
	resultText := contentToText(toolMessage["content"])
	traceCall := TraceToolCall{
		ID:     callID,
		Name:   name,
		Args:   cloneMap(args),
		Result: compactToolResult(name, args, resultText),
		Status: status,
	}
	if traceCall.Status == "" && resultText != "" {
		traceCall.Status = "success"
	}
	if traceCall.Status == "error" || strings.HasPrefix(strings.ToLower(strings.TrimSpace(resultText)), "error:") {
		traceCall.Error = previewText(resultText, 240)
	}
	traceCall.Sources = sourcesFromToolResult(name, callID, args, traceCall.Result)
	return traceCall
}

func compactToolResult(name string, args map[string]any, resultText string) any {
	name = strings.TrimSpace(strings.ToLower(name))
	switch name {
	case "web_search":
		if results := parseSearchResults(resultText); len(results) > 0 {
			return results
		}
		return map[string]any{
			"query":   strings.TrimSpace(toString(args["query"])),
			"preview": previewText(resultText, 320),
		}
	case "web_fetch":
		pageURL := strings.TrimSpace(toString(args["url"]))
		title := extractMarkdownTitle(resultText)
		if title == "" {
			title = pageURL
		}
		return map[string]any{
			"url":     pageURL,
			"title":   title,
			"preview": previewText(resultText, 360),
		}
	case "write_todos":
		if todos := parseTodos(resultText); len(todos) > 0 {
			return todos
		}
		return previewText(resultText, 240)
	case "read_file", "write_file", "str_replace", "ls":
		return map[string]any{
			"path":    strings.TrimSpace(toString(args["path"])),
			"preview": previewText(resultText, 280),
		}
	case "bash":
		return map[string]any{
			"command": strings.TrimSpace(toString(args["command"])),
			"preview": previewText(resultText, 280),
		}
	case "ask_clarification":
		return map[string]any{
			"question": previewText(resultText, 240),
		}
	default:
		parsed := parseJSONValue(resultText)
		if parsed != nil {
			return parsed
		}
		return previewText(resultText, 320)
	}
}

func sourcesFromToolResult(name, toolCallID string, args map[string]any, result any) []TraceSource {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "web_search":
		query := strings.TrimSpace(toString(args["query"]))
		items, ok := result.([]map[string]any)
		if !ok {
			return nil
		}
		sources := make([]TraceSource, 0, len(items))
		for _, item := range items {
			source := traceSourceFromFields(
				strings.TrimSpace(toString(item["title"])),
				strings.TrimSpace(toString(item["url"])),
				strings.TrimSpace(toString(item["snippet"])),
				query,
				name,
				toolCallID,
			)
			if source.URL == "" {
				continue
			}
			sources = append(sources, source)
		}
		return sources
	case "web_fetch":
		page, ok := result.(map[string]any)
		if !ok {
			return nil
		}
		source := traceSourceFromFields(
			strings.TrimSpace(toString(page["title"])),
			strings.TrimSpace(toString(page["url"])),
			strings.TrimSpace(toString(page["preview"])),
			"",
			name,
			toolCallID,
		)
		if source.URL == "" {
			return nil
		}
		return []TraceSource{source}
	default:
		return nil
	}
}

func traceSourceFromFields(title, rawURL, snippet, query, toolName, toolCallID string) TraceSource {
	sourceType, quality, lowConfidence, domain := classifySource(rawURL)
	return TraceSource{
		Title:         strings.TrimSpace(title),
		URL:           strings.TrimSpace(rawURL),
		Domain:        domain,
		Snippet:       previewText(snippet, 320),
		Query:         strings.TrimSpace(query),
		ToolName:      strings.TrimSpace(toolName),
		ToolCallID:    strings.TrimSpace(toolCallID),
		SourceType:    sourceType,
		Quality:       quality,
		LowConfidence: lowConfidence,
	}
}

func classifySource(rawURL string) (sourceType string, quality string, lowConfidence bool, domain string) {
	domain = normalizeDomain(rawURL)
	switch {
	case domain == "":
		return "unknown", "unknown", false, ""
	case domainMatches(domain, "platform.moonshot.cn"),
		domainMatches(domain, "platform.moonshot.ai"),
		domainMatches(domain, "moonshot.ai"),
		domainMatches(domain, "kimi.com"),
		domainMatches(domain, "docs.z.ai"),
		domainMatches(domain, "z.ai"),
		domainMatches(domain, "bigmodel.cn"),
		domainMatches(domain, "open.bigmodel.cn"):
		return "official", "high", false, domain
	case domainMatches(domain, "xbench.org"),
		domainMatches(domain, "artificialanalysis.ai"),
		domainMatches(domain, "livebench.ai"):
		return "benchmark", "high", false, domain
	case domainMatches(domain, "openrouter.ai"),
		domainMatches(domain, "huggingface.co"),
		strings.HasPrefix(domain, "docs."):
		return "provider", "medium", false, domain
	case domainMatches(domain, "youtube.com"),
		domainMatches(domain, "youtu.be"),
		domainMatches(domain, "reddit.com"),
		domainMatches(domain, "zhihu.com"),
		domainMatches(domain, "medium.com"),
		domainMatches(domain, "cnblogs.com"),
		domainMatches(domain, "sohu.com"),
		domainMatches(domain, "eesel.ai"),
		domainMatches(domain, "costgoat.com"),
		domainMatches(domain, "pricepertoken.com"):
		return "community", "low", true, domain
	case domainMatches(domain, "news.qq.com"),
		domainMatches(domain, "finance.sina.com.cn"),
		domainMatches(domain, "qbitai.com"),
		domainMatches(domain, "36kr.com"):
		return "media", "medium", false, domain
	default:
		return "unknown", "medium", false, domain
	}
}

func extractTodos(values map[string]any) []TraceTodo {
	if len(values) == 0 {
		return nil
	}
	rawTodos, _ := values["todos"].([]any)
	if len(rawTodos) == 0 {
		return nil
	}
	todos := make([]TraceTodo, 0, len(rawTodos))
	for _, item := range rawTodos {
		mapped, ok := item.(map[string]any)
		if !ok {
			continue
		}
		content := strings.TrimSpace(toString(mapped["content"]))
		status := strings.TrimSpace(strings.ToLower(toString(mapped["status"])))
		if content == "" && status == "" {
			continue
		}
		todos = append(todos, TraceTodo{
			Content: content,
			Status:  status,
		})
	}
	return todos
}

func parseSearchResults(text string) []map[string]any {
	parsed := parseJSONValue(text)
	items, ok := parsed.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	results := make([]map[string]any, 0, len(items))
	for _, item := range items {
		mapped, ok := item.(map[string]any)
		if !ok {
			continue
		}
		entry := map[string]any{
			"title":   strings.TrimSpace(toString(mapped["title"])),
			"url":     strings.TrimSpace(toString(mapped["url"])),
			"snippet": previewText(strings.TrimSpace(toString(mapped["snippet"])), 320),
		}
		if strings.TrimSpace(toString(entry["url"])) == "" {
			continue
		}
		results = append(results, entry)
		if len(results) >= 5 {
			break
		}
	}
	return results
}

func parseTodos(text string) []map[string]any {
	parsed := parseJSONValue(text)
	items, ok := parsed.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	todos := make([]map[string]any, 0, len(items))
	for _, item := range items {
		mapped, ok := item.(map[string]any)
		if !ok {
			continue
		}
		content := strings.TrimSpace(toString(mapped["content"]))
		status := strings.TrimSpace(strings.ToLower(toString(mapped["status"])))
		if content == "" && status == "" {
			continue
		}
		todos = append(todos, map[string]any{
			"content": content,
			"status":  status,
		})
	}
	return todos
}

func parseJSONValue(text string) any {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if !strings.HasPrefix(text, "{") && !strings.HasPrefix(text, "[") {
		return nil
	}
	var parsed any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return nil
	}
	return parsed
}

func messageVisibleText(msg map[string]any) string {
	if msg == nil {
		return ""
	}
	text := contentToText(msg["content"])
	if strings.TrimSpace(strings.ToLower(toString(msg["type"]))) != "ai" {
		return text
	}
	visible, _ := splitInlineReasoning(text)
	return visible
}

func messageReasoningText(msg map[string]any) string {
	if msg == nil {
		return ""
	}
	additional, _ := msg["additional_kwargs"].(map[string]any)
	if reasoning := strings.TrimSpace(toString(additional["reasoning_content"])); reasoning != "" {
		return reasoning
	}
	_, reasoning := splitInlineReasoning(contentToText(msg["content"]))
	return reasoning
}

func splitInlineReasoning(text string) (content string, reasoning string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", ""
	}
	matches := thinkTagRE.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return text, ""
	}
	parts := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		if item := strings.TrimSpace(match[1]); item != "" {
			parts = append(parts, item)
		}
	}
	cleaned := strings.TrimSpace(thinkTagRE.ReplaceAllString(text, ""))
	return cleaned, strings.Join(parts, "\n\n")
}

func stateValueMap(state map[string]any) map[string]any {
	if state == nil {
		return nil
	}
	values, _ := state["values"].(map[string]any)
	if len(values) > 0 {
		return values
	}
	return state
}

func dedupeSourceKey(source TraceSource) string {
	if source.URL != "" {
		return source.URL
	}
	if source.Title != "" {
		return source.ToolCallID + ":" + source.Title
	}
	return ""
}

func cloneMap(source map[string]any) map[string]any {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func previewText(text string, limit int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.Join(strings.Fields(text), " ")
	if limit <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return strings.TrimSpace(string(runes[:limit-1])) + "…"
}

func extractMarkdownTitle(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#") {
			continue
		}
		title := strings.TrimSpace(strings.TrimLeft(line, "#"))
		if title != "" && !strings.EqualFold(title, "Untitled") {
			return title
		}
	}
	return ""
}

func normalizeDomain(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	host = strings.TrimPrefix(host, "www.")
	return host
}

func domainMatches(host, domain string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	domain = strings.ToLower(strings.TrimSpace(domain))
	if host == "" || domain == "" {
		return false
	}
	return host == domain || strings.HasSuffix(host, "."+domain)
}
