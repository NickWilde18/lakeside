package knowledgeagent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"

	"lakeside/internal/infra/ragclient"
	"lakeside/internal/service/chatmodels"
)

// NewRegistry 按配置创建全部 knowledge subagent。
func NewRegistry(ctx context.Context) (*Registry, error) {
	cfgs, err := loadConfigs(ctx)
	if err != nil {
		return nil, err
	}
	client := ragclient.NewClient(ragclient.Config{
		BaseURL: g.Cfg().MustGet(ctx, "knowledge.rag.baseURL").String(),
		Timeout: time.Duration(g.Cfg().MustGet(ctx, "knowledge.rag.timeoutMs", 10000).Int()) * time.Millisecond,
	})
	model := chatmodels.GetChatModel(ctx)

	agents := make([]adk.Agent, 0, len(cfgs))
	infos := make([]AgentInfo, 0, len(cfgs))
	for _, cfg := range cfgs {
		agents = append(agents, NewKnowledgeAgent(cfg, client, model))
		infos = append(infos, AgentInfo{Name: cfg.Name, Description: cfg.Description})
	}
	return &Registry{agents: agents, infos: infos}, nil
}

func (r *Registry) Agents() []adk.Agent {
	if r == nil || len(r.agents) == 0 {
		return nil
	}
	out := make([]adk.Agent, 0, len(r.agents))
	out = append(out, r.agents...)
	return out
}

func (r *Registry) Infos() []AgentInfo {
	if r == nil || len(r.infos) == 0 {
		return nil
	}
	out := make([]AgentInfo, 0, len(r.infos))
	out = append(out, r.infos...)
	return out
}

func (r *Registry) Names() []string {
	if r == nil || len(r.infos) == 0 {
		return nil
	}
	out := make([]string, 0, len(r.infos))
	for _, info := range r.infos {
		out = append(out, info.Name)
	}
	return out
}

func loadConfigs(ctx context.Context) ([]Config, error) {
	var cfgs []Config
	if err := gconv.Structs(g.Cfg().MustGet(ctx, "knowledge.agents").Interfaces(), &cfgs); err != nil {
		return nil, fmt.Errorf("parse knowledge.agents failed: %w", err)
	}
	defaultTopK := g.Cfg().MustGet(ctx, "knowledge.rag.defaultTopK", 5).Int()
	for i := range cfgs {
		cfgs[i].Name = strings.TrimSpace(cfgs[i].Name)
		cfgs[i].Description = strings.TrimSpace(cfgs[i].Description)
		cfgs[i].KBIDs = trimStrings(cfgs[i].KBIDs)
		if cfgs[i].TopK <= 0 {
			cfgs[i].TopK = defaultTopK
		}
		if cfgs[i].TopK <= 0 {
			cfgs[i].TopK = 5
		}
		if cfgs[i].MaxContextDocs <= 0 {
			cfgs[i].MaxContextDocs = minInt(cfgs[i].TopK, 4)
		}
		if cfgs[i].SourceLimit <= 0 {
			cfgs[i].SourceLimit = minInt(cfgs[i].TopK, 4)
		}
	}
	return cfgs, nil
}

func trimStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
