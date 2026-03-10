package assistant

import (
	"context"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/redis/go-redis/v9"

	"lakeside/internal/consts"
	"lakeside/internal/infra/rediskit"
)

type checkpointStore interface {
	adk.CheckPointStore
	Delete(ctx context.Context, checkPointID string) error
}

func newCheckpointStore(ctx context.Context) checkpointStore {
	client := rediskit.MustClient(ctx)
	return &redisCheckpointStore{
		client:    client,
		keyPrefix: consts.AssistantCheckpointPrefix,
		ttl:       time.Duration(g.Cfg().MustGet(ctx, "assistant.checkpoint.ttlHours", 24).Int()) * time.Hour,
	}
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
