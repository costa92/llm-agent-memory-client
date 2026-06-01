package memoryclient

import (
	"context"
	"net/http"
	"testing"
)

func TestWriteAutoGeneratesIdempotencyKey(t *testing.T) {
	var body WriteRequest
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body = decodeBody[WriteRequest](t, r)
		if r.URL.Path != "/memory/write" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"memory":{"memory_id":"m9","version":1,"status":"created"}}`))
	})

	res, err := c.Write(context.Background(), WriteRequest{
		Record: WriteRecordPayload{Kind: "fact", Source: "chat", Category: "general", Content: "x"},
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if body.IdempotencyKey == "" {
		t.Fatalf("idempotency_key should be auto-generated, was empty")
	}
	if len(body.IdempotencyKey) != 32 {
		t.Errorf("expected 32-hex-char key, got %d chars: %q", len(body.IdempotencyKey), body.IdempotencyKey)
	}
	if body.Scope.TenantID != "tenant-1" || body.Scope.UserID != "user-1" {
		t.Errorf("scope not filled: %+v", body.Scope)
	}
	if res.MemoryID != "m9" || res.Version != 1 || res.Status != "created" {
		t.Errorf("result decode mismatch: %+v", res)
	}
}

func TestWriteGeneratesUniqueKeys(t *testing.T) {
	keys := map[string]bool{}
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		b := decodeBody[WriteRequest](t, r)
		keys[b.IdempotencyKey] = true
		_, _ = w.Write([]byte(`{"memory":{"memory_id":"m","version":1,"status":"created"}}`))
	})
	for i := 0; i < 3; i++ {
		if _, err := c.Write(context.Background(), WriteRequest{Record: WriteRecordPayload{Content: "x"}}); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	if len(keys) != 3 {
		t.Errorf("expected 3 unique keys, got %d", len(keys))
	}
}

func TestWritePassesThroughCallerKey(t *testing.T) {
	var body WriteRequest
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body = decodeBody[WriteRequest](t, r)
		_, _ = w.Write([]byte(`{"memory":{"memory_id":"m","version":2,"status":"updated"}}`))
	})

	if _, err := c.Write(context.Background(), WriteRequest{
		IdempotencyKey: "caller-key-123",
		Record:         WriteRecordPayload{Content: "x"},
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if body.IdempotencyKey != "caller-key-123" {
		t.Errorf("caller key not passed through: %q", body.IdempotencyKey)
	}
}
