package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

// Config 定义项目启动时需要校验的配置结构。
// 该结构与 config/config.yaml 一一对应，用于在启动阶段提前发现缺失或格式错误。
type Config struct {
	// Server HTTP 服务配置。
	Server serverConfig `json:"server" v:"required"`
	// Logger 日志输出配置。
	Logger loggerConfig `json:"logger" v:"required"`
	// Model 对话模型配置。
	Model modelConfig `json:"model" v:"required"`
	// Embedding 向量模型与缓存配置。
	Embedding embeddingConfig `json:"embedding" v:"required"`
	// Agent ITSM 子代理配置。
	Agent agentConfig `json:"agent" v:"required"`
	// Assistant 顶层助手配置。
	Assistant assistantConfig `json:"assistant" v:"required"`
	// ITSM 下游接口配置。
	ITSM itsmConfig `json:"itsm" v:"required"`
}

type serverConfig struct {
	Address     string `json:"address" v:"required#server.address 不能为空"`
	OpenAPIPath string `json:"openapiPath" v:"required#server.openapiPath 不能为空"`
	SwaggerPath string `json:"swaggerPath" v:"required#server.swaggerPath 不能为空"`
}

type loggerConfig struct {
	Level  string `json:"level" v:"required#logger.level 不能为空"`
	Stdout bool   `json:"stdout"`
}

type modelConfig struct {
	Provider       string          `json:"provider" v:"required|in:qwen,openrouter#model.provider 不能为空|model.provider 仅支持 qwen/openrouter"`
	EnableThinking bool            `json:"enableThinking"`
	Qwen           modelQwenConfig `json:"qwen" v:"required"`
	OpenRouter     modelORConfig   `json:"openrouter" v:"required"`
}

type modelQwenConfig struct {
	APIKey    string `json:"apiKey"`
	ModelName string `json:"modelName"`
}

type modelORConfig struct {
	APIKey    string `json:"apiKey"`
	ModelName string `json:"modelName"`
}

type embeddingConfig struct {
	OpenRouter embeddingORConfig `json:"openrouter" v:"required"`
	Cache      embeddingCache    `json:"cache" v:"required"`
}

type embeddingORConfig struct {
	APIKey    string `json:"apiKey"`
	BaseURL   string `json:"baseURL" v:"required|url#embedding.openrouter.baseURL 不能为空|embedding.openrouter.baseURL 不是合法 URL"`
	ModelName string `json:"modelName" v:"required#embedding.openrouter.modelName 不能为空"`
	TimeoutMs int    `json:"timeoutMs" v:"required|min:100|max:120000#embedding.openrouter.timeoutMs 不能为空|embedding.openrouter.timeoutMs 过小|embedding.openrouter.timeoutMs 过大"`
}

type embeddingCache struct {
	Enabled  bool `json:"enabled"`
	TTLHours int  `json:"ttlHours" v:"required|min:1|max:168#embedding.cache.ttlHours 不能为空|embedding.cache.ttlHours 至少 1 小时|embedding.cache.ttlHours 不能超过 168 小时"`
}

type agentConfig struct {
	EnumConfidenceThreshold float64                 `json:"enumConfidenceThreshold" v:"required|min:0|max:1#agent.enumConfidenceThreshold 不能为空|agent.enumConfidenceThreshold 不能小于 0|agent.enumConfidenceThreshold 不能大于 1"`
	Redis                   agentRedisConfig        `json:"redis" v:"required"`
	Checkpoint              agentCheckpointConfig   `json:"checkpoint" v:"required"`
	Idempotency             agentIdempotencyConfig  `json:"idempotency" v:"required"`
}

type agentRedisConfig struct {
	Addr     string `json:"addr" v:"required#agent.redis.addr 不能为空"`
	Password string `json:"password"`
	DB       int    `json:"db" v:"min:0#agent.redis.db 不能小于 0"`
}

type agentCheckpointConfig struct {
	TTLHours  int    `json:"ttlHours" v:"required|min:1|max:168#agent.checkpoint.ttlHours 不能为空|agent.checkpoint.ttlHours 至少 1 小时|agent.checkpoint.ttlHours 不能超过 168 小时"`
}

