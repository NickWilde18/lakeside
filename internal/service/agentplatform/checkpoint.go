package agentplatform

import (
	"context"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/redis/go-redis/v9"

	"lakeside/internal/consts"
	"lakeside/internal/infra/rediskit"
)

type checkpointStore interface {
	adk.CheckPointStore
	Delete(ctx context.Context, checkPointID string) error
}

type redisCheckpointStore struct {
	client    *redis.Client
	keyPrefix string
	ttl       time.Duration
}

func newCheckpointStore(ctx context.Context, assistantKey string, ttl time.Duration) checkpointStore {
	return &redisCheckpointStore{
		client:    rediskit.MustClient(ctx),
		keyPrefix: consts.RootAssistantCheckpointPrefix + assistantKey + ":",
		ttl:       ttl,
	}
}

func (s *redisCheckpointStore) Get(ctx context.Context, checkPointID string) ([]byte, bool, error) {
	val, err := s.client.Get(ctx, s.keyPrefix+checkPointID).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return val, true, nil
}

func (s *redisCheckpointStore) Set(ctx context.Context, checkPointID string, checkPoint []byte) error {
	return s.client.Set(ctx, s.keyPrefix+checkPointID, checkPoint, s.ttl).Err()
}

func (s *redisCheckpointStore) Delete(ctx context.Context, checkPointID string) error {
	return s.client.Del(ctx, s.keyPrefix+checkPointID).Err()
}
