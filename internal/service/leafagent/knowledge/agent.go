package knowledge

import (
	"context"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"

	"lakeside/internal/infra/ragclient"
	legacy "lakeside/internal/service/knowledgeagent"
)

type Config struct {
	Key            string
	Description    string
	KBIDs          []string
	TopK           int
	RewriteQueries int
	MaxContextDocs int
	SourceLimit    int
}

func New(_ context.Context, cfg Config, client *ragclient.Client, chatModel model.ToolCallingChatModel) adk.Agent {
	return legacy.NewKnowledgeAgent(legacy.Config{
		Name:           cfg.Key,
		Description:    cfg.Description,
		KBIDs:          append([]string(nil), cfg.KBIDs...),
		TopK:           cfg.TopK,
		RewriteQueries: cfg.RewriteQueries,
		MaxContextDocs: cfg.MaxContextDocs,
		SourceLimit:    cfg.SourceLimit,
	}, client, chatModel)
}
