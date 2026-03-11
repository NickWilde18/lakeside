package agentplatform

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"

	itsmv1 "lakeside/api/itsm/v1"
	"lakeside/internal/service/agentplatform/eventctx"
	legacyitsm "lakeside/internal/service/itsmagent"
	legacyknowledge "lakeside/internal/service/knowledgeagent"
)

func (s *Service) consumeIterator(ctx context.Context, assistantKey string, iter *adk.AsyncIterator[*adk.AgentEvent], sessionID, checkpointID, language string) *Response {
	resp := &Response{
		AssistantKey: assistantKey,
		Status:       "done",
		SessionID:    sessionID,
		ActivePath:   []string{assistantKey},
	}
	var (
		lastMessage       string
		lastPath          []string
		rootFinalMessage  string
		aggregatedSources []Source
	)

	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		path := runPathStrings(event.RunPath)
		if len(path) > 0 {
			lastPath = preferLongerPath(lastPath, path)
		}
		if event.Err != nil {
			g.Log().Errorf(ctx, "agentplatform event failed, assistant_key=%s session_id=%s checkpoint_id=%s err=%v", assistantKey, sessionID, checkpointID, event.Err)
			return s.errorResponse(assistantKey, sessionID, pathOrDefault(path, assistantKey), event.Err.Error())
		}
		if event.Output != nil && event.Output.CustomizedOutput != nil {
			if result := legacyknowledge.ResultFromAny(event.Output.CustomizedOutput); result != nil {
				stepPath := s.resolveNodePath(assistantKey, path, result.AgentName, strings.TrimSpace(event.AgentName))
				step := StepResult{
					Path:    stepPath,
					Kind:    stepKindKnowledge,
					Message: result.Message,
					Sources: resultFromKnowledge(result).Sources,
				}
				resp.Steps = append(resp.Steps, step)
				aggregatedSources = mergeSources(aggregatedSources, step.Sources)
				resp.ActivePath = step.Path
				lastPath = preferLongerPath(lastPath, stepPath)
				eventctx.Emit(ctx, eventTypeKnowledgeAnswerReady, step.Path, step.Message, g.Map{
					"agent_name": result.AgentName,
					"sources":    step.Sources,
				})
			}
			if (pathContains(path, "itsm") || strings.TrimSpace(event.AgentName) == "itsm") && legacyitsm.ExecutionResultFromAny(event.Output.CustomizedOutput) != nil {
				result := legacyitsm.ExecutionResultFromAny(event.Output.CustomizedOutput)
				stepPath := s.resolveNodePath(assistantKey, path, "itsm", strings.TrimSpace(event.AgentName))
				step := StepResult{
					Path:    stepPath,
					Kind:    stepKindITSMDone,
					Message: result.Message,
				}
				resp.Steps = append(resp.Steps, step)
				resp.Result = &Result{
					Success:  result.Success,
					TicketNo: result.TicketNo,
					Message:  result.Message,
					Code:     result.Code,
					Sources:  aggregatedSources,
				}
				resp.ActivePath = step.Path
				lastPath = preferLongerPath(lastPath, stepPath)
				eventctx.Emit(ctx, eventTypeITSMDone, step.Path, step.Message, g.Map{
					"ticket_no": result.TicketNo,
					"code":      result.Code,
					"success":   result.Success,
				})
			}
		}
		if event.Output != nil {
			msg, _, err := adk.GetMessage(event)
			if err == nil && msg != nil && strings.TrimSpace(msg.Content) != "" {
				content := strings.TrimSpace(msg.Content)
				if isInternalTransferMessage(content) {
					continue
				}
				lastMessage = content
				if strings.TrimSpace(event.AgentName) == assistantKey {
					rootFinalMessage = content
				}
			}
		}
		if event.Action != nil && event.Action.Interrupted != nil {
			interruptPath := s.resolveNodePath(assistantKey, preferLongerPath(path, lastPath), "itsm", strings.TrimSpace(event.AgentName))
			interrupts, status := legacyitsm.APIInterruptsFromContexts(event.Action.Interrupted.InterruptContexts)
			step := StepResult{
				Path:       interruptPath,
				Kind:       stepKindITSMInterrupt,
				Interrupts: interrupts,
			}
			if len(interrupts) > 0 {
				step.Message = interrupts[0].Prompt
			}
			resp.Steps = append(resp.Steps, step)
			resp.Status = status
			resp.CheckpointID = checkpointID
			resp.ActivePath = step.Path
			lastPath = preferLongerPath(lastPath, step.Path)
			resp.Interrupts = interrupts
			eventctx.Emit(ctx, eventTypeITSMInterruptEmitted, step.Path, step.Message, g.Map{
				"status":     status,
				"interrupts": interrupts,
				"checkpoint": checkpointID,
				"assistant":  assistantKey,
				"session_id": sessionID,
			})
			if resp.Result == nil && (rootFinalMessage != "" || len(aggregatedSources) > 0) {
				resp.Result = &Result{
					Success: false,
					Message: chooseMessage(firstStepMessage(resp.Steps), chooseMessage(rootFinalMessage, lastMessage)),
					Sources: aggregatedSources,
				}
			}
			return resp
		}
	}

	resp.Status = "done"
	resp.CheckpointID = ""
	if len(resp.Steps) == 0 || len(lastPath) > len(resp.ActivePath) {
		resp.ActivePath = pathOrDefault(lastPath, assistantKey)
	}
	if resp.Result == nil {
		finalMessage := chooseMessage(rootFinalMessage, chooseMessage(firstStepMessage(resp.Steps), chooseMessage(lastMessage, localizeText(language, "操作完成。", "Done."))))
		if len(aggregatedSources) == 0 {
			finalMessage = chooseMessage(firstStepMessage(resp.Steps), finalMessage)
		}
		resp.Result = &Result{
			Success: true,
			Message: finalMessage,
			Sources: aggregatedSources,
		}
	} else {
		resp.Result.Sources = mergeSources(resp.Result.Sources, aggregatedSources)
		if strings.TrimSpace(rootFinalMessage) != "" {
			resp.Result.Message = rootFinalMessage
		} else if strings.TrimSpace(resp.Result.Message) == "" {
			resp.Result.Message = chooseMessage(lastMessage, localizeText(language, "操作完成。", "Done."))
		}
	}
	s.invalidateCheckpoint(ctx, assistantKey, checkpointID)
	return resp
}

