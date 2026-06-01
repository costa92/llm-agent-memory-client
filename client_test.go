package memoryclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestClient spins up a fake gateway running handler and returns a client
// pointed at it, configured with tenant/user (and any extra options).
func newTestClient(t *testing.T, handler http.HandlerFunc, extra ...Option) (*GatewayMemory, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	opts := append([]Option{
		WithBaseURL(srv.URL),
		WithTenant("tenant-1"),
		WithUser("user-1"),
	}, extra...)

	c, err := New(opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c, srv
}

func TestNewRequiresBaseURLTenantUser(t *testing.T) {
	cases := []struct {
		name string
		opts []Option
	}{
		{"missing all", nil},
		{"missing baseURL", []Option{WithTenant("t"), WithUser("u")}},
		{"missing tenant", []Option{WithBaseURL("http://x"), WithUser("u")}},
		{"missing user", []Option{WithBaseURL("http://x"), WithTenant("t")}},
		{"blank tenant", []Option{WithBaseURL("http://x"), WithTenant("  "), WithUser("u")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.opts...); err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}

func TestNewSucceedsWithRequired(t *testing.T) {
	c, err := New(WithBaseURL("http://x/"), WithTenant("t"), WithUser("u"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.baseURL != "http://x" {
		t.Errorf("baseURL trailing slash not trimmed: %q", c.baseURL)
	}
	if c.httpClient == nil || c.httpClient.Timeout == 0 {
		t.Errorf("expected default http client with timeout")
	}
	if c.userAgent != defaultUserAgent {
		t.Errorf("expected default user agent, got %q", c.userAgent)
	}
}

func TestDoSetsRequiredHeadersAndDecodes(t *testing.T) {
	var got *http.Request
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	},
		WithProject("proj-1"),
		WithSession("sess-1"),
		WithUserAgent("my-agent/1.0"),
		WithCredential(StaticCredential(map[string]string{"Authorization": "Bearer tok"})),
	)

	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.do(context.Background(), "POST", "/memory/write", map[string]string{"a": "b"}, &out); err != nil {
		t.Fatalf("do: %v", err)
	}
	if !out.OK {
		t.Fatalf("response not decoded")
	}

	checks := map[string]string{
		"X-Tenant-Id":   "tenant-1",
		"X-User-Id":     "user-1",
		"X-Project-Id":  "proj-1",
		"X-Session-Id":  "sess-1",
		"Content-Type":  "application/json",
		"Accept":        "application/json",
		"User-Agent":    "my-agent/1.0",
		"Authorization": "Bearer tok",
	}
	for k, want := range checks {
		if g := got.Header.Get(k); g != want {
			t.Errorf("header %s = %q, want %q", k, g, want)
		}
	}
}

func TestDoOmitsOptionalScopeHeadersWhenUnset(t *testing.T) {
	var got *http.Request
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r
		_, _ = w.Write([]byte(`{}`))
	})

	if err := c.do(context.Background(), "POST", "/x", map[string]string{}, nil); err != nil {
		t.Fatalf("do: %v", err)
	}
	if _, ok := got.Header["X-Project-Id"]; ok {
		t.Errorf("X-Project-Id should be absent when unset")
	}
	if _, ok := got.Header["X-Session-Id"]; ok {
		t.Errorf("X-Session-Id should be absent when unset")
	}
}

func TestDoNoContentTypeWhenNoBody(t *testing.T) {
	var got *http.Request
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r
		_, _ = w.Write([]byte(`{}`))
	})
	if err := c.do(context.Background(), "GET", "/x", nil, nil); err != nil {
		t.Fatalf("do: %v", err)
	}
	if _, ok := got.Header["Content-Type"]; ok {
		t.Errorf("Content-Type should be absent when no body")
	}
}

func TestDoTransportErrorIsRetryableNonGateway(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {})
	srv.Close() // force a connection failure

	err := c.do(context.Background(), "POST", "/x", map[string]string{}, nil)
	if err == nil {
		t.Fatal("expected transport error")
	}
	var ge *GatewayError
	if errors.As(err, &ge) {
		t.Fatalf("transport error should not be a GatewayError, got %T", err)
	}
	var te *TransportError
	if !errors.As(err, &te) {
		t.Fatalf("expected *TransportError, got %T", err)
	}
	if !te.Retryable() {
		t.Errorf("transport error should be retryable")
	}
}

func TestStaticCredentialCopiesInput(t *testing.T) {
	src := map[string]string{"K": "v1"}
	cp := StaticCredential(src)
	src["K"] = "mutated"
	h, err := cp.AuthHeaders(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if h["K"] != "v1" {
		t.Errorf("StaticCredential did not copy input: %q", h["K"])
	}
}

// helper to decode a request body in tests.
func decodeBody[T any](t *testing.T, r *http.Request) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return v
}
