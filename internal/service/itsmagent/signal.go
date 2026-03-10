package itsmagent

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"lakeside/internal/consts"
	"lakeside/internal/service/embeddings"
	"lakeside/internal/infra/rediskit"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	scopeUnknown    = "unknown"
	scopeSingleUser = "single_user"
	scopeMultiUser  = "multi_user"
	scopeRoom       = "room"
	scopeBuilding   = "building"
	scopeArea       = "area"
	scopeCampus     = "campus"
)

type signalService struct {
	client     *redis.Client
	embeddings *embeddings.Service
	cfg        signalConfig
}

// signalConfig 描述“相似问题聚合”这层服务自己的运行参数。
// 这里全部是服务端规则，不暴露给模型，也不要求前端理解。
type signalConfig struct {
	// itsm.signal.windowMinutes
	// 只回看最近 N 分钟内的相似事件。
	windowMinutes int
	// itsm.signal.retentionHours
	// Redis 中 signal 事件与 cluster 成员记录的保留时长。
	retentionHours int
	// itsm.signal.minDistinctUsersForP1
	// 最近窗口内至少多少个不同用户命中同簇，才允许升到 P1。
	minDistinctUsers int
	// itsm.signal.maxCandidates
	// 每次聚类时最多回看的候选事件数，避免单次扫描过大。
	maxCandidates int
	// itsm.signal.similarityThreshold
	// embedding 余弦相似度阈值，达到后才认为是同一簇。
	similarityThreshold float64
	// 当前 ITSM 最近事件 ZSET 的 key。
	recentKey string
	// 当前 ITSM signal 事件详情记录前缀。
	eventKeyPrefix string
	// 当前 ITSM signal cluster 成员记录前缀。
	clusterKeyPrefix string
}

// signalEvent 是写入 Redis 的标准化事件。
// 它不是下游 ITSM payload，而是“相似性分析专用”结构。
type signalEvent struct {
	ID                   string    `json:"id"`
	ClusterID            string    `json:"cluster_id"`
	UserCode             string    `json:"user_code"`
	Priority             string    `json:"priority"`
	OriginalServiceLevel string    `json:"original_service_level"`
	AppliedServiceLevel  string    `json:"applied_service_level"`
	Kind                 string    `json:"kind"`
	Domain               string    `json:"domain"`
	Object               string    `json:"object"`
	Symptom              string    `json:"symptom"`
	Scope                string    `json:"scope"`
	LocationScope        string    `json:"location_scope"`
	NormalizedSummary    string    `json:"normalized_summary"`
	Embedding            []float64 `json:"embedding"`
	CreatedAt            time.Time `json:"created_at"`
}

// signalDecision 是本次提交前的聚合判断结果。
// 它既包含最终是否升到 P1，也保留聚类细节，便于日志与后续落库。
type signalDecision struct {
	Event            signalEvent
	AppliedLevel     string
	DistinctUsers    int
	ImpactScope      string
	ShouldPromoteP1  bool
	Similarity       float64
	MatchedClusterID string
}

// newSignalService 构造 ITSM 相似问题聚合服务。
// 这层是“增强能力”：signal 关闭或 embedding 不可用时直接关闭，不阻塞工单主流程。
func newSignalService(ctx context.Context, embedder *embeddings.Service) *signalService {
	if !g.Cfg().MustGet(ctx, "itsm.signal.enabled", true).Bool() {
		return nil
	}
	client := rediskit.MustClient(ctx)
	cfg := signalConfig{
		windowMinutes:       positiveInt(g.Cfg().MustGet(ctx, "itsm.signal.windowMinutes", 10).Int(), 10),
		retentionHours:      positiveInt(g.Cfg().MustGet(ctx, "itsm.signal.retentionHours", 24).Int(), 24),
		minDistinctUsers:    positiveInt(g.Cfg().MustGet(ctx, "itsm.signal.minDistinctUsersForP1", 5).Int(), 5),
		maxCandidates:       positiveInt(g.Cfg().MustGet(ctx, "itsm.signal.maxCandidates", 200).Int(), 200),
		similarityThreshold: positiveFloat(g.Cfg().MustGet(ctx, "itsm.signal.similarityThreshold", 0.90).Float64(), 0.90),
		recentKey:           consts.ITSMSignalRecentKey,
		eventKeyPrefix:      consts.ITSMSignalEventPrefix,
		clusterKeyPrefix:    consts.ITSMSignalClusterPrefix,
	}
	return &signalService{client: client, embeddings: embedder, cfg: cfg}
}

