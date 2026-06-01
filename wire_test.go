package memoryclient

import (
	"encoding/json"
	"testing"
)

// TestRecallRequestGoldenJSON pins the exact wire bytes for a fully-populated
// recall request so a future json-tag change is caught.
func TestRecallRequestGoldenJSON(t *testing.T) {
	req := RecallRequest{
		Scope: ScopePayload{
			TenantID:  "t1",
			UserID:    "u1",
			ProjectID: "p1",
			SessionID: "s1",
		},
		Query:             "hello",
		TopK:              5,
		TokenBudget:       1000,
		MemoryTokenBudget: 200,
		ConsistencyLevel:  ConsistencyBounded,
		AllowStaleCache:   true,
		Debug:             true,
	}
	want := `{"scope":{"tenant_id":"t1","user_id":"u1","project_id":"p1","session_id":"s1"},"query":"hello","top_k":5,"token_budget":1000,"memory_token_budget":200,"consistency_level":"bounded","allow_stale_cache":true,"debug":true}`
	assertJSON(t, req, want)
}

func TestWriteRequestGoldenJSON(t *testing.T) {
	req := WriteRequest{
		IdempotencyKey: "idem-1",
		Scope: ScopePayload{
			TenantID:  "t1",
			UserID:    "u1",
			ProjectID: "p1",
			SessionID: "s1",
		},
		Record: WriteRecordPayload{
			Kind:       "fact",
			Source:     "chat",
			Category:   "general",
			Content:    "remember this",
			Tags:       []string{"a", "b"},
			Importance: 0.7,
			Pinned:     true,
		},
	}
	want := `{"idempotency_key":"idem-1","scope":{"tenant_id":"t1","user_id":"u1","project_id":"p1","session_id":"s1"},"record":{"kind":"fact","source":"chat","category":"general","content":"remember this","tags":["a","b"],"importance":0.7,"pinned":true}}`
	assertJSON(t, req, want)
}

func TestResponseGoldenDecode(t *testing.T) {
	// Decode-then-marshal a response to pin the response-side tags.
	in := `{"hits":[{"memory_id":"m1","kind":"fact","score":0.5,"version":2,"content":"c","tags":["x"],"source":"chat","category":"gen","pinned":false,"disabled":true,"metadata":{"matched_by":"keyword","token_cost_estimate":3}}],"trace":{"cache_level":"l2","consistency_level":"eventual","stale_served":true,"memory_token_budget":50,"returned_token_estimate":3}}`
	var resp RecallResponse
	if err := json.Unmarshal([]byte(in), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(out) != in {
		t.Errorf("round-trip mismatch:\n got: %s\nwant: %s", out, in)
	}
}

func assertJSON(t *testing.T, v any, want string) {
	t.Helper()
	got, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != want {
		t.Errorf("json mismatch:\n got: %s\nwant: %s", got, want)
	}
}
