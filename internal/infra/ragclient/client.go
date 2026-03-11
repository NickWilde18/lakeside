package ragclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/gclient"
)

// Config 定义 RAG 服务客户端配置。
type Config struct {
	BaseURL string
	Timeout time.Duration
}

// Client 负责调用外部 RAG 服务。
type Client struct {
	baseURL    string
	httpClient *gclient.Client
}

// RetrieveRequest 表示一次检索请求。
type RetrieveRequest struct {
	UserUPN string
	KBID    string
	Query   string
	TopK    int
}

// RetrievedNode 表示 RAG 服务返回的一个命中片段。
type RetrievedNode struct {
	KBID          string
	DocID         string
	NodeID        string
	NamespaceUUID string
	Text          string
	Score         float64
}

// BatchGetDocumentsRequest 表示批量查询文档元信息请求。
type BatchGetDocumentsRequest struct {
	UserUPN string
	KBID    string
	DocIDs  []string
}

// Document 表示 RAG 服务返回的文档元信息。
type Document struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	MimeType string `json:"mime_type"`
}

// APIError 表示 RAG 服务返回的 HTTP 业务错误。
type APIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Code) == "" {
		return fmt.Sprintf("rag api request failed: status=%d message=%s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("rag api request failed: status=%d code=%s message=%s", e.StatusCode, e.Code, e.Message)
}

// NewClient 创建 RAG 服务客户端。
func NewClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Client{
		baseURL:    strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		httpClient: gclient.New().Timeout(timeout).ContentJson(),
	}
}

// Retrieve 调用 RAG 的 /v1/retrieve 接口。
func (c *Client) Retrieve(ctx context.Context, req RetrieveRequest) ([]RetrievedNode, error) {
	if c == nil || c.httpClient == nil {
		return nil, fmt.Errorf("rag client is nil")
	}
	if strings.TrimSpace(c.baseURL) == "" {
		return nil, fmt.Errorf("rag base url is empty")
	}
	if strings.TrimSpace(req.UserUPN) == "" {
		return nil, fmt.Errorf("rag retrieve missing user upn")
	}
	if strings.TrimSpace(req.KBID) == "" {
		return nil, fmt.Errorf("rag retrieve missing kb_id")
	}
	if strings.TrimSpace(req.Query) == "" {
		return nil, fmt.Errorf("rag retrieve query is empty")
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}

	resp, err := c.httpClient.Header(map[string]string{
		"x-user-upn": strings.TrimSpace(req.UserUPN),
		"x-kb-id":    strings.TrimSpace(req.KBID),
	}).Post(ctx, c.baseURL+"/v1/retrieve", g.Map{
		"query": req.Query,
		"top_k": req.TopK,
	})
	if err != nil {
		return nil, fmt.Errorf("call rag retrieve failed: %w", err)
	}
	defer resp.Close()

	body := resp.ReadAll()
	if err := parseAPIError(resp.StatusCode, body); err != nil {
		return nil, err
	}

	var raw struct {
		Nodes []struct {
			Score    float64 `json:"score"`
			Text     string  `json:"text"`
			Metadata struct {
				NodeID        string `json:"node_id"`
				DocID         string `json:"doc_id"`
				NamespaceUUID string `json:"namespace_uuid"`
			} `json:"metadata"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode rag retrieve response failed: %w", err)
	}

	out := make([]RetrievedNode, 0, len(raw.Nodes))
	for _, node := range raw.Nodes {
		out = append(out, RetrievedNode{
			KBID:          strings.TrimSpace(req.KBID),
			DocID:         strings.TrimSpace(node.Metadata.DocID),
			NodeID:        strings.TrimSpace(node.Metadata.NodeID),
			NamespaceUUID: strings.TrimSpace(node.Metadata.NamespaceUUID),
			Text:          strings.TrimSpace(node.Text),
			Score:         node.Score,
		})
	}
	return out, nil
}

// BatchGetDocuments 调用 RAG 的 /v1/documents/batch_get 接口。
func (c *Client) BatchGetDocuments(ctx context.Context, req BatchGetDocumentsRequest) ([]Document, error) {
	if c == nil || c.httpClient == nil {
		return nil, fmt.Errorf("rag client is nil")
	}
	if strings.TrimSpace(c.baseURL) == "" {
		return nil, fmt.Errorf("rag base url is empty")
	}
	if strings.TrimSpace(req.UserUPN) == "" {
		return nil, fmt.Errorf("rag batch_get missing user upn")
	}
	if strings.TrimSpace(req.KBID) == "" {
		return nil, fmt.Errorf("rag batch_get missing kb_id")
	}
	if len(req.DocIDs) == 0 {
		return nil, nil
	}

	resp, err := c.httpClient.Header(map[string]string{
		"x-user-upn": strings.TrimSpace(req.UserUPN),
		"x-kb-id":    strings.TrimSpace(req.KBID),
	}).Post(ctx, c.baseURL+"/v1/documents/batch_get", g.Map{
		"doc_ids": req.DocIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("call rag batch_get failed: %w", err)
	}
	defer resp.Close()

	body := resp.ReadAll()
	if err := parseAPIError(resp.StatusCode, body); err != nil {
		return nil, err
	}

	var raw struct {
		Documents []Document `json:"documents"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode rag batch_get response failed: %w", err)
	}
	return raw.Documents, nil
}

func parseAPIError(statusCode int, body []byte) error {
	if statusCode >= 200 && statusCode < 300 {
		return nil
	}
	var raw struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return &APIError{StatusCode: statusCode, Message: strings.TrimSpace(string(body))}
	}
	return &APIError{
		StatusCode: statusCode,
		Code:       strings.TrimSpace(raw.Error.Code),
		Message:    strings.TrimSpace(raw.Error.Message),
	}
}