type agentIdempotencyConfig struct {
	TTLHours  int    `json:"ttlHours" v:"required|min:1|max:168#agent.idempotency.ttlHours 不能为空|agent.idempotency.ttlHours 至少 1 小时|agent.idempotency.ttlHours 不能超过 168 小时"`
}

type assistantConfig struct {
	Storage       assistantStorageConfig       `json:"storage" v:"required"`
	Checkpoint    assistantCheckpointConfig    `json:"checkpoint" v:"required"`
	Summarization assistantSummarizationConfig `json:"summarization" v:"required"`
	Agent         assistantAgentConfig         `json:"agent" v:"required"`
	Memory        assistantMemoryConfig        `json:"memory" v:"required"`
}

type assistantStorageConfig struct {
	SQLitePath string `json:"sqlitePath" v:"required#assistant.storage.sqlitePath 不能为空"`
}

type assistantCheckpointConfig struct {
	TTLHours  int    `json:"ttlHours" v:"required|min:1|max:168#assistant.checkpoint.ttlHours 不能为空|assistant.checkpoint.ttlHours 至少 1 小时|assistant.checkpoint.ttlHours 不能超过 168 小时"`
}

type assistantSummarizationConfig struct {
	ContextTokens             int `json:"contextTokens" v:"required|min:1000|max:200000#assistant.summarization.contextTokens 不能为空|assistant.summarization.contextTokens 过小|assistant.summarization.contextTokens 过大"`
	PreserveUserMessageTokens int `json:"preserveUserMessageTokens" v:"required|min:100|max:100000#assistant.summarization.preserveUserMessageTokens 不能为空|assistant.summarization.preserveUserMessageTokens 过小|assistant.summarization.preserveUserMessageTokens 过大"`
}

type assistantAgentConfig struct {
	MaxIterations int `json:"maxIterations" v:"required|min:1|max:50#assistant.agent.maxIterations 不能为空|assistant.agent.maxIterations 至少为 1|assistant.agent.maxIterations 不能超过 50"`
}

type assistantMemoryConfig struct {
	MaxItems  int `json:"maxItems" v:"required|min:1|max:1000#assistant.memory.maxItems 不能为空|assistant.memory.maxItems 至少为 1|assistant.memory.maxItems 不能超过 1000"`
	Workers   int `json:"workers" v:"required|min:1|max:32#assistant.memory.workers 不能为空|assistant.memory.workers 至少为 1|assistant.memory.workers 不能超过 32"`
	QueueSize int `json:"queueSize" v:"required|min:1|max:5000#assistant.memory.queueSize 不能为空|assistant.memory.queueSize 至少为 1|assistant.memory.queueSize 不能超过 5000"`
}

type itsmConfig struct {
	BaseURL   string           `json:"baseURL" v:"required|url#itsm.baseURL 不能为空|itsm.baseURL 不是合法 URL"`
	AppSecret string           `json:"appSecret" v:"required#itsm.appSecret 不能为空"`
	TimeoutMs int              `json:"timeoutMs" v:"required|min:100|max:120000#itsm.timeoutMs 不能为空|itsm.timeoutMs 过小|itsm.timeoutMs 过大"`
	Retry     itsmRetryConfig  `json:"retry" v:"required"`
	Signal    itsmSignalConfig `json:"signal" v:"required"`
}

type itsmRetryConfig struct {
	MaxAttempts int   `json:"maxAttempts" v:"required|min:1|max:10#itsm.retry.maxAttempts 不能为空|itsm.retry.maxAttempts 至少为 1|itsm.retry.maxAttempts 不能超过 10"`
	BackoffMs   []int `json:"backoffMs" v:"required#itsm.retry.backoffMs 不能为空"`
}

