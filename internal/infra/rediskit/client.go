package rediskit

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/redis/go-redis/v9"
)

// MustClient 返回可用的 Redis 客户端。
// 当前统一读取 agent.redis.*，因为 checkpoint、embedding cache、signal 聚合都共用同一个 Redis。
// 当 Redis 未配置或不可用时，直接终止进程。
func MustClient(ctx context.Context) *redis.Client {
	redisAddr := strings.TrimSpace(g.Cfg().MustGet(ctx, "agent.redis.addr").String())
	if redisAddr == "" {
		g.Log().Fatal(ctx, "Redis is required: agent.redis.addr is empty")
	}
	client := newClient(ctx, redisAddr)
	if err := pingClient(ctx, client); err != nil {
		_ = client.Close()
		g.Log().Fatalf(ctx, "Redis is required but unavailable: addr=%s err=%v", redisAddr, err)
	}
	return client
}

func newClient(ctx context.Context, redisAddr string) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: g.Cfg().MustGet(ctx, "agent.redis.password").String(),
		DB:       g.Cfg().MustGet(ctx, "agent.redis.db", 0).Int(),
	})
}

func pingClient(ctx context.Context, client *redis.Client) error {
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return client.Ping(pingCtx).Err()
}