func resultFromKnowledge(result *legacyknowledge.Result) *Result {
	if result == nil {
		return nil
	}
	sources := make([]Source, 0, len(result.Sources))
	for _, item := range result.Sources {
		sources = append(sources, Source{
			KBID:     item.KBID,
			DocID:    item.DocID,
			NodeID:   item.NodeID,
			Filename: item.Filename,
			Snippet:  item.Snippet,
			Score:    item.Score,
		})
	}
	return &Result{Success: result.Success, Message: result.Message, Sources: sources}
}

func mergeSources(base, incoming []Source) []Source {
	if len(incoming) == 0 {
		return base
	}
	seen := make(map[string]struct{}, len(base)+len(incoming))
	out := make([]Source, 0, len(base)+len(incoming))
	for _, item := range base {
		key := item.KBID + ":" + item.NodeID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	for _, item := range incoming {
		key := item.KBID + ":" + item.NodeID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func runPathStrings(path []adk.RunStep) []string {
	if len(path) == 0 {
		return nil
	}
	out := make([]string, 0, len(path))
	for i := range path {
		name := strings.TrimSpace((&path[i]).String())
		if name == "" {
			continue
		}
		if len(out) > 0 && out[len(out)-1] == name {
			continue
		}
		out = append(out, name)
	}
	return out
}

func pathContains(path []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, item := range path {
		if strings.TrimSpace(item) == target {
			return true
		}
	}
	return false
}

func pathOrDefault(path []string, assistantKey string) []string {
	if len(path) > 0 {
		return append([]string(nil), path...)
	}
	if strings.TrimSpace(assistantKey) == "" {
		return nil
	}
	return []string{strings.TrimSpace(assistantKey)}
}

func inferITSMPath(currentPath, lastPath []string, assistantKey string) []string {
	if pathContains(currentPath, "itsm") {
		return append([]string(nil), currentPath...)
	}
	if len(lastPath) >= 2 {
		out := append([]string(nil), lastPath[:len(lastPath)-1]...)
		out = append(out, "itsm")
		return out
	}
	if len(lastPath) == 1 {
		return []string{lastPath[0], "itsm"}
	}
	if strings.TrimSpace(assistantKey) == "" {
		return currentPath
	}
	return []string{strings.TrimSpace(assistantKey), "itsm"}
}

func preferLongerPath(current, candidate []string) []string {
	if len(candidate) > len(current) {
		return append([]string(nil), candidate...)
	}
	if len(current) == 0 && len(candidate) > 0 {
		return append([]string(nil), candidate...)
	}
	return append([]string(nil), current...)
}

func (s *Service) resolveNodePath(assistantKey string, currentPath []string, nodeKeys ...string) []string {
	best := pathOrDefault(currentPath, assistantKey)
	if s == nil || s.registry == nil {
		return best
	}
	for _, nodeKey := range nodeKeys {
		candidate := s.registry.pathForNode(assistantKey, nodeKey)
		if len(candidate) > len(best) || (len(candidate) == len(best) && !pathMatchesAnyNode(best, nodeKeys)) {
			best = candidate
		}
	}
	if containsNodeKey(nodeKeys, "itsm") && !pathContains(best, "itsm") {
		if inferred := inferITSMPath(best, best, assistantKey); len(inferred) > len(best) {
			best = inferred
		}
	}
	return pathOrDefault(best, assistantKey)
}

func containsNodeKey(nodeKeys []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, nodeKey := range nodeKeys {
		if strings.TrimSpace(nodeKey) == target {
			return true
		}
	}
	return false
}

func pathMatchesAnyNode(path []string, nodeKeys []string) bool {
	if len(path) == 0 {
		return false
	}
	return containsNodeKey(nodeKeys, path[len(path)-1])
}

func responseToVisibleMessage(resp *Response) (string, string) {
	if resp == nil {
		return "", "{}"
	}
	if len(resp.Interrupts) > 0 {
		return resp.Interrupts[0].Prompt, toJSONString(resp)
	}
	if resp.Result != nil {
		return resp.Result.Message, toJSONString(resp)
	}
	return resp.Status, toJSONString(resp)
}

func sessionStatusFromResponse(resp *Response) string {
	if resp == nil || resp.Status == "done" {
		return statusDone
	}
	return statusActive
}

func genCheckpointID() string {
	return "ckpt-" + uuid.NewString()
}

func toJSONString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func chooseMessage(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}

func localizeText(language, zh, en string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "zh") {
		return zh
	}
	return en
}

func summarizeTargets(targets map[string]*itsmv1.ResumeTarget, language string) string {
	parts := make([]string, 0, len(targets))
	for _, target := range targets {
		if target == nil {
			continue
		}
		if strings.TrimSpace(target.Answer) != "" {
			parts = append(parts, target.Answer)
			continue
		}
		if target.Confirmed != nil {
			if *target.Confirmed {
				parts = append(parts, localizeText(language, "用户确认继续提交", "User confirmed ticket submission"))
			} else {
				parts = append(parts, localizeText(language, "用户取消提交", "User canceled ticket submission"))
			}
		}
		if strings.TrimSpace(target.Subject) != "" {
			parts = append(parts, target.Subject)
		}
		if strings.TrimSpace(target.OthersDesc) != "" {
			parts = append(parts, target.OthersDesc)
		}
	}
	return strings.Join(parts, " | ")
}

func detectLanguage(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "zh"
	}
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FFF {
			return "zh"
		}
	}
	return "en"
}

