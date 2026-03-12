package agentplatform

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/gogf/gf/v2/frame/g"

	"lakeside/internal/infra/ragclient"
	"lakeside/internal/service/chatmodels"
	"lakeside/internal/service/domainassistant"
	leafitsm "lakeside/internal/service/leafagent/itsm"
	leafknowledge "lakeside/internal/service/leafagent/knowledge"
	"lakeside/internal/service/rootassistant"
)

type runtimeRegistry struct {
	bundles map[string]*runnerBundle
	infos   map[string]nodeInfo
	paths   nodePathIndex
	names   []string
}

func newRuntimeRegistry(ctx context.Context, cfg *config) (*runtimeRegistry, error) {
	if cfg == nil {
		return nil, fmt.Errorf("agentplatform config is nil")
	}
	infos := make(map[string]nodeInfo)
	leafAgents := make(map[string]adk.Agent)
	leafBindings := make(map[string]domainassistant.LeafBinding)

	rag := ragclient.NewClient(ragclient.Config{
		BaseURL: cfg.RAG.BaseURL,
		Timeout: time.Duration(cfg.RAG.TimeoutMs) * time.Millisecond,
	})
	chatModel := chatmodels.GetChatModel(ctx)

	for _, leaf := range cfg.Leaves {
		switch leaf.Type {
		case "itsm":
			agent := leafitsm.New(ctx, leaf.Key, leaf.Description)
			leafAgents[leaf.Key] = agent
			leafBindings[leaf.Key] = domainassistant.LeafBinding{
				Key:           leaf.Key,
				Description:   agent.Description(ctx),
				Kind:          leaf.Type,
				Interruptible: true,
				Agent:         agent,
			}
			infos[leaf.Key] = nodeInfo{Key: leaf.Key, Description: agent.Description(ctx), Kind: leaf.Type}
		case "knowledge":
			agent := leafknowledge.New(ctx, leafknowledge.Config{
				Key:            leaf.Key,
				Description:    leaf.Description,
				KBIDs:          leaf.KBIDs,
				TopK:           leaf.TopK,
				RewriteQueries: leaf.RewriteQueries,
				MaxContextDocs: leaf.MaxContextDocs,
				SourceLimit:    leaf.SourceLimit,
			}, rag, chatModel)
			leafAgents[leaf.Key] = agent
			leafBindings[leaf.Key] = domainassistant.LeafBinding{
				Key:           leaf.Key,
				Description:   leaf.Description,
				Kind:          leaf.Type,
				Interruptible: false,
				Agent:         agent,
			}
			infos[leaf.Key] = nodeInfo{Key: leaf.Key, Description: leaf.Description, Kind: leaf.Type}
		default:
			return nil, fmt.Errorf("unsupported leaf type: %s", leaf.Type)
		}
	}

	domainAgents := make(map[string]adk.Agent)
	for _, domain := range cfg.Domains {
		children := make([]adk.Agent, 0, len(domain.Children))
		orderedLeaves := make([]domainassistant.LeafBinding, 0, len(domain.Children))
		for _, childKey := range domain.Children {
			agent, ok := leafAgents[childKey]
			if !ok {
				return nil, fmt.Errorf("build domain %s failed: unknown child key %s", domain.Key, childKey)
			}
			binding, ok := leafBindings[childKey]
			if !ok {
				return nil, fmt.Errorf("build domain %s failed: missing leaf binding %s", domain.Key, childKey)
			}
			children = append(children, agent)
			orderedLeaves = append(orderedLeaves, binding)
		}
		instruction := renderInstruction(domain.InstructionTemplate, domain.Key, childCatalog(domain.Children, infos))
		agent, err := domainassistant.New(ctx, domain.Key, domain.Description, instruction, domain.MaxIterations, children, orderedLeaves)
		if err != nil {
			return nil, err
		}
		domainAgents[domain.Key] = agent
		infos[domain.Key] = nodeInfo{Key: domain.Key, Description: domain.Description, Kind: "domain"}
	}

	bundles := make(map[string]*runnerBundle)
	for _, root := range cfg.Roots {
		children, err := collectChildren(root.Children, leafAgents, domainAgents)
		if err != nil {
			return nil, fmt.Errorf("build root %s failed: %w", root.Key, err)
		}
		instruction := renderInstruction(root.InstructionTemplate, root.Key, childCatalog(root.Children, infos))
		agent, err := rootassistant.New(ctx, root.Key, root.Description, instruction, root.MaxIterations, children)
		if err != nil {
			return nil, err
		}
		store := newCheckpointStore(ctx, root.Key, checkpointTTL(cfg))
		bundles[root.Key] = &runnerBundle{
			RootKey: root.Key,
			Runner: adk.NewRunner(ctx, adk.RunnerConfig{
				Agent:           agent,
				EnableStreaming: false,
				CheckPointStore: store,
			}),
			CheckpointStore: store,
		}
		infos[root.Key] = nodeInfo{Key: root.Key, Description: root.Description, Kind: "root"}
	}

	names := make([]string, 0, len(infos))
	for key := range infos {
		names = append(names, key)
	}
	paths, err := buildNodePaths(cfg)
	if err != nil {
		return nil, err
	}
	g.Log().Infof(ctx, "agentplatform registry initialized, root_count=%d domain_count=%d leaf_count=%d", len(cfg.Roots), len(cfg.Domains), len(cfg.Leaves))
	return &runtimeRegistry{bundles: bundles, infos: infos, paths: paths, names: names}, nil
}

