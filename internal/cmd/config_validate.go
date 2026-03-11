package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"

	"lakeside/internal/infra/rediskit"
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
	// Agent ITSM 子代理公共配置。
	Agent agentConfig `json:"agent" v:"required"`
	// Agents 多层 agent 平台配置。
	Agents agentsConfig `json:"agents" v:"required"`
	// ITSM 下游接口配置。
	ITSM itsmConfig `json:"itsm" v:"required"`
	// UniAuth 用户身份查询配置。
	UniAuth uniauthConfig `json:"uniauth" v:"required"`
}

type serverConfig struct {
	Address     string `json:"address" v:"required#server.address 不能为空"`
	Mode        string `json:"mode"`
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
	EnumConfidenceThreshold float64                `json:"enumConfidenceThreshold" v:"required|min:0|max:1#agent.enumConfidenceThreshold 不能为空|agent.enumConfidenceThreshold 不能小于 0|agent.enumConfidenceThreshold 不能大于 1"`
	Redis                   agentRedisConfig       `json:"redis" v:"required"`
	Checkpoint              agentCheckpointConfig  `json:"checkpoint" v:"required"`
	Idempotency             agentIdempotencyConfig `json:"idempotency" v:"required"`
}

type agentRedisConfig struct {
	Addr     string `json:"addr" v:"required#agent.redis.addr 不能为空"`
	Password string `json:"password"`
	DB       int    `json:"db" v:"min:0#agent.redis.db 不能小于 0"`
}

type agentCheckpointConfig struct {
	TTLHours int `json:"ttlHours" v:"required|min:1|max:168#agent.checkpoint.ttlHours 不能为空|agent.checkpoint.ttlHours 至少 1 小时|agent.checkpoint.ttlHours 不能超过 168 小时"`
}

type agentIdempotencyConfig struct {
	TTLHours int `json:"ttlHours" v:"required|min:1|max:168#agent.idempotency.ttlHours 不能为空|agent.idempotency.ttlHours 至少 1 小时|agent.idempotency.ttlHours 不能超过 168 小时"`
}

type agentsConfig struct {
	Storage       agentsStorageConfig       `json:"storage" v:"required"`
	Runtime       agentsRuntimeConfig       `json:"runtime"`
	Checkpoint    agentsCheckpointConfig    `json:"checkpoint" v:"required"`
	Summarization agentsSummarizationConfig `json:"summarization" v:"required"`
	Memory        agentsMemoryConfig        `json:"memory" v:"required"`
	RAG           agentsRAGConfig           `json:"rag"`
	Roots         []agentsSupervisorConfig  `json:"roots" v:"required"`
	Domains       []agentsSupervisorConfig  `json:"domains" v:"required"`
	Leaves        []agentsLeafConfig        `json:"leaves" v:"required"`
}

type agentsStorageConfig struct {
	Provider   string            `json:"provider"`
	SQLitePath string            `json:"sqlitePath"`
	MSSQL      agentsMSSQLConfig `json:"mssql"`
}

type agentsMSSQLConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
}

type agentsRuntimeConfig struct {
	WorkerCount    int    `json:"workerCount"`
	StreamKey      string `json:"streamKey"`
	StreamGroup    string `json:"streamGroup"`
	ConsumerPrefix string `json:"consumerPrefix"`
	ReadBlockMs    int    `json:"readBlockMs"`
	PubSubChannel  string `json:"pubsubChannel"`
}

type agentsCheckpointConfig struct {
	TTLHours int `json:"ttlHours" v:"required|min:1|max:168#agents.checkpoint.ttlHours 不能为空|agents.checkpoint.ttlHours 至少 1 小时|agents.checkpoint.ttlHours 不能超过 168 小时"`
}