type itsmSignalConfig struct {
	Enabled               bool    `json:"enabled"`
	WindowMinutes         int     `json:"windowMinutes" v:"required|min:1|max:120#itsm.signal.windowMinutes 不能为空|itsm.signal.windowMinutes 至少为 1|itsm.signal.windowMinutes 不能超过 120"`
	RetentionHours        int     `json:"retentionHours" v:"required|min:1|max:168#itsm.signal.retentionHours 不能为空|itsm.signal.retentionHours 至少为 1|itsm.signal.retentionHours 不能超过 168"`
	MinDistinctUsersForP1 int     `json:"minDistinctUsersForP1" v:"required|min:2|max:100#itsm.signal.minDistinctUsersForP1 不能为空|itsm.signal.minDistinctUsersForP1 至少为 2|itsm.signal.minDistinctUsersForP1 不能超过 100"`
	MaxCandidates         int     `json:"maxCandidates" v:"required|min:10|max:5000#itsm.signal.maxCandidates 不能为空|itsm.signal.maxCandidates 至少为 10|itsm.signal.maxCandidates 不能超过 5000"`
	SimilarityThreshold   float64 `json:"similarityThreshold" v:"required|min:0|max:1#itsm.signal.similarityThreshold 不能为空|itsm.signal.similarityThreshold 不能小于 0|itsm.signal.similarityThreshold 不能大于 1"`
}

func validateStartupConfig(ctx context.Context) {
	var config Config
	if err := gconv.Struct(g.Cfg("config.yaml").MustData(ctx), &config); err != nil {
		g.Log().Fatalf(ctx, "解析 CONFIG 失败: %v", err)
	}
	if err := g.Validator().Data(config).Run(ctx); err != nil {
		g.Dump(err.Items())
		g.Log().Fatalf(ctx, "配置文件校验失败:\n%s", err.String())
	}
	// provider 相关的跨字段规则单独校验，避免在 v 标签里写过多耦合逻辑。
	switch strings.ToLower(strings.TrimSpace(config.Model.Provider)) {
	case "qwen":
		if strings.TrimSpace(config.Model.Qwen.APIKey) == "" || strings.TrimSpace(config.Model.Qwen.ModelName) == "" {
			g.Log().Fatalf(ctx, "配置文件校验失败: model.provider=qwen 时，model.qwen.apiKey/model.qwen.modelName 必须填写")
		}
	case "openrouter":
		if strings.TrimSpace(config.Model.OpenRouter.APIKey) == "" || strings.TrimSpace(config.Model.OpenRouter.ModelName) == "" {
			g.Log().Fatalf(ctx, "配置文件校验失败: model.provider=openrouter 时，model.openrouter.apiKey/model.openrouter.modelName 必须填写")
		}
	default:
		g.Log().Fatalf(ctx, "配置文件校验失败: model.provider 仅支持 qwen/openrouter")
	}
	// embedding 可以复用 model.openrouter.apiKey，二者至少一个要有值。
	if strings.TrimSpace(config.Embedding.OpenRouter.APIKey) == "" && strings.TrimSpace(config.Model.OpenRouter.APIKey) == "" {
		g.Log().Fatalf(ctx, "配置文件校验失败: embedding.openrouter.apiKey 为空时，model.openrouter.apiKey 不能为空")
	}
	if !strings.HasPrefix(config.Server.OpenAPIPath, "/") {
		g.Log().Fatal(ctx, "配置文件校验失败: server.openapiPath 必须以 / 开头")
	}
	if !strings.HasPrefix(config.Server.SwaggerPath, "/") {
		g.Log().Fatal(ctx, "配置文件校验失败: server.swaggerPath 必须以 / 开头")
	}
	if len(config.ITSM.Retry.BackoffMs) != 3 {
		g.Log().Fatalf(ctx, "配置文件校验失败: itsm.retry.backoffMs 必须为 3 个回退值，当前长度=%d", len(config.ITSM.Retry.BackoffMs))
	}
	for i, v := range config.ITSM.Retry.BackoffMs {
		if v <= 0 {
			g.Log().Fatalf(ctx, "配置文件校验失败: itsm.retry.backoffMs[%d] 必须大于 0，当前值=%d", i, v)
		}
	}
	if config.Assistant.Summarization.PreserveUserMessageTokens >= config.Assistant.Summarization.ContextTokens {
		g.Log().Fatalf(ctx, "配置文件校验失败: assistant.summarization.preserveUserMessageTokens 必须小于 contextTokens")
	}
	g.Log().Info(ctx, "配置文件校验通过")
}

func (c Config) String() string {
	return fmt.Sprintf("provider=%s itsmBaseURL=%s", c.Model.Provider, c.ITSM.BaseURL)
}
