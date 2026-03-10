package itsmagent

import (
	"context"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/redis/go-redis/v9"
)

type checkpointStore interface {
	adk.CheckPointStore
	Delete(ctx context.Context, checkPointID string) error
}

type idempotencyStore interface {
	Get(ctx context.Context, key string) (string, bool, error)
	SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error)
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

type redisIdempotencyStore struct {
	client *redis.Client
}

func (s *redisIdempotencyStore) Get(ctx context.Context, key string) (string, bool, error) {
	val, err := s.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return val, true, nil
}

func (s *redisIdempotencyStore) SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error) {
	ok, err := s.client.SetNX(ctx, key, value, ttl).Result()
	if err != nil {
		return false, err
	}
	return ok, nil
}