func collectChildren(keys []string, leaves map[string]adk.Agent, domains map[string]adk.Agent) ([]adk.Agent, error) {
	children := make([]adk.Agent, 0, len(keys))
	for _, key := range keys {
		if agent, ok := leaves[key]; ok {
			children = append(children, agent)
			continue
		}
		if domains != nil {
			if agent, ok := domains[key]; ok {
				children = append(children, agent)
				continue
			}
		}
		return nil, fmt.Errorf("unknown child key: %s", key)
	}
	return children, nil
}

func childCatalog(childKeys []string, infos map[string]nodeInfo) string {
	parts := make([]string, 0, len(childKeys))
	for _, key := range childKeys {
		info, ok := infos[key]
		if !ok {
			continue
		}
		parts = append(parts, fmt.Sprintf("- %s：%s", info.Key, info.Description))
	}
	return strings.Join(parts, "\n")
}

func renderInstruction(template, key, childCatalog string) string {
	replacer := strings.NewReplacer(
		"{agent_key}", key,
		"{child_catalog}", childCatalog,
	)
	return strings.TrimSpace(replacer.Replace(strings.TrimSpace(template)))
}

func buildNodePaths(cfg *config) (nodePathIndex, error) {
	paths := make(nodePathIndex)
	if cfg == nil {
		return paths, nil
	}

	domainByKey := make(map[string]supervisorNodeConfig, len(cfg.Domains))
	for _, domain := range cfg.Domains {
		domainByKey[domain.Key] = domain
	}
	leafKeys := make(map[string]struct{}, len(cfg.Leaves))
	for _, leaf := range cfg.Leaves {
		leafKeys[leaf.Key] = struct{}{}
	}

	for _, root := range cfg.Roots {
		if _, ok := paths[root.Key]; !ok {
			paths[root.Key] = make(map[string][]string)
		}
		paths[root.Key][root.Key] = []string{root.Key}
		for _, child := range root.Children {
			if _, ok := leafKeys[child]; ok {
				paths[root.Key][child] = []string{root.Key, child}
				continue
			}
			domain, ok := domainByKey[child]
			if !ok {
				return nil, fmt.Errorf("build node paths failed: unknown root child %s under %s", child, root.Key)
			}
			paths[root.Key][domain.Key] = []string{root.Key, domain.Key}
			for _, grandChild := range domain.Children {
				if _, ok := leafKeys[grandChild]; !ok {
					return nil, fmt.Errorf("build node paths failed: unknown domain child %s under %s", grandChild, domain.Key)
				}
				paths[root.Key][grandChild] = []string{root.Key, domain.Key, grandChild}
			}
		}
	}
	return paths, nil
}

func (r *runtimeRegistry) pathForNode(rootKey, nodeKey string) []string {
	if r == nil || strings.TrimSpace(rootKey) == "" || strings.TrimSpace(nodeKey) == "" {
		return nil
	}
	items := r.paths[strings.TrimSpace(rootKey)]
	if len(items) == 0 {
		return nil
	}
	path := items[strings.TrimSpace(nodeKey)]
	if len(path) == 0 {
		return nil
	}
	return append([]string(nil), path...)
}