// Prepare 在提交工单前评估“是否需要因集中爆发而升级到 P1”，但不会立刻落库。
func (s *signalService) Prepare(ctx context.Context, draft TicketDraft) (*signalDecision, error) {
	if s == nil || s.client == nil || s.embeddings == nil {
		return nil, nil
	}
	// 当前只对故障类工单做聚合升级，避免把咨询/服务/反馈误升到最高等级。
	if strings.TrimSpace(draft.Priority) != "3" {
		return nil, nil
	}
	if strings.TrimSpace(draft.ServiceLevel) != "1" && strings.TrimSpace(draft.ServiceLevel) != "2" && strings.TrimSpace(draft.ServiceLevel) != "3" {
		return nil, nil
	}

	event := normalizeSignalEvent(draft)
	if strings.TrimSpace(event.NormalizedSummary) == "" {
		return nil, nil
	}
	vector, err := s.embeddings.EmbedText(ctx, event.NormalizedSummary)
	if err != nil {
		return nil, err
	}
	event.ID = "evt-" + uuid.NewString()
	event.CreatedAt = time.Now().UTC()
	event.Embedding = vector
	event.OriginalServiceLevel = strings.TrimSpace(draft.ServiceLevel)
	event.AppliedServiceLevel = event.OriginalServiceLevel

	recentEvents, err := s.loadRecentEvents(ctx, event.CreatedAt)
	if err != nil {
		return nil, err
	}

	// 先尝试归到已有 cluster；找不到再为本次事件分配新 cluster。
	clusterID, similarity := s.matchCluster(event, recentEvents)
	if clusterID == "" {
		clusterID = "cluster-" + uuid.NewString()
	}
	event.ClusterID = clusterID

	distinctUsers, impactScope := summarizeCluster(clusterID, recentEvents, event)
	decision := &signalDecision{
		Event:            event,
		AppliedLevel:     event.AppliedServiceLevel,
		DistinctUsers:    distinctUsers,
		ImpactScope:      impactScope,
		MatchedClusterID: clusterID,
		Similarity:       similarity,
	}

	if shouldPromoteP1(event.OriginalServiceLevel, distinctUsers, impactScope, s.cfg.minDistinctUsers) {
		decision.ShouldPromoteP1 = true
		decision.AppliedLevel = "1"
		decision.Event.AppliedServiceLevel = "1"
	}
	return decision, nil
}

// Record 在下游建单成功后持久化本次事件，供后续相似问题聚合使用。
func (s *signalService) Record(ctx context.Context, decision *signalDecision) error {
	if s == nil || s.client == nil || decision == nil {
		return nil
	}
	payload, err := json.Marshal(decision.Event)
	if err != nil {
		return err
	}
	retention := time.Duration(s.cfg.retentionHours) * time.Hour
	score := float64(decision.Event.CreatedAt.Unix())
	eventKey := s.cfg.eventKeyPrefix + decision.Event.ID
	clusterKey := s.cfg.clusterKeyPrefix + decision.Event.ClusterID
	cutoff := fmt.Sprintf("%d", decision.Event.CreatedAt.Add(-retention).Unix())

	pipe := s.client.TxPipeline()
	pipe.Set(ctx, eventKey, payload, retention)
	pipe.ZAdd(ctx, s.cfg.recentKey, redis.Z{Score: score, Member: decision.Event.ID})
	pipe.Expire(ctx, s.cfg.recentKey, retention)
	pipe.ZRemRangeByScore(ctx, s.cfg.recentKey, "-inf", cutoff)
	pipe.ZAdd(ctx, clusterKey, redis.Z{Score: score, Member: decision.Event.ID})
	pipe.Expire(ctx, clusterKey, retention)
	pipe.ZRemRangeByScore(ctx, clusterKey, "-inf", cutoff)
	_, err = pipe.Exec(ctx)
	return err
}