type agentsSummarizationConfig struct {
	ContextTokens             int `json:"contextTokens" v:"required|min:1000|max:200000#agents.summarization.contextTokens 不能为空|agents.summarization.contextTokens 过小|agents.summarization.contextTokens 过大"`
	PreserveUserMessageTokens int `json:"preserveUserMessageTokens" v:"required|min:100|max:100000#agents.summarization.preserveUserMessageTokens 不能为空|agents.summarization.preserveUserMessageTokens 过小|agents.summarization.preserveUserMessageTokens 过大"`
}

type agentsMemoryConfig struct {
	MaxItems  int `json:"maxItems" v:"required|min:1|max:1000#agents.memory.maxItems 不能为空|agents.memory.maxItems 至少为 1|agents.memory.maxItems 不能超过 1000"`
	Workers   int `json:"workers" v:"required|min:1|max:32#agents.memory.workers 不能为空|agents.memory.workers 至少为 1|agents.memory.workers 不能超过 32"`
	QueueSize int `json:"queueSize" v:"required|min:1|max:5000#agents.memory.queueSize 不能为空|agents.memory.queueSize 至少为 1|agents.memory.queueSize 不能超过 5000"`
}

type agentsRAGConfig struct {
	BaseURL     string `json:"baseURL"`
	TimeoutMs   int    `json:"timeoutMs"`
	DefaultTopK int    `json:"defaultTopK"`
}

type agentsSupervisorConfig struct {
	Key                 string   `json:"key"`
	Description         string   `json:"description"`
	InstructionTemplate string   `json:"instructionTemplate"`
	Children            []string `json:"children"`
	MaxIterations       int      `json:"maxIterations"`
}

type agentsLeafConfig struct {
	Key            string   `json:"key"`
	Type           string   `json:"type"`
	Description    string   `json:"description"`
	KBIDs          []string `json:"kbIDs"`
	TopK           int      `json:"topK"`
	RewriteQueries int      `json:"rewriteQueries"`
	MaxContextDocs int      `json:"maxContextDocs"`
	SourceLimit    int      `json:"sourceLimit"`
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
	RetentionHours        int     `json:"retentionHours" v:"required|min:1|max:168#itsm.signal.retentionHours 不能为空|itsm.signal.retentionHours 至少为 1|itsm.signal.retentionHours 不能超过 168 小时"`
	MinDistinctUsersForP1 int     `json:"minDistinctUsersForP1" v:"required|min:2|max:100#itsm.signal.minDistinctUsersForP1 不能为空|itsm.signal.minDistinctUsersForP1 至少为 2|itsm.signal.minDistinctUsersForP1 不能超过 100"`
	MaxCandidates         int     `json:"maxCandidates" v:"required|min:10|max:5000#itsm.signal.maxCandidates 不能为空|itsm.signal.maxCandidates 至少为 10|itsm.signal.maxCandidates 不能超过 5000"`
	SimilarityThreshold   float64 `json:"similarityThreshold" v:"required|min:0|max:1#itsm.signal.similarityThreshold 不能为空|itsm.signal.similarityThreshold 不能小于 0|itsm.signal.similarityThreshold 不能大于 1"`
}

