package agentplatform

import "lakeside/internal/service/deerflowclient"

type DeerFlowTrace struct {
	ThreadID  string                  `json:"thread_id,omitempty"`
	RunID     string                  `json:"run_id,omitempty"`
	RunStatus string                  `json:"run_status,omitempty"`
	Title     string                  `json:"title,omitempty"`
	StateTail string                  `json:"state_tail,omitempty"`
	Messages  []DeerFlowTraceMessage  `json:"messages,omitempty"`
	Todos     []DeerFlowTraceTodo     `json:"todos,omitempty"`
	Artifacts []string                `json:"artifacts,omitempty"`
	Sources   []DeerFlowTraceSource   `json:"sources,omitempty"`
}

type DeerFlowTraceMessage struct {
	ID         string                  `json:"id,omitempty"`
	Type       string                  `json:"type,omitempty"`
	Name       string                  `json:"name,omitempty"`
	Content    string                  `json:"content,omitempty"`
	Reasoning  string                  `json:"reasoning,omitempty"`
	ToolCallID string                  `json:"tool_call_id,omitempty"`
	ToolCalls  []DeerFlowTraceToolCall `json:"tool_calls,omitempty"`
	Status     string                  `json:"status,omitempty"`
}

type DeerFlowTraceToolCall struct {
	ID      string                 `json:"id,omitempty"`
	Name    string                 `json:"name,omitempty"`
	Args    map[string]any         `json:"args,omitempty"`
	Result  any                    `json:"result,omitempty"`
	Status  string                 `json:"status,omitempty"`
	Error   string                 `json:"error,omitempty"`
	Sources []DeerFlowTraceSource  `json:"sources,omitempty"`
}

type DeerFlowTraceTodo struct {
	Content string `json:"content,omitempty"`
	Status  string `json:"status,omitempty"`
}

type DeerFlowTraceSource struct {
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

func deerflowTraceFromClient(trace *deerflowclient.Trace) *DeerFlowTrace {
	if trace == nil {
		return nil
	}
	result := &DeerFlowTrace{
		ThreadID:  trace.ThreadID,
		RunID:     trace.RunID,
		RunStatus: trace.RunStatus,
		Title:     trace.Title,
		StateTail: trace.StateTail,
		Artifacts: append([]string(nil), trace.Artifacts...),
		Messages:  make([]DeerFlowTraceMessage, 0, len(trace.Messages)),
		Todos:     make([]DeerFlowTraceTodo, 0, len(trace.Todos)),
		Sources:   make([]DeerFlowTraceSource, 0, len(trace.Sources)),
	}
	for _, item := range trace.Messages {
		toolCalls := make([]DeerFlowTraceToolCall, 0, len(item.ToolCalls))
		for _, call := range item.ToolCalls {
			toolCalls = append(toolCalls, DeerFlowTraceToolCall{
				ID:      call.ID,
				Name:    call.Name,
				Args:    cloneAnyMap(call.Args),
				Result:  call.Result,
				Status:  call.Status,
				Error:   call.Error,
				Sources: deerflowSourcesFromClient(call.Sources),
			})
		}
		result.Messages = append(result.Messages, DeerFlowTraceMessage{
			ID:         item.ID,
			Type:       item.Type,
			Name:       item.Name,
			Content:    item.Content,
			Reasoning:  item.Reasoning,
			ToolCallID: item.ToolCallID,
			ToolCalls:  toolCalls,
			Status:     item.Status,
		})
	}
	for _, item := range trace.Todos {
		result.Todos = append(result.Todos, DeerFlowTraceTodo{
			Content: item.Content,
			Status:  item.Status,
		})
	}
	result.Sources = deerflowSourcesFromClient(trace.Sources)
	return result
}

func deerflowSourcesFromClient(items []deerflowclient.TraceSource) []DeerFlowTraceSource {
	if len(items) == 0 {
		return nil
	}
	result := make([]DeerFlowTraceSource, 0, len(items))
	for _, item := range items {
		result = append(result, DeerFlowTraceSource{
			Title:         item.Title,
			URL:           item.URL,
			Domain:        item.Domain,
			Snippet:       item.Snippet,
			Query:         item.Query,
			ToolName:      item.ToolName,
			ToolCallID:    item.ToolCallID,
			SourceType:    item.SourceType,
			Quality:       item.Quality,
			LowConfidence: item.LowConfidence,
		})
	}
	return result
}

func cloneAnyMap(source map[string]any) map[string]any {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}