// loadRecentEvents 只回看最近 windowMinutes 内的事件，避免把历史旧故障误算进当前爆发。
func (s *signalService) loadRecentEvents(ctx context.Context, now time.Time) ([]signalEvent, error) {
	windowStart := now.Add(-time.Duration(s.cfg.windowMinutes) * time.Minute).Unix()
	members, err := s.client.ZRevRangeByScore(ctx, s.cfg.recentKey, &redis.ZRangeBy{
		Min:    fmt.Sprintf("%d", windowStart),
		Max:    "+inf",
		Offset: 0,
		Count:  int64(s.cfg.maxCandidates),
	}).Result()
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(members))
	for _, member := range members {
		keys = append(keys, s.cfg.eventKeyPrefix+member)
	}
	items, err := s.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	out := make([]signalEvent, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		var event signalEvent
		if err := json.Unmarshal([]byte(text), &event); err != nil {
			continue
		}
		out = append(out, event)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// matchCluster 用“结构化粗过滤 + embedding 相似度”把新事件归到最像的 cluster。
func (s *signalService) matchCluster(event signalEvent, candidates []signalEvent) (string, float64) {
	bestClusterID := ""
	bestSimilarity := 0.0
	for _, candidate := range candidates {
		if !sameCoarseGroup(event, candidate) {
			continue
		}
		similarity := cosineSimilarity(event.Embedding, candidate.Embedding)
		if similarity < s.cfg.similarityThreshold || similarity < bestSimilarity {
			continue
		}
		bestSimilarity = similarity
		bestClusterID = strings.TrimSpace(candidate.ClusterID)
		if bestClusterID == "" {
			bestClusterID = "cluster-" + candidate.ID
		}
	}
	return bestClusterID, bestSimilarity
}

// normalizeSignalEvent 把工单草稿变成可聚类的标准事件表示。
func normalizeSignalEvent(draft TicketDraft) signalEvent {
	combined := compactSignalText(strings.Join([]string{draft.Subject, draft.OthersDesc}, " "))
	domain := detectSignalDomain(combined)
	object := detectSignalObject(combined, domain)
	symptom := detectSignalSymptom(combined)
	locationScope := extractLocationScope(combined)
	scope := detectSignalScope(combined, locationScope)
	kind := priorityToKind(draft.Priority)

	return signalEvent{
		UserCode:          strings.TrimSpace(draft.UserCode),
		Priority:          strings.TrimSpace(draft.Priority),
		Kind:              kind,
		Domain:            domain,
		Object:            object,
		Symptom:           symptom,
		Scope:             scope,
		LocationScope:     locationScope,
		NormalizedSummary: buildNormalizedSummary(kind, domain, object, symptom, scope, locationScope, combined),
	}
}

func priorityToKind(priority string) string {
	switch strings.TrimSpace(priority) {
	case "1":
		return "consult"
	case "2":
		return "service"
	case "3":
		return "incident"
	case "4":
		return "feedback"
	default:
		return "unknown"
	}
}

// detectSignalDomain 尽量把故障归到稳定的大类，便于先做粗过滤再做向量相似度判断。
func detectSignalDomain(text string) string {
	switch {
	case containsAny(text, "wifi", "无线", "网络", "网页", "上网", "internet", "network", "vpn"):
		return "network"
	case containsAny(text, "邮件", "邮箱", "email", "mail", "outlook", "smtp"):
		return "mail"
	case containsAny(text, "打印", "打印机", "printer", "print"):
		return "printer"
	case containsAny(text, "账号", "登录", "密码", "sso", "account", "login", "password", "权限", "permission"):
		return "account"
	case containsAny(text, "电脑", "笔记本", "设备", "蓝屏", "开机", "laptop", "pc", "device"):
		return "device"
	case containsAny(text, "会议", "投影", "会议室", "meeting", "projector", "zoom", "teams"):
		return "meeting"
	default:
		return "other"
	}
}

// detectSignalObject 在 domain 内继续下钻，减少“同域不同问题”被误聚到一起。
func detectSignalObject(text, domain string) string {
	switch domain {
	case "network":
		switch {
		case containsAny(text, "wifi", "无线"):
			return "wifi"
		case containsAny(text, "网页", "网站", "web", "browser"):
			return "web"
		case containsAny(text, "vpn"):
			return "vpn"
		default:
			return "network"
		}
	case "mail":
		return "mailbox"
	case "printer":
		return "printer"
	case "account":
		if containsAny(text, "sso") {
			return "sso"
		}
		return "account"
	case "device":
		return "device"
	case "meeting":
		if containsAny(text, "投影", "projector") {
			return "projector"
		}
		return "meeting_room"
	default:
		return "other"
	}
}

// detectSignalSymptom 把用户原话中的现象标准化，避免每次都直接拿原文比对。
func detectSignalSymptom(text string) string {
	switch {
	case containsAny(text, "打不开", "无法打开", "无法访问", "打不开网页", "无法上网", "无法使用", "不可用", "down", "unavailable", "cannot open", "no internet"):
		return "unavailable"
	case containsAny(text, "很慢", "卡", "网络差", "延迟", "转圈", "degraded", "slow", "lag", "poor"):
		return "degraded"
	case containsAny(text, "登录失败", "认证失败", "密码错误", "权限", "auth", "login failed", "unauthorized", "forbidden"):
		return "auth_fail"
	case containsAny(text, "发送失败", "收不到", "无法收发", "send failed", "receive failed"):
		return "send_receive_fail"
	case containsAny(text, "打印失败", "卡纸", "无法打印", "print failed", "paper jam"):
		return "print_fail"
	case containsAny(text, "报错", "error", "exception", "异常"):
		return "error"
	default:
		return "unknown"
	}
}

// detectSignalScope 把单用户、多用户、楼宇、区域、全校等影响范围映射成固定枚举。
func detectSignalScope(text, locationScope string) string {
	switch {
	case containsAny(text, "全校", "全校园", "campus-wide", "campus wide", "整个学校"):
		return scopeCampus
	case containsAny(text, "多个区域", "多个楼", "多栋楼", "整片区域", "整个校区", "across buildings"):
		return scopeArea
	case containsAny(text, "整个书院", "整栋", "整层", "整楼", "多个寝室", "多个办公室", "多个教室", "whole building", "entire dorm"):
		return scopeBuilding
	case locationScope != "":
		return scopeBuilding
	case containsAny(text, "所有设备", "多台设备", "多个人", "多人", "多位同事", "many colleagues", "all devices", "multiple devices"):
		return scopeMultiUser
	case roomPattern.MatchString(text):
		return scopeRoom
	default:
		return scopeSingleUser
	}
}

// buildNormalizedSummary 生成稳定、短小、适合做 embedding 的摘要文本。
func buildNormalizedSummary(kind, domain, object, symptom, scope, locationScope, summary string) string {
	summary = strings.TrimSpace(summary)
	if len([]rune(summary)) > 80 {
		summary = string([]rune(summary)[:80])
	}
	return fmt.Sprintf("kind=%s;domain=%s;object=%s;symptom=%s;scope=%s;location=%s;summary=%s", kind, domain, object, symptom, scope, locationScope, summary)
}

// summarizeCluster 基于当前 cluster 内最近事件，估算“不同用户数”和“影响范围”。
// 影响范围除了直接看单条事件的 scope，还会结合同一地点是否被多名用户反复提到。
func summarizeCluster(clusterID string, candidates []signalEvent, current signalEvent) (int, string) {
	users := make(map[string]struct{})
	locationUsers := make(map[string]map[string]struct{})
	impactLevel := scopeLevel(current.Scope)
	addSignalUser(users, current.UserCode)
	addLocationUser(locationUsers, current.LocationScope, current.UserCode)
	for _, event := range candidates {
		if strings.TrimSpace(event.ClusterID) != strings.TrimSpace(clusterID) {
			continue
		}
		addSignalUser(users, event.UserCode)
		addLocationUser(locationUsers, event.LocationScope, event.UserCode)
		if level := scopeLevel(event.Scope); level > impactLevel {
			impactLevel = level
		}
	}
	if impactLevel < scopeLevel(scopeArea) {
		for _, groupedUsers := range locationUsers {
			if len(groupedUsers) >= 2 {
				impactLevel = scopeLevel(scopeBuilding)
				break
			}
		}
	}
	return len(users), scopeByLevel(impactLevel)
}

func addSignalUser(users map[string]struct{}, userCode string) {
	userCode = strings.TrimSpace(userCode)
	if userCode == "" {
		return
	}
	users[userCode] = struct{}{}
}

func addLocationUser(locationUsers map[string]map[string]struct{}, locationScope, userCode string) {
	locationScope = strings.TrimSpace(locationScope)
	userCode = strings.TrimSpace(userCode)
	if locationScope == "" || userCode == "" {
		return
	}
	if _, ok := locationUsers[locationScope]; !ok {
		locationUsers[locationScope] = make(map[string]struct{})
	}
	locationUsers[locationScope][userCode] = struct{}{}
}

func sameCoarseGroup(current, candidate signalEvent) bool {
	if current.Kind != candidate.Kind {
		return false
	}
	if current.Domain != "" && candidate.Domain != "" && current.Domain != candidate.Domain {
		return false
	}
	if current.Object != "" && candidate.Object != "" && current.Object != candidate.Object && current.Object != "other" && candidate.Object != "other" {
		return false
	}
	if current.Symptom != "" && candidate.Symptom != "" && current.Symptom != candidate.Symptom && current.Symptom != "unknown" && candidate.Symptom != "unknown" {
		return false
	}
	if current.LocationScope != "" && candidate.LocationScope != "" && current.LocationScope != candidate.LocationScope && scopeLevel(current.Scope) < scopeLevel(scopeArea) && scopeLevel(candidate.Scope) < scopeLevel(scopeArea) {
		return false
	}
	return true
}

// cosineSimilarity 用于应用侧向量比对；首期不依赖 Redis 向量索引。
func cosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func scopeLevel(scope string) int {
	switch strings.TrimSpace(scope) {
	case scopeSingleUser:
		return 1
	case scopeMultiUser:
		return 2
	case scopeRoom:
		return 3
	case scopeBuilding:
		return 4
	case scopeArea:
		return 5
	case scopeCampus:
		return 6
	default:
		return 0
	}
}

func scopeByLevel(level int) string {
	switch {
	case level >= scopeLevel(scopeCampus):
		return scopeCampus
	case level >= scopeLevel(scopeArea):
		return scopeArea
	case level >= scopeLevel(scopeBuilding):
		return scopeBuilding
	case level >= scopeLevel(scopeRoom):
		return scopeRoom
	case level >= scopeLevel(scopeMultiUser):
		return scopeMultiUser
	case level >= scopeLevel(scopeSingleUser):
		return scopeSingleUser
	default:
		return scopeUnknown
	}
}

func shouldPromoteP1(originalLevel string, distinctUsers int, impactScope string, minDistinctUsers int) bool {
	if strings.TrimSpace(originalLevel) != "2" && strings.TrimSpace(originalLevel) != "3" {
		return false
	}
	if distinctUsers < minDistinctUsers {
		return false
	}
	return scopeLevel(impactScope) >= scopeLevel(scopeBuilding)
}

func positiveInt(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func positiveFloat(value, fallback float64) float64 {
	if value <= 0 {
		return fallback
	}
	return value
}

var (
	roomPattern     = regexp.MustCompile(`(?i)(room\s*\w{1,6}|[a-z]?\d{3,4})`)
	buildingPattern = regexp.MustCompile(`([\p{Han}A-Za-z]{1,24}(书院|教学楼|楼|中心|馆|大厦|实验室|办公室|宿舍|公寓|building|dorm|office))`)
)

func extractLocationScope(text string) string {
	matches := buildingPattern.FindStringSubmatch(text)
	if len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

func compactSignalText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	fields := strings.Fields(text)
	return strings.Join(fields, " ")
}

func containsAny(text string, keywords ...string) bool {
	for _, keyword := range keywords {
		if keyword != "" && strings.Contains(text, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}