type uniauthConfig struct {
	BaseURL   string `json:"baseURL" v:"required|url#uniauth.baseURL 不能为空|uniauth.baseURL 不是合法 URL"`
	TimeoutMs int    `json:"timeoutMs" v:"required|min:100|max:120000#uniauth.timeoutMs 不能为空|uniauth.timeoutMs 过小|uniauth.timeoutMs 过大"`
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
	if config.ITSM.Signal.Enabled && strings.TrimSpace(config.Embedding.OpenRouter.APIKey) == "" && strings.TrimSpace(config.Model.OpenRouter.APIKey) == "" {
		g.Log().Fatalf(ctx, "配置文件校验失败: itsm.signal.enabled=true 时，embedding.openrouter.apiKey 为空则 model.openrouter.apiKey 不能为空")
	}
	if !strings.HasPrefix(config.Server.OpenAPIPath, "/") {
		g.Log().Fatal(ctx, "配置文件校验失败: server.openapiPath 必须以 / 开头")
	}
	if !strings.HasPrefix(config.Server.SwaggerPath, "/") {
		g.Log().Fatal(ctx, "配置文件校验失败: server.swaggerPath 必须以 / 开头")
	}
	serverMode := strings.ToLower(strings.TrimSpace(config.Server.Mode))
	if serverMode != "" && serverMode != "api" && serverMode != "worker" && serverMode != "all" {
		g.Log().Fatalf(ctx, "配置文件校验失败: server.mode 仅支持 api/worker/all，当前=%s", config.Server.Mode)
	}
	if len(config.ITSM.Retry.BackoffMs) != 3 {
		g.Log().Fatalf(ctx, "配置文件校验失败: itsm.retry.backoffMs 必须为 3 个回退值，当前长度=%d", len(config.ITSM.Retry.BackoffMs))
	}
	for i, v := range config.ITSM.Retry.BackoffMs {
		if v <= 0 {
			g.Log().Fatalf(ctx, "配置文件校验失败: itsm.retry.backoffMs[%d] 必须大于 0，当前值=%d", i, v)
		}
	}
	if config.Agents.Summarization.PreserveUserMessageTokens >= config.Agents.Summarization.ContextTokens {
		g.Log().Fatalf(ctx, "配置文件校验失败: agents.summarization.preserveUserMessageTokens 必须小于 contextTokens")
	}
	validateAgentStorageConfig(ctx, &config)
	validateAgentRuntimeConfig(ctx, &config)
	validateAgentTree(ctx, &config)
	validateRequiredRedis(ctx)
	g.Log().Info(ctx, "配置文件校验通过")
}

func validateAgentStorageConfig(ctx context.Context, config *Config) {
	if config == nil {
		g.Log().Fatal(ctx, "配置文件校验失败: config 为空")
	}
	provider := strings.ToLower(strings.TrimSpace(config.Agents.Storage.Provider))
	if provider == "" {
		provider = "sqlite"
	}
	switch provider {
	case "sqlite":
		if strings.TrimSpace(config.Agents.Storage.SQLitePath) == "" {
			g.Log().Fatal(ctx, "配置文件校验失败: agents.storage.provider=sqlite 时，agents.storage.sqlitePath 不能为空")
		}
	case "mssql":
		m := config.Agents.Storage.MSSQL
		if strings.TrimSpace(m.Host) == "" || strings.TrimSpace(m.User) == "" || strings.TrimSpace(m.Password) == "" || strings.TrimSpace(m.Database) == "" {
			g.Log().Fatal(ctx, "配置文件校验失败: agents.storage.provider=mssql 时，agents.storage.mssql.host/user/password/database 必须填写")
		}
		if m.Port <= 0 {
			g.Log().Fatal(ctx, "配置文件校验失败: agents.storage.provider=mssql 时，agents.storage.mssql.port 必须大于 0")
		}
	default:
		g.Log().Fatalf(ctx, "配置文件校验失败: agents.storage.provider 仅支持 sqlite/mssql，当前=%s", provider)
	}
}

func validateAgentRuntimeConfig(ctx context.Context, config *Config) {
	if config == nil {
		return
	}
	runtime := config.Agents.Runtime
	if runtime.WorkerCount < 0 {
		g.Log().Fatal(ctx, "配置文件校验失败: agents.runtime.workerCount 不能小于 0")
	}
	if runtime.ReadBlockMs < 0 {
		g.Log().Fatal(ctx, "配置文件校验失败: agents.runtime.readBlockMs 不能小于 0")
	}
}

func validateRequiredRedis(ctx context.Context) {
	client := rediskit.MustClient(ctx)
	_ = client.Close()
}

