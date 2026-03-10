package embeddings

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"lakeside/internal/consts"
	"lakeside/internal/infra/rediskit"

	cacheembed "github.com/cloudwego/eino-ext/components/embedding/cache"
	cacheredis "github.com/cloudwego/eino-ext/components/embedding/cache/redis"
	openaienbed "github.com/cloudwego/eino-ext/components/embedding/openai"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/gogf/gf/v2/frame/g"
)

const openRouterEmbeddingBaseURL = "https://openrouter.ai/api/v1"

// Service 统一封装项目内的 embedding 能力。
// 当前职责只有两件事：
// 1. 用 OpenRouter 的 OpenAI-compatible embeddings 生成向量
// 2. 用 Eino cache embedding 把重复文本结果缓存到 Redis
type Service struct {
	embedder embedding.Embedder
	model    string
}

var (
	once sync.Once
	inst *Service
)

// GetService 返回全局单例 embedding 服务。
// 首次调用时会按配置初始化 OpenRouter embedder 与可选的 Redis 缓存层。
func GetService(ctx context.Context) *Service {
	once.Do(func() {
		inst = newService(ctx)
	})
	return inst
}

// newService 按配置创建 embedding 服务实例。
// 当模型配置不完整或初始化失败时，会按能力降级，但不阻塞主流程启动。
func newService(ctx context.Context) *Service {
	apiKey := strings.TrimSpace(g.Cfg().MustGet(ctx, "embedding.openrouter.apiKey").String())
	if apiKey == "" {
		apiKey = strings.TrimSpace(g.Cfg().MustGet(ctx, "model.openrouter.apiKey").String())
	}
	baseURL := strings.TrimSpace(g.Cfg().MustGet(ctx, "embedding.openrouter.baseURL").String())
	if baseURL == "" {
		baseURL = openRouterEmbeddingBaseURL
	}
	modelName := strings.TrimSpace(g.Cfg().MustGet(ctx, "embedding.openrouter.modelName", "qwen/qwen3-embedding-8b").String())
	if apiKey == "" || modelName == "" {
		g.Log().Warning(ctx, "embedding service disabled: missing embedding.openrouter apiKey or modelName")
		return &Service{model: modelName}
	}

	baseEmbedder, err := openaienbed.NewEmbedder(ctx, &openaienbed.EmbeddingConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   modelName,
		Timeout: time.Duration(g.Cfg().MustGet(ctx, "embedding.openrouter.timeoutMs", 15000).Int()) * time.Millisecond,
	})
	if err != nil {
		g.Log().Warningf(ctx, "init embedding service failed: %v", err)
		return &Service{model: modelName}
	}

	embedder := embedding.Embedder(baseEmbedder)
	if g.Cfg().MustGet(ctx, "embedding.cache.enabled", true).Bool() {
		redisClient := rediskit.MustClient(ctx)
		// cache 前缀由 Redis namespace 统一控制，具体 key 仍由自定义 generator 决定，
		// 这样后续即使多个 agent 共用 embedding cache，也不会和 checkpoint 等键混淆。
		cachedEmbedder, cacheErr := cacheembed.NewEmbedder(embedder,
			cacheembed.WithCacher(cacheredis.NewCacher(redisClient, cacheredis.WithPrefix(consts.SharedEmbeddingCachePrefix))),
			cacheembed.WithGenerator(&keyGenerator{provider: "openrouter"}),
			cacheembed.WithExpiration(time.Duration(g.Cfg().MustGet(ctx, "embedding.cache.ttlHours", 24).Int())*time.Hour),
		)
		if cacheErr != nil {
			g.Log().Warningf(ctx, "init embedding cache failed, fallback to direct embedder: %v", cacheErr)
		} else {
			embedder = cachedEmbedder
		}
	}

	g.Log().Infof(ctx, "init embedding service success, model=%s cache_enabled=%t", modelName, g.Cfg().MustGet(ctx, "embedding.cache.enabled", true).Bool())
	return &Service{embedder: embedder, model: modelName}
}

// EmbedText 是当前项目最小化暴露的 embedding 接口：单段文本进，单个向量出。
func (s *Service) EmbedText(ctx context.Context, text string) ([]float64, error) {
	if s == nil || s.embedder == nil {
		return nil, fmt.Errorf("embedding service unavailable")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("embedding input is empty")
	}
	vectors, err := s.embedder.EmbedStrings(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 || len(vectors[0]) == 0 {
		return nil, fmt.Errorf("embedding result is empty")
	}
	return vectors[0], nil
}

// ModelName 返回当前 embedding 模型名，便于日志或调试输出。
func (s *Service) ModelName() string {
	if s == nil {
		return ""
	}
	return s.model
}

// keyGenerator 负责生成 embedding cache 的业务 key。
// 最终 Redis key 形如：
// lakeside:shared:embedding:cache:v1:openrouter:qwen_qwen3_embedding_8b:{sha256}
type keyGenerator struct {
	provider string
}

// Generate 生成 embedding cache 的业务 key 片段。
// Redis 最终键会由 cache 前缀与这里返回的 provider:model:hash 共同组成。
func (gk *keyGenerator) Generate(_ context.Context, text string, opt cacheembed.GeneratorOption) string {
	provider := sanitizeSegment(gk.provider)
	if provider == "" {
		provider = "default"
	}
	model := sanitizeSegment(opt.Model)
	if model == "" {
		model = "default"
	}
	h := sha256.Sum256([]byte(opt.Model + "\n" + text))
	return provider + ":" + model + ":" + hex.EncodeToString(h[:])
}

// sanitizeSegment 把 provider、model 等配置值清洗成适合写入 Redis key 的片段。
// 主要用于统一大小写，并替换路径分隔符、空格等不稳定字符。
func sanitizeSegment(value string) string {
	replacer := strings.NewReplacer("/", "_", ":", "_", " ", "_", "-", "_")
	return strings.Trim(replacer.Replace(strings.ToLower(strings.TrimSpace(value))), "_")
}
