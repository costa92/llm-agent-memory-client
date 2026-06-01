package memoryclient

import (
	"context"
	"net/http"
	"testing"
)

func TestCloseSession(t *testing.T) {
	var method, path, ct string
	var body map[string]any
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		method, path, ct = r.Method, r.URL.Path, r.Header.Get("Content-Type")
		body = decodeBody[map[string]any](t, r)
		_, _ = w.Write([]byte(`{"session_id":"s1","status":"closed"}`))
	}, WithSession("s1"))

	st, err := c.CloseSession(context.Background(), "s1", "flush")
	if err != nil {
		t.Fatalf("CloseSession: %v", err)
	}
	if method != "POST" || path != "/memory/sessions/s1/close" {
		t.Errorf("method/path = %q %q", method, path)
	}
	if ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	if body["mode"] != "flush" {
		t.Errorf("mode = %v", body["mode"])
	}
	if st.SessionID != "s1" || st.Status != "closed" {
		t.Errorf("status = %+v", st)
	}
}

func TestCloseSessionEmptyModeSent(t *testing.T) {
	var body map[string]any
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body = decodeBody[map[string]any](t, r)
		_, _ = w.Write([]byte(`{"session_id":"s1","status":"closed"}`))
	})
	if _, err := c.CloseSession(context.Background(), "s1", ""); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}
	// mode has no omitempty in the gateway contract, so it is always present.
	if _, ok := body["mode"]; !ok {
		t.Errorf("mode must be present (no omitempty), body=%v", body)
	}
}

func TestHeartbeat(t *testing.T) {
	var method, path string
	var scope map[string]any
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		scope, _ = decodeBody[map[string]any](t, r)["scope"].(map[string]any)
		_, _ = w.Write([]byte(`{"session_id":"s2","status":"active"}`))
	})
	st, err := c.Heartbeat(context.Background(), "s2")
	if err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	if method != "POST" || path != "/memory/sessions/s2/heartbeat" {
		t.Errorf("method/path = %q %q", method, path)
	}
	if scope["tenant_id"] != "tenant-1" || scope["user_id"] != "user-1" {
		t.Errorf("scope = %v", scope)
	}
	if st.SessionID != "s2" || st.Status != "active" {
		t.Errorf("status = %+v", st)
	}
}

func TestSessionPathEscaping(t *testing.T) {
	var rawPath string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		rawPath = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"session_id":"x","status":"active"}`))
	})
	if _, err := c.Heartbeat(context.Background(), "sess/odd id"); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	if rawPath != "/memory/sessions/sess%2Fodd%20id/heartbeat" {
		t.Errorf("escaped wire path = %q", rawPath)
	}
}