func validateAgentTree(ctx context.Context, config *Config) {
	if config == nil {
		g.Log().Fatal(ctx, "配置文件校验失败: agents 配置为空")
	}
	if len(config.Agents.Roots) == 0 {
		g.Log().Fatal(ctx, "配置文件校验失败: agents.roots 至少需要配置一个顶层助手")
	}
	if len(config.Agents.Domains) == 0 {
		g.Log().Fatal(ctx, "配置文件校验失败: agents.domains 至少需要配置一个领域助手")
	}
	if len(config.Agents.Leaves) == 0 {
		g.Log().Fatal(ctx, "配置文件校验失败: agents.leaves 至少需要配置一个叶子 agent")
	}

	rootKeys := make(map[string]struct{}, len(config.Agents.Roots))
	domainKeys := make(map[string]struct{}, len(config.Agents.Domains))
	leafKeys := make(map[string]struct{}, len(config.Agents.Leaves))
	allKeys := make(map[string]struct{}, len(config.Agents.Roots)+len(config.Agents.Domains)+len(config.Agents.Leaves))
	hasKnowledgeLeaf := false

	for i, item := range config.Agents.Roots {
		validateSupervisorNode(ctx, fmt.Sprintf("agents.roots[%d]", i), item)
		registerAgentKey(ctx, fmt.Sprintf("agents.roots[%d]", i), item.Key, rootKeys, allKeys)
	}
	for i, item := range config.Agents.Domains {
		validateSupervisorNode(ctx, fmt.Sprintf("agents.domains[%d]", i), item)
		registerAgentKey(ctx, fmt.Sprintf("agents.domains[%d]", i), item.Key, domainKeys, allKeys)
	}
	for i, item := range config.Agents.Leaves {
		validateLeafNode(ctx, fmt.Sprintf("agents.leaves[%d]", i), item)
		registerAgentKey(ctx, fmt.Sprintf("agents.leaves[%d]", i), item.Key, leafKeys, allKeys)
		if strings.EqualFold(strings.TrimSpace(item.Type), "knowledge") {
			hasKnowledgeLeaf = true
		}
	}

	if hasKnowledgeLeaf {
		if strings.TrimSpace(config.Agents.RAG.BaseURL) == "" {
			g.Log().Fatal(ctx, "配置文件校验失败: 存在 knowledge 叶子 agent 时，agents.rag.baseURL 不能为空")
		}
		if !strings.HasPrefix(strings.TrimSpace(config.Agents.RAG.BaseURL), "http://") && !strings.HasPrefix(strings.TrimSpace(config.Agents.RAG.BaseURL), "https://") {
			g.Log().Fatal(ctx, "配置文件校验失败: agents.rag.baseURL 必须是合法 URL")
		}
		if config.Agents.RAG.TimeoutMs < 100 || config.Agents.RAG.TimeoutMs > 120000 {
			g.Log().Fatal(ctx, "配置文件校验失败: agents.rag.timeoutMs 必须在 100~120000 之间")
		}
		if config.Agents.RAG.DefaultTopK < 1 || config.Agents.RAG.DefaultTopK > 50 {
			g.Log().Fatal(ctx, "配置文件校验失败: agents.rag.defaultTopK 必须在 1~50 之间")
		}
	}

	for i, item := range config.Agents.Roots {
		for j, child := range item.Children {
			child = strings.TrimSpace(child)
			if _, ok := domainKeys[child]; ok {
				continue
			}
			if _, ok := leafKeys[child]; ok {
				continue
			}
			g.Log().Fatalf(ctx, "配置文件校验失败: agents.roots[%d].children[%d]=%q 未在 agents.domains 或 agents.leaves 中定义", i, j, child)
		}
	}
	for i, item := range config.Agents.Domains {
		for j, child := range item.Children {
			child = strings.TrimSpace(child)
			if _, ok := leafKeys[child]; ok {
				continue
			}
			g.Log().Fatalf(ctx, "配置文件校验失败: agents.domains[%d].children[%d]=%q 未在 agents.leaves 中定义", i, j, child)
		}
	}
}

