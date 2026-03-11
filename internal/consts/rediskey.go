package consts

const (
	// RootAssistantCheckpointPrefix 是顶层助手 checkpoint 前缀。
	//   每个顶层助手都会在此基础上继续拼接 assistant_key，形成独立 checkpoint 命名空间。
	RootAssistantCheckpointPrefix = "lakeside:interactive:rootassistant:checkpoint:v1:"

	// AssistantCheckpointPrefix 是 IT 主助手 checkpoint 前缀。
	//   主助手（assistant host）保存 checkpoint 的默认前缀。
	AssistantCheckpointPrefix = "lakeside:interactive:it:host:main:checkpoint:v1:"

	// ITSMCheckpointPrefix 是 ITSM Sub-Agent Checkpoint 前缀。
	//   ITSM Sub-Agent 保存 checkpoint 的默认前缀。
	ITSMCheckpointPrefix = "lakeside:interactive:it:subagent:itsm:checkpoint:v1:"

	// ITSMIdempotencyPrefix 是 ITSM Sub-Agent 幂等缓存前缀。
	//   ITSM 提交幂等结果的默认前缀。
	ITSMIdempotencyPrefix = "lakeside:interactive:it:subagent:itsm:idempotency:v1:"

	// ITSMSignalEventPrefix 是 ITSM 信号事件存储前缀。
	//   相似问题聚合里，单条事件详情 key 的前缀（后面会拼 eventID）。
	//   最终 key：...:signal:event:v1:{eventID}
	ITSMSignalEventPrefix = "lakeside:interactive:it:subagent:itsm:signal:event:v1:"

	// ITSMSignalClusterPrefix 是 ITSM 相似簇存储前缀。
	//   存的是“某个相似簇的数据（成员/统计）”。后面会拼 clusterID）。
	//   最终 key：...:signal:cluster:v1:{clusterID}
	ITSMSignalClusterPrefix = "lakeside:interactive:it:subagent:itsm:signal:cluster:v1:"

	// ITSMSignalRecentKey 是 ITSM 最近事件的有序集合固定 key。
	//   存的是“最近事件 ID 列表（ZSET）”，用于时间窗检索。
	ITSMSignalRecentKey = "lakeside:interactive:it:subagent:itsm:signal:recent:v1"

	// SharedEmbeddingCachePrefix 是全局 Embedding 缓存前缀。
	//   这是 embedding 组件自己的缓存前缀，不属于 ITSM signal。
	//   最终会变成：...:embedding:cache:v1:{provider}:{model}:{hash}
	SharedEmbeddingCachePrefix = "lakeside:shared:embedding:cache:v1:"
)
