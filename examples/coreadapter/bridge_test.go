package coreadapter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	memclient "github.com/costa92/llm-agent-memory-client"
)

// newBridge stands up a fake gateway running handler and returns a bridge
// wrapping a real GatewayMemory pointed at it.
func newBridge(t *testing.T, handler http.HandlerFunc) *RecallSearchBridge {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, err := memclient.New(
		memclient.WithBaseURL(srv.URL),
		memclient.WithTenant("tenant-1"),
		memclient.WithUser("user-1"),
	)
	if err != nil {
		t.Fatalf("memclient.New: %v", err)
	}
	return NewRecallSearchBridge(c)
}

func TestSearchMapsHitsToSearchResults(t *testing.T) {
	b := newBridge(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"hits": [
				{"memory_id":"m1","kind":"semantic","score":0.91,"version":3,
				 "content":"user prefers dark mode","tags":["pref","ui"],
				 "source":"chat","category":"general","pinned":false,"disabled":false,
				 "metadata":{"matched_by":"vector","token_cost_estimate":7}},
				{"memory_id":"m2","kind":"episodic","score":0.42,"version":1,
				 "content":"signed up on 2026-01-02","tags":["account"],
				 "source":"system","category":"timeline","pinned":false,"disabled":false,
				 "metadata":{}}
			]
		}`))
	})

	results, err := b.Search(context.Background(), "preferences")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}

	got0 := results[0]
	if got0.Item.ID != "m1" {
		t.Errorf("results[0].Item.ID = %q, want m1", got0.Item.ID)
	}
	if got0.Item.Content != "user prefers dark mode" {
		t.Errorf("results[0].Item.Content = %q", got0.Item.Content)
	}
	if !reflect.DeepEqual(got0.Item.Tags, []string{"pref", "ui"}) {
		t.Errorf("results[0].Item.Tags = %v", got0.Item.Tags)
	}
	if got0.Score != 0.91 {
		t.Errorf("results[0].Score = %v, want 0.91", got0.Score)
	}
	// RecallHit carries no timestamp; CreatedAt must be the zero time.
	if !got0.Item.CreatedAt.IsZero() {
		t.Errorf("results[0].Item.CreatedAt = %v, want zero", got0.Item.CreatedAt)
	}

	got1 := results[1]
	if got1.Item.ID != "m2" || got1.Score != 0.42 {
		t.Errorf("results[1] mismatch: %+v", got1)
	}
}

func TestSearchOptionsFlowIntoRecallRequest(t *testing.T) {
	var body map[string]any
	b := newBridge(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hits":[]}`))
	})

	_, err := b.Search(context.Background(), "who am I",
		WithTopK(7),
		WithConsistencyLevel(memclient.ConsistencyBounded),
		WithAllowStaleCache(true),
	)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if body["query"] != "who am I" {
		t.Errorf("query = %v", body["query"])
	}
	if got := body["top_k"]; got != float64(7) {
		t.Errorf("top_k = %v, want 7", got)
	}
	if body["consistency_level"] != "bounded" {
		t.Errorf("consistency_level = %v, want bounded", body["consistency_level"])
	}
	if body["allow_stale_cache"] != true {
		t.Errorf("allow_stale_cache = %v, want true", body["allow_stale_cache"])
	}
}

func TestSearchEmptyHitsYieldsNonNilEmptySlice(t *testing.T) {
	b := newBridge(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hits":[]}`))
	})

	results, err := b.Search(context.Background(), "q")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if results == nil {
		t.Fatal("results is nil, want non-nil empty slice")
	}
	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}
}
