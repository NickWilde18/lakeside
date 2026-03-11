package itsm

import (
	"context"

	"github.com/cloudwego/eino/adk"

	legacy "lakeside/internal/service/itsmagent"
)

// Agent 是给多层 agent 树使用的 ITSM 叶子代理包装器。
// 它复用现有 itsmagent 的实现，只把对外 Name/Description 改成树配置里的 key/description。
type Agent struct {
	key         string
	description string
	inner       adk.Agent
}

func New(ctx context.Context, key, description string) adk.Agent {
	return &Agent{
		key:         key,
		description: description,
		inner:       legacy.GetAgent(ctx),
	}
}

func (a *Agent) Name(_ context.Context) string {
	return a.key
}

func (a *Agent) Description(ctx context.Context) string {
	if a.description != "" {
		return a.description
	}
	if a.inner == nil {
		return ""
	}
	return a.inner.Description(ctx)
}

func (a *Agent) GetType() string {
	if typed, ok := a.inner.(interface{ GetType() string }); ok {
		return typed.GetType()
	}
	return "Agent"
}

func (a *Agent) Run(ctx context.Context, input *adk.AgentInput, options ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	return a.inner.Run(ctx, input, options...)
}

func (a *Agent) Resume(ctx context.Context, info *adk.ResumeInfo, opts ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	if resumable, ok := a.inner.(adk.ResumableAgent); ok {
		return resumable.Resume(ctx, info, opts...)
	}
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		gen.Send(&adk.AgentEvent{Err: context.Canceled})
		gen.Close()
	}()
	return iter
}
