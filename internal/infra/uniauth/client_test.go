package uniauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClientResolveEmployeeID(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/userinfos", r.URL.Path)
		require.Equal(t, "alice@example.edu", r.URL.Query().Get("upn"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"upn":"alice@example.edu","employeeId":"122020255","name":"Alice"}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, Timeout: 2 * time.Second})
	employeeID, err := client.ResolveEmployeeID(context.Background(), "alice@example.edu")
	require.NoError(t, err)
	require.Equal(t, "122020255", employeeID)
}

func TestClientResolveEmployeeIDRequiresEmployeeID(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"upn":"alice@example.edu","employeeId":""}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, Timeout: 2 * time.Second})
	_, err := client.ResolveEmployeeID(context.Background(), "alice@example.edu")
	require.Error(t, err)
	require.Contains(t, err.Error(), "employeeId is empty")
}