func chooseLanguage(current, candidate string) string {
	if strings.TrimSpace(candidate) != "" {
		return candidate
	}
	if strings.TrimSpace(current) != "" {
		return current
	}
	return "zh"
}

func targetsLanguage(targets map[string]*itsmv1.ResumeTarget) string {
	for _, target := range targets {
		if target == nil {
			continue
		}
		if strings.TrimSpace(target.Answer) != "" {
			return detectLanguage(target.Answer)
		}
		if strings.TrimSpace(target.Subject) != "" {
			return detectLanguage(target.Subject)
		}
		if strings.TrimSpace(target.OthersDesc) != "" {
			return detectLanguage(target.OthersDesc)
		}
	}
	return ""
}

func withoutCancel(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return context.WithoutCancel(ctx)
}

func marshalPath(path []string) string {
	if len(path) == 0 {
		return "[]"
	}
	b, err := json.Marshal(path)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func unmarshalPath(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	out, err := decodeJSON[[]string](raw)
	if err != nil {
		return nil
	}
	return out
}

func firstStepMessage(steps []StepResult) string {
	for _, step := range steps {
		if strings.TrimSpace(step.Message) != "" {
			return step.Message
		}
	}
	return ""
}

func formatError(err error) string {
	if err == nil {
		return "internal error"
	}
	return fmt.Sprintf("%v", err)
}

func isInternalTransferMessage(content string) bool {
	text := strings.ToLower(strings.TrimSpace(content))
	return strings.HasPrefix(text, "successfully transferred to agent [")
}
