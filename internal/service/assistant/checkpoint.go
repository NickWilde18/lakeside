package assistant

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/redis/go-redis/v9"
)

type checkpointStore interface {
	adk.CheckPointStore
	Delete(ctx context.Context, checkPointID string) error
}

type inMemoryCheckPointStore struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newCheckpointStore(ctx context.Context) checkpointStore {
	redisAddr := strings.TrimSpace(g.Cfg().MustGet(ctx, "agent.redis.addr").String())
	if redisAddr == "" {
		g.Log().Warning(ctx, "assistant checkpoint redis addr is empty, fallback to in-memory store")
		return newInMemoryCheckpointStore()
	}
	client := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: g.Cfg().MustGet(ctx, "agent.redis.password").String(),
		DB:       g.Cfg().MustGet(ctx, "agent.redis.db", 0).Int(),
	})
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		g.Log().Warningf(ctx, "assistant checkpoint redis unavailable, fallback to in-memory store: %v", err)
		return newInMemoryCheckpointStore()
	}
	return &redisCheckpointStore{
		client:    client,
		keyPrefix: g.Cfg().MustGet(ctx, "assistant.checkpoint.keyPrefix", "assistant:adk:checkpoint:").String(),
		ttl:       time.Duration(g.Cfg().MustGet(ctx, "assistant.checkpoint.ttlHours", 24).Int()) * time.Hour,
	}
}

func newInMemoryCheckpointStore() checkpointStore {
	return &inMemoryCheckPointStore{data: make(map[string][]byte)}
}

func (s *inMemoryCheckPointStore) Get(_ context.Context, checkPointID string) ([]byte, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[checkPointID]
	if !ok {
		return nil, false, nil
	}
	cp := make([]byte, len(v))
	copy(cp, v)
	return cp, true, nil
}

func (s *inMemoryCheckPointStore) Set(_ context.Context, checkPointID string, checkPoint []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(checkPoint))
	copy(cp, checkPoint)
	s.data[checkPointID] = cp
	return nil
}

func (s *inMemoryCheckPointStore) Delete(_ context.Context, checkPointID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, checkPointID)
	return nil
}

type redisCheckpointStore struct {
	client    *redis.Client
	keyPrefix string
	ttl       time.Duration
}

func (s *redisCheckpointStore) Get(ctx context.Context, checkPointID string) ([]byte, bool, error) {
	key := s.keyPrefix + checkPointID
	val, err := s.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return val, true, nil
}

func (s *redisCheckpointStore) Set(ctx context.Context, checkPointID string, checkPoint []byte) error {
	key := s.keyPrefix + checkPointID
	return s.client.Set(ctx, key, checkPoint, s.ttl).Err()
}

func (s *redisCheckpointStore) Delete(ctx context.Context, checkPointID string) error {
	key := s.keyPrefix + checkPointID
	return s.client.Del(ctx, key).Err()
}
