package itsmagent

import (
	"context"
	"sync"
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

type inMemoryCheckPointStore struct {
	mu   sync.RWMutex
	data map[string][]byte
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

type inMemoryIdempotencyStore struct {
	mu   sync.RWMutex
	data map[string]string
}

func newInMemoryIdempotencyStore() idempotencyStore {
	return &inMemoryIdempotencyStore{data: make(map[string]string)}
}

func (s *inMemoryIdempotencyStore) Get(_ context.Context, key string) (string, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok, nil
}

func (s *inMemoryIdempotencyStore) SetNX(_ context.Context, key string, value string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[key]; ok {
		return false, nil
	}
	s.data[key] = value
	return true, nil
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
