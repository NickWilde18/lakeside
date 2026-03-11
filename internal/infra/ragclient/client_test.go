package ragclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClientRetrieve(t *testing.T) {
	t.Parallel()

	var got struct {
		UserUPN string
		KBID    string
		Query   string
		TopK    int
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/retrieve", r.URL.Path)
		got.UserUPN = r.Header.Get("x-user-upn")
		got.KBID = r.Header.Get("x-kb-id")
		defer r.Body.Close()
		var body struct {
			Query string `json:"query"`
			TopK  int    `json:"top_k"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		got.Query = body.Query
		got.TopK = body.TopK
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[{"score":0.91,"text":"VPN 使用说明","metadata":{"node_id":"node-1","doc_id":"doc-1","namespace_uuid":"ns-1"}}]}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, Timeout: 2 * time.Second})
	nodes, err := client.Retrieve(context.Background(), RetrieveRequest{
		UserUPN: "alice@example.edu",
		KBID:    "kb-it",
		Query:   "怎么用 VPN",
		TopK:    3,
	})
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	require.Equal(t, "alice@example.edu", got.UserUPN)
	require.Equal(t, "kb-it", got.KBID)
	require.Equal(t, "怎么用 VPN", got.Query)
	require.Equal(t, 3, got.TopK)
	require.Equal(t, "node-1", nodes[0].NodeID)
	require.Equal(t, "doc-1", nodes[0].DocID)
	require.Equal(t, "kb-it", nodes[0].KBID)
}

func TestClientBatchGetDocuments(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/documents/batch_get", r.URL.Path)
		require.Equal(t, "alice@example.edu", r.Header.Get("x-user-upn"))
		require.Equal(t, "kb-it", r.Header.Get("x-kb-id"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"documents":[{"id":"doc-1","filename":"vpn-user-guide.md","mime_type":"text/markdown"}]}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, Timeout: 2 * time.Second})
	docs, err := client.BatchGetDocuments(context.Background(), BatchGetDocumentsRequest{
		UserUPN: "alice@example.edu",
		KBID:    "kb-it",
		DocIDs:  []string{"doc-1"},
	})
	require.NoError(t, err)
	require.Len(t, docs, 1)
	require.Equal(t, "vpn-user-guide.md", docs[0].Filename)
}

func TestClientRetrieveReturnsAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"INVALID_KB_ID","message":"kb_id not found"}}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, Timeout: 2 * time.Second})
	_, err := client.Retrieve(context.Background(), RetrieveRequest{
		UserUPN: "alice@example.edu",
		KBID:    "kb-it",
		Query:   "test",
	})
	require.Error(t, err)
	apiErr, ok := err.(*APIError)
	require.True(t, ok)
	require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	require.Equal(t, "INVALID_KB_ID", apiErr.Code)
}
