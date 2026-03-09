package itsmclient

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/gclient"
)

type TicketPayload struct {
	UserCode     string
	Subject      string
	ServiceLevel string
	Priority     string
	OthersDesc   string
}

type CreateResult struct {
	Success   bool   `json:"success"`
	Code      int    `json:"code"`
	TicketNo  string `json:"ticket_no,omitempty"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp,omitempty"`
}

type Config struct {
	BaseURL     string
	AppSecret   string
	Timeout     time.Duration
	MaxAttempts int
	Backoffs    []time.Duration
}

type Client struct {
	baseURL     string
	appSecret   string
	httpClient  *gclient.Client
	maxAttempts int
	backoffs    []time.Duration
}

func NewClient(cfg Config) *Client {
	maxAttempts := cfg.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	backoffs := cfg.Backoffs
	if len(backoffs) == 0 {
		backoffs = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	// 使用 gclient 统一接入 GoFrame 的客户端能力（包含内置观测链路）。
	httpClient := gclient.New().Timeout(timeout).ContentJson()
	return &Client{
		baseURL:     strings.TrimRight(cfg.BaseURL, "/"),
		appSecret:   cfg.AppSecret,
		httpClient:  httpClient,
		maxAttempts: maxAttempts,
		backoffs:    backoffs,
	}
}

func (c *Client) CreateTicket(ctx context.Context, p TicketPayload) (*CreateResult, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("itsm base url is empty")
	}
	if c.appSecret == "" {
		return nil, fmt.Errorf("itsm appSecret is empty")
	}

	var lastErr error
	// 仅对可重试错误做退避重试（网络抖动、服务端 5xx），避免把参数错误无限放大。
	for attempt := 1; attempt <= c.maxAttempts; attempt++ {
		result, retryable, err := c.createTicketOnce(ctx, p)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !retryable || attempt == c.maxAttempts {
			break
		}
		delay := c.backoffForAttempt(attempt)
		if sleepErr := sleepWithContext(ctx, delay); sleepErr != nil {
			return nil, sleepErr
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("create ticket failed")
	}
	return nil, lastErr
}

func (c *Client) createTicketOnce(ctx context.Context, p TicketPayload) (*CreateResult, bool, error) {
	nowMs := time.Now().UnixMilli()
	token := md5Token(nowMs, c.appSecret)

	reqBody := g.Map{
		"token": token,
		"time":  nowMs,
		"data": g.Map{
			"userCode":     p.UserCode,
			"subject":      p.Subject,
			"serviceLevel": p.ServiceLevel,
			"priority":     p.Priority,
			"othersDesc":   p.OthersDesc,
		},
	}
	resp, err := c.httpClient.Post(ctx, c.baseURL, reqBody)
	if err != nil {
		// 简化策略：仅超时类错误重试，其余错误直接失败。
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, true, fmt.Errorf("itsm request retryable error: %w", err)
		}
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return nil, true, fmt.Errorf("itsm request retryable error: %w", err)
		}
		return nil, false, fmt.Errorf("itsm request failed: %w", err)
	}
	defer resp.Close()

	body := resp.ReadAll()

	if resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("itsm server error status=%d body=%s", resp.StatusCode, string(body))
	}
	if resp.StatusCode >= 400 {
		return nil, false, fmt.Errorf("itsm client error status=%d body=%s", resp.StatusCode, string(body))
	}

	var raw struct {
		Code      int    `json:"code"`
		Success   bool   `json:"success"`
		Obj       string `json:"obj"`
		Message   string `json:"message"`
		Timestamp int64  `json:"timestamp"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, false, fmt.Errorf("decode itsm response failed: %w body=%s", err, string(body))
	}

	result := &CreateResult{
		Success:   raw.Success,
		Code:      raw.Code,
		TicketNo:  strings.TrimSpace(raw.Obj),
		Message:   strings.TrimSpace(raw.Message),
		Timestamp: raw.Timestamp,
	}

	// 业务失败通常是字段或业务规则问题，不做自动重试，交由上层给用户明确反馈。
	if !raw.Success {
		return nil, false, fmt.Errorf("itsm business failed: %s", result.Message)
	}
	return result, false, nil
}

func (c *Client) backoffForAttempt(attempt int) time.Duration {
	idx := attempt - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(c.backoffs) {
		idx = len(c.backoffs) - 1
	}
	if idx < 0 {
		return time.Second
	}
	return c.backoffs[idx]
}

func md5Token(nowMs int64, appSecret string) string {
	// 按 ITSM 接口约定计算 token：md5(time + appSecret)。
	h := md5.Sum([]byte(fmt.Sprintf("%d%s", nowMs, appSecret)))
	return hex.EncodeToString(h[:])
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
