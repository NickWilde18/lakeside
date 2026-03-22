package domainassistant

import (
	"context"
	"strings"

	"github.com/cloudwego/eino/adk"
	componenttool "github.com/cloudwego/eino/components/tool"

	"lakeside/internal/service/moduleapi"
)

// LeafBinding 描述一个可供领域模块调度的叶子能力。
type LeafBinding struct {
	Key           string
	Description   string
	Kind          string
	Interruptible bool
	Tool          componenttool.InvokableTool
	Agent         adk.Agent
}

// New 创建领域模块。
//
// 领域模块不再保留 supervisor fallback，统一走：
// 1. assessment 决定 ready / need_clarify / reject；
// 2. planner 产出结构化叶子执行计划；
// 3. workflow 按计划顺序执行叶子能力。
func New(ctx context.Context, key, description, instruction string, _ int, leaves []LeafBinding) (moduleapi.Module, error) {
	items := make(map[string]LeafBinding, len(leaves))
	ordered := make([]LeafBinding, 0, len(leaves))
	for _, leaf := range leaves {
		leaf.Key = strings.TrimSpace(leaf.Key)
		if leaf.Key == "" || (leaf.Agent == nil && leaf.Tool == nil) {
			continue
		}
		items[leaf.Key] = leaf
		ordered = append(ordered, leaf)
	}
	if len(items) == 0 {
		return nil, nil
	}
	return &plannedAgent{
		key:         strings.TrimSpace(key),
		description: strings.TrimSpace(description),
		instruction: strings.TrimSpace(instruction),
		leaves:      items,
		planner:     newPlanner(ctx, key, description, instruction, ordered),
	}, nil
}
