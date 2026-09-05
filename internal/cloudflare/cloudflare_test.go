package cloudflare

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListFlareTunnelWorkersCountsOnlyPrefix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/accounts/acc-1/workers/scripts" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok-1" {
			t.Errorf("missing bearer token")
		}
		w.Header().Set("Content-Type", "application/json")
		// 150 total workers: 80 flaretunnel-* + 70 other workers.
		items := make([]map[string]string, 0, 150)
		for i := 0; i < 80; i++ {
			items = append(items, map[string]string{"id": "flaretunnel-1-abcdef"})
		}
		for i := 0; i < 70; i++ {
			items = append(items, map[string]string{"id": "my-app-worker"})
		}
		body := `{"success":true,"result":[`
		for i, it := range items {
			if i > 0 {
				body += ","
			}
			body += `{"id":"` + it["id"] + `"}`
		}
		body += `]}`
		w.Write([]byte(body))
	}))
	defer srv.Close()

	c := New("tok-1", "acc-1")
	c.BaseURL = srv.URL

	names, err := c.ListFlareTunnelWorkers(context.Background())
	if err != nil {
		t.Fatalf("ListFlareTunnelWorkers() error = %v", err)
	}
	if len(names) != 80 {
		t.Errorf("counted %d FlareTunnel workers, want 80 (only flaretunnel-* prefix)", len(names))
	}

	n, err := c.CountFlareTunnelWorkers(context.Background())
	if err != nil {
		t.Fatalf("CountFlareTunnelWorkers() error = %v", err)
	}
	if n != 80 {
		t.Errorf("count = %d, want 80", n)
	}
}

func TestListFlareTunnelWorkersAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"success":false,"errors":[{"message":"invalid token"}]}`))
	}))
	defer srv.Close()

	c := New("bad", "acc-1")
	c.BaseURL = srv.URL

	if _, err := c.ListFlareTunnelWorkers(context.Background()); err == nil {
		t.Fatal("expected error for API failure")
	}
}

func TestListFlareTunnelWorkersSuccessFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":false,"errors":[{"message":"boom"}]}`))
	}))
	defer srv.Close()

	c := New("t", "acc-1")
	c.BaseURL = srv.URL

	if _, err := c.ListFlareTunnelWorkers(context.Background()); err == nil {
		t.Fatal("expected error when success=false")
	}
}
