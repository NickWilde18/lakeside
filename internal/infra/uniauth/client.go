package uniauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gogf/gf/v2/net/gclient"
)

// Config 定义 UniAuth 客户端配置。
type Config struct {
	BaseURL string
	Timeout time.Duration
}

// UserInfo 表示 UniAuth 返回的用户信息。
type UserInfo struct {
	UPN        string `json:"upn"`
	EmployeeID string `json:"employeeId"`
	Name       string `json:"name"`
	Department string `json:"department"`
}

// APIError 表示 UniAuth 返回的 HTTP 错误。
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("uniauth request failed: status=%d message=%s", e.StatusCode, e.Message)
}

// Client 负责调用 UniAuth 查询用户信息。
type Client struct {
	baseURL    string
	httpClient *gclient.Client
}

// NewClient 创建 UniAuth 客户端。
func NewClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{
		baseURL:    strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		httpClient: gclient.New().Timeout(timeout).ContentJson(),
	}
}

// GetUserInfo 按 UPN 查询 UniAuth 用户信息。
func (c *Client) GetUserInfo(ctx context.Context, upn string) (*UserInfo, error) {
	if c == nil || c.httpClient == nil {
		return nil, fmt.Errorf("uniauth client is nil")
	}
	if strings.TrimSpace(c.baseURL) == "" {
		return nil, fmt.Errorf("uniauth base url is empty")
	}
	upn = strings.TrimSpace(upn)
	if upn == "" {
		return nil, fmt.Errorf("uniauth upn is empty")
	}

	endpoint := c.baseURL + "/userinfos?upn=" + url.QueryEscape(upn)
	resp, err := c.httpClient.Get(ctx, endpoint)
	if err != nil {
		return nil, fmt.Errorf("call uniauth failed: %w", err)
	}
	defer resp.Close()

	body := resp.ReadAll()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{StatusCode: resp.StatusCode, Message: strings.TrimSpace(string(body))}
	}

	var user UserInfo
	if err := json.Unmarshal(body, &user); err == nil && (strings.TrimSpace(user.EmployeeID) != "" || strings.TrimSpace(user.UPN) != "") {
		return &user, nil
	}
	var wrapped struct {
		Data *UserInfo `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, fmt.Errorf("decode uniauth response failed: %w", err)
	}
	if wrapped.Data == nil {
		return nil, fmt.Errorf("decode uniauth response failed: missing data field")
	}
	return wrapped.Data, nil
}

// ResolveEmployeeID 只抽取 UniAuth 结果里的 employeeId。
func (c *Client) ResolveEmployeeID(ctx context.Context, upn string) (string, error) {
	info, err := c.GetUserInfo(ctx, upn)
	if err != nil {
		return "", err
	}
	employeeID := strings.TrimSpace(info.EmployeeID)
	if employeeID == "" {
		return "", fmt.Errorf("employeeId is empty for upn=%s", strings.TrimSpace(upn))
	}
	return employeeID, nil
}
