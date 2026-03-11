package agentplatform

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

func loadConfig(ctx context.Context) (*config, error) {
	cfg := &config{}
	cfg.Storage.Provider = strings.ToLower(strings.TrimSpace(g.Cfg().MustGet(ctx, "agents.storage.provider", "sqlite").String()))
	cfg.Storage.SQLitePath = strings.TrimSpace(g.Cfg().MustGet(ctx, "agents.storage.sqlitePath").String())
	cfg.Storage.MSSQL.Host = strings.TrimSpace(g.Cfg().MustGet(ctx, "agents.storage.mssql.host").String())
	cfg.Storage.MSSQL.Port = g.Cfg().MustGet(ctx, "agents.storage.mssql.port", 1433).Int()
	cfg.Storage.MSSQL.User = strings.TrimSpace(g.Cfg().MustGet(ctx, "agents.storage.mssql.user").String())
	cfg.Storage.MSSQL.Pass = g.Cfg().MustGet(ctx, "agents.storage.mssql.password").String()
	cfg.Storage.MSSQL.DBName = strings.TrimSpace(g.Cfg().MustGet(ctx, "agents.storage.mssql.database").String())
	cfg.Checkpoint.TTLHours = g.Cfg().MustGet(ctx, "agents.checkpoint.ttlHours", 24).Int()
	cfg.Summarization.ContextTokens = g.Cfg().MustGet(ctx, "agents.summarization.contextTokens", 12000).Int()
	cfg.Summarization.PreserveUserMessageTokens = g.Cfg().MustGet(ctx, "agents.summarization.preserveUserMessageTokens", 2000).Int()
	cfg.Memory.MaxItems = g.Cfg().MustGet(ctx, "agents.memory.maxItems", 8).Int()
	cfg.Memory.Workers = g.Cfg().MustGet(ctx, "agents.memory.workers", 1).Int()
	cfg.Memory.QueueSize = g.Cfg().MustGet(ctx, "agents.memory.queueSize", 32).Int()
	cfg.RAG.BaseURL = strings.TrimSpace(g.Cfg().MustGet(ctx, "agents.rag.baseURL").String())
	cfg.RAG.TimeoutMs = g.Cfg().MustGet(ctx, "agents.rag.timeoutMs", 10000).Int()
	cfg.RAG.DefaultTopK = g.Cfg().MustGet(ctx, "agents.rag.defaultTopK", 5).Int()
	cfg.Runtime.WorkerCount = g.Cfg().MustGet(ctx, "agents.runtime.workerCount", 1).Int()
	cfg.Runtime.StreamKey = strings.TrimSpace(g.Cfg().MustGet(ctx, "agents.runtime.streamKey", "lakeside:agent:runs:v1").String())
	cfg.Runtime.StreamGroup = strings.TrimSpace(g.Cfg().MustGet(ctx, "agents.runtime.streamGroup", "lakeside-agent-workers").String())
	cfg.Runtime.ConsumerPrefix = strings.TrimSpace(g.Cfg().MustGet(ctx, "agents.runtime.consumerPrefix", "agent-worker").String())
	cfg.Runtime.ReadBlockMs = g.Cfg().MustGet(ctx, "agents.runtime.readBlockMs", 5000).Int()
	cfg.Runtime.PubSubChannel = strings.TrimSpace(g.Cfg().MustGet(ctx, "agents.runtime.pubsubChannel", "lakeside:agent:events:v1").String())
	if err := gconv.Structs(g.Cfg().MustGet(ctx, "agents.roots").Interfaces(), &cfg.Roots); err != nil {
		return nil, fmt.Errorf("parse agents.roots failed: %w", err)
	}
	if err := gconv.Structs(g.Cfg().MustGet(ctx, "agents.domains").Interfaces(), &cfg.Domains); err != nil {
		return nil, fmt.Errorf("parse agents.domains failed: %w", err)
	}
	if err := gconv.Structs(g.Cfg().MustGet(ctx, "agents.leaves").Interfaces(), &cfg.Leaves); err != nil {
		return nil, fmt.Errorf("parse agents.leaves failed: %w", err)
	}
	for i := range cfg.Leaves {
		cfg.Leaves[i].Key = strings.TrimSpace(cfg.Leaves[i].Key)
		cfg.Leaves[i].Type = strings.ToLower(strings.TrimSpace(cfg.Leaves[i].Type))
		cfg.Leaves[i].Description = strings.TrimSpace(cfg.Leaves[i].Description)
		cfg.Leaves[i].KBIDs = trimStrings(cfg.Leaves[i].KBIDs)
		if cfg.Leaves[i].TopK <= 0 {
			cfg.Leaves[i].TopK = cfg.RAG.DefaultTopK
		}
		if cfg.Leaves[i].TopK <= 0 {
			cfg.Leaves[i].TopK = 5
		}
		if cfg.Leaves[i].RewriteQueries <= 0 {
			cfg.Leaves[i].RewriteQueries = 3
		}
		if cfg.Leaves[i].MaxContextDocs <= 0 {
			cfg.Leaves[i].MaxContextDocs = minInt(cfg.Leaves[i].TopK, 4)
		}
		if cfg.Leaves[i].SourceLimit <= 0 {
			cfg.Leaves[i].SourceLimit = minInt(cfg.Leaves[i].TopK, 4)
		}
	}
	for i := range cfg.Roots {
		normalizeSupervisorConfig(&cfg.Roots[i])
	}
	for i := range cfg.Domains {
		normalizeSupervisorConfig(&cfg.Domains[i])
	}
	return cfg, nil
}

func normalizeSupervisorConfig(cfg *supervisorNodeConfig) {
	if cfg == nil {
		return
	}
	cfg.Key = strings.TrimSpace(cfg.Key)
	cfg.Description = strings.TrimSpace(cfg.Description)
	cfg.InstructionTemplate = strings.TrimSpace(cfg.InstructionTemplate)
	cfg.Children = trimStrings(cfg.Children)
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = 6
	}
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

func checkpointTTL(cfg *config) time.Duration {
	if cfg == nil || cfg.Checkpoint.TTLHours <= 0 {
		return 24 * time.Hour
	}
	return time.Duration(cfg.Checkpoint.TTLHours) * time.Hour
}
