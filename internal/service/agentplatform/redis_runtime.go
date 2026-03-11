package agentplatform

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/redis/go-redis/v9"

	"lakeside/internal/infra/rediskit"
)

type redisRuntime struct {
	client         *redis.Client
	streamKey      string
	streamGroup    string
	consumerPrefix string
	readBlock      time.Duration
	pubsubChannel  string
	cancelChannel  string
}

func newRedisRuntime(ctx context.Context, cfg *config) *redisRuntime {
	if cfg == nil {
		return nil
	}
	client := rediskit.MustClient(ctx)
	if client == nil {
		return nil
	}
	blockMs := cfg.Runtime.ReadBlockMs
	if blockMs <= 0 {
		blockMs = 5000
	}
	pubsubChannel := strings.TrimSpace(cfg.Runtime.PubSubChannel)
	if pubsubChannel == "" {
		pubsubChannel = "lakeside:agent:events:v1"
	}
	return &redisRuntime{
		client:         client,
		streamKey:      strings.TrimSpace(cfg.Runtime.StreamKey),
		streamGroup:    strings.TrimSpace(cfg.Runtime.StreamGroup),
		consumerPrefix: strings.TrimSpace(cfg.Runtime.ConsumerPrefix),
		readBlock:      time.Duration(blockMs) * time.Millisecond,
		pubsubChannel:  pubsubChannel,
		cancelChannel:  pubsubChannel + ":cancel",
	}
}

func (r *redisRuntime) ensureConsumerGroup(ctx context.Context) error {
	if r == nil || r.client == nil {
		return errors.New("redis runtime unavailable")
	}
	if r.streamKey == "" || r.streamGroup == "" {
		return errors.New("stream key/group is empty")
	}
	err := r.client.XGroupCreateMkStream(ctx, r.streamKey, r.streamGroup, "$").Err()
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToUpper(err.Error()), "BUSYGROUP") {
		return nil
	}
	return err
}

func (r *redisRuntime) enqueueRun(ctx context.Context, runID string) error {
	if r == nil || r.client == nil {
		return errors.New("redis runtime unavailable")
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return errors.New("run_id is empty")
	}
	_, err := r.client.XAdd(ctx, &redis.XAddArgs{
		Stream: r.streamKey,
		Values: map[string]any{
			"run_id": runID,
		},
	}).Result()
	return err
}

func (r *redisRuntime) consumeRuns(ctx context.Context, consumerName string, batchSize int64, handler func(context.Context, string) error) error {
	if r == nil || r.client == nil {
		return errors.New("redis runtime unavailable")
	}
	if strings.TrimSpace(consumerName) == "" {
		return errors.New("consumer name is empty")
	}
	if batchSize <= 0 {
		batchSize = 16
	}
	minIdle := r.pendingMinIdle()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := r.reclaimPendingRuns(ctx, consumerName, batchSize, minIdle, handler); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			g.Log().Warningf(ctx, "redis stream reclaim pending failed, consumer=%s err=%v", consumerName, err)
		}
		streams, err := r.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    r.streamGroup,
			Consumer: consumerName,
			Streams:  []string{r.streamKey, ">"},
			Count:    batchSize,
			Block:    r.readBlock,
			NoAck:    false,
		}).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}
		for _, stream := range streams {
			for _, msg := range stream.Messages {
				if err := r.processRunMessage(ctx, msg, handler); err != nil {
					g.Log().Warningf(ctx, "redis stream process message failed, stream=%s message_id=%s err=%v", stream.Stream, msg.ID, err)
				}
			}
		}
	}
}

func (r *redisRuntime) pendingMinIdle() time.Duration {
	minIdle := r.readBlock * 3
	if minIdle < 15*time.Second {
		minIdle = 15 * time.Second
	}
	return minIdle
}

func (r *redisRuntime) reclaimPendingRuns(ctx context.Context, consumerName string, batchSize int64, minIdle time.Duration, handler func(context.Context, string) error) error {
	if r == nil || r.client == nil {
		return errors.New("redis runtime unavailable")
	}
	if minIdle <= 0 {
		minIdle = 15 * time.Second
	}
	msgs, _, err := r.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   r.streamKey,
		Group:    r.streamGroup,
		Consumer: consumerName,
		MinIdle:  minIdle,
		Start:    "0-0",
		Count:    batchSize,
	}).Result()
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unknown command") {
			return nil
		}
		return err
	}
	for _, msg := range msgs {
		if err := r.processRunMessage(ctx, msg, handler); err != nil {
			g.Log().Warningf(ctx, "redis stream process reclaimed message failed, message_id=%s err=%v", msg.ID, err)
		}
	}
	return nil
}

func (r *redisRuntime) processRunMessage(ctx context.Context, msg redis.XMessage, handler func(context.Context, string) error) error {
	if r == nil || r.client == nil {
		return errors.New("redis runtime unavailable")
	}
	runID := strings.TrimSpace(fmt.Sprintf("%v", msg.Values["run_id"]))
	if runID == "" {
		_ = r.client.XAck(ctx, r.streamKey, r.streamGroup, msg.ID).Err()
		return errors.New("run_id is empty")
	}
	if err := handler(ctx, runID); err != nil {
		// 处理失败不 ack，保持 pending 以便后续 reclaim 重试。
		return err
	}
	return r.client.XAck(ctx, r.streamKey, r.streamGroup, msg.ID).Err()
}

func (r *redisRuntime) publishRunEvent(ctx context.Context, runID string) {
	if r == nil || r.client == nil || strings.TrimSpace(r.pubsubChannel) == "" {
		return
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return
	}
	_ = r.client.Publish(ctx, r.pubsubChannel, runID).Err()
}

func (r *redisRuntime) publishCancelRequest(ctx context.Context, runID string) error {
	if r == nil || r.client == nil {
		return errors.New("redis runtime unavailable")
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return errors.New("run_id is empty")
	}
	if strings.TrimSpace(r.cancelChannel) == "" {
		return errors.New("cancel channel is empty")
	}
	return r.client.Publish(ctx, r.cancelChannel, runID).Err()
}

func (r *redisRuntime) consumeCancelRequests(ctx context.Context, handler func(string)) error {
	if r == nil || r.client == nil {
		return errors.New("redis runtime unavailable")
	}
	if strings.TrimSpace(r.cancelChannel) == "" {
		return errors.New("cancel channel is empty")
	}
	pubsub := r.client.Subscribe(ctx, r.cancelChannel)
	defer func() {
		_ = pubsub.Close()
	}()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-pubsub.Channel():
			if !ok {
				return nil
			}
			runID := strings.TrimSpace(msg.Payload)
			if runID == "" || handler == nil {
				continue
			}
			handler(runID)
		}
	}
}

func (r *redisRuntime) subscribeRunEvent(ctx context.Context, runID string) (<-chan struct{}, func(), error) {
	if r == nil || r.client == nil || strings.TrimSpace(r.pubsubChannel) == "" {
		return nil, nil, errors.New("pubsub unavailable")
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, nil, errors.New("run_id is empty")
	}
	pubsub := r.client.Subscribe(ctx, r.pubsubChannel)
	ch := make(chan struct{}, 1)
	stop := make(chan struct{})
	go func() {
		defer close(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case msg, ok := <-pubsub.Channel():
				if !ok {
					return
				}
				if strings.TrimSpace(msg.Payload) != runID {
					continue
				}
				select {
				case ch <- struct{}{}:
				default:
				}
			}
		}
	}()
	cancel := func() {
		close(stop)
		_ = pubsub.Close()
	}
	return ch, cancel, nil
}