func validateSupervisorNode(ctx context.Context, path string, item agentsSupervisorConfig) {
	if strings.TrimSpace(item.Key) == "" {
		g.Log().Fatalf(ctx, "配置文件校验失败: %s.key 不能为空", path)
	}
	if strings.TrimSpace(item.Description) == "" {
		g.Log().Fatalf(ctx, "配置文件校验失败: %s.description 不能为空", path)
	}
	if strings.TrimSpace(item.InstructionTemplate) == "" {
		g.Log().Fatalf(ctx, "配置文件校验失败: %s.instructionTemplate 不能为空", path)
	}
	if len(item.Children) == 0 {
		g.Log().Fatalf(ctx, "配置文件校验失败: %s.children 至少需要一个 child", path)
	}
	if item.MaxIterations <= 0 || item.MaxIterations > 50 {
		g.Log().Fatalf(ctx, "配置文件校验失败: %s.maxIterations 必须在 1~50 之间", path)
	}
}

func validateLeafNode(ctx context.Context, path string, item agentsLeafConfig) {
	if strings.TrimSpace(item.Key) == "" {
		g.Log().Fatalf(ctx, "配置文件校验失败: %s.key 不能为空", path)
	}
	if strings.TrimSpace(item.Description) == "" {
		g.Log().Fatalf(ctx, "配置文件校验失败: %s.description 不能为空", path)
	}
	switch strings.ToLower(strings.TrimSpace(item.Type)) {
	case "itsm":
	case "knowledge":
		if len(item.KBIDs) == 0 {
			g.Log().Fatalf(ctx, "配置文件校验失败: %s.kbIDs 至少需要一个 kb_id", path)
		}
		for i, kbID := range item.KBIDs {
			if strings.TrimSpace(kbID) == "" {
				g.Log().Fatalf(ctx, "配置文件校验失败: %s.kbIDs[%d] 不能为空", path, i)
			}
		}
		if item.TopK < 0 || item.TopK > 50 {
			g.Log().Fatalf(ctx, "配置文件校验失败: %s.topK 不能小于 0 或大于 50", path)
		}
		if item.RewriteQueries < 0 || item.RewriteQueries > 8 {
			g.Log().Fatalf(ctx, "配置文件校验失败: %s.rewriteQueries 不能小于 0 或大于 8", path)
		}
		if item.MaxContextDocs < 0 || item.MaxContextDocs > 20 {
			g.Log().Fatalf(ctx, "配置文件校验失败: %s.maxContextDocs 不能小于 0 或大于 20", path)
		}
		if item.SourceLimit < 0 || item.SourceLimit > 20 {
			g.Log().Fatalf(ctx, "配置文件校验失败: %s.sourceLimit 不能小于 0 或大于 20", path)
		}
		if item.SourceLimit > 0 && item.MaxContextDocs > 0 && item.SourceLimit > item.MaxContextDocs {
			g.Log().Fatalf(ctx, "配置文件校验失败: %s.sourceLimit 不能大于 maxContextDocs", path)
		}
	default:
		g.Log().Fatalf(ctx, "配置文件校验失败: %s.type 仅支持 itsm/knowledge，当前值=%q", path, item.Type)
	}
}

func registerAgentKey(ctx context.Context, path, key string, own map[string]struct{}, all map[string]struct{}) {
	key = strings.TrimSpace(key)
	if _, ok := own[key]; ok {
		g.Log().Fatalf(ctx, "配置文件校验失败: %s.key=%q 在同层级重复", path, key)
	}
	if _, ok := all[key]; ok {
		g.Log().Fatalf(ctx, "配置文件校验失败: %s.key=%q 与其他 agent key 重复", path, key)
	}
	own[key] = struct{}{}
	all[key] = struct{}{}
}

func (c Config) String() string {
	return fmt.Sprintf("provider=%s itsmBaseURL=%s", c.Model.Provider, c.ITSM.BaseURL)
}
