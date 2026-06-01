package memoryclient

import (
	"context"
	"net/http"
	"testing"
)

func TestRecallMarshalsRequestAndDecodesResponse(t *testing.T) {
	var body map[string]any
	var path, ct string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		ct = r.Header.Get("Content-Type")
		body = decodeBody[map[string]any](t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"hits": [
				{"memory_id":"m1","kind":"fact","score":0.9,"version":3,"content":"hi",
				 "source":"chat","category":"general","pinned":true,"disabled":false,
				 "metadata":{"matched_by":"vector","token_cost_estimate":12}}
			],
			"trace": {"cache_level":"l1","consistency_level":"bounded","stale_served":false,
			          "memory_token_budget":100,"returned_token_estimate":12}
		}`))
	},
		WithProject("proj-1"),
	)

	resp, err := c.Recall(context.Background(), RecallRequest{
		Query:            "who am I",
		TopK:             5,
		ConsistencyLevel: ConsistencyBounded,
		AllowStaleCache:  true,
		Debug:            true,
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}

	if path != "/memory/recall/unified" {
		t.Errorf("path = %q", path)
	}
	if ct != "application/json" {
		t.Errorf("Content-Type = %q (must be exact)", ct)
	}

	// request fidelity
	if body["query"] != "who am I" {
		t.Errorf("query = %v", body["query"])
	}
	if body["consistency_level"] != "bounded" {
		t.Errorf("consistency_level passthrough = %v", body["consistency_level"])
	}
	if body["allow_stale_cache"] != true {
		t.Errorf("allow_stale_cache = %v", body["allow_stale_cache"])
	}
	if body["debug"] != true {
		t.Errorf("debug = %v", body["debug"])
	}
	scope, ok := body["scope"].(map[string]any)
	if !ok {
		t.Fatalf("scope missing or wrong type: %v", body["scope"])
	}
	if scope["tenant_id"] != "tenant-1" || scope["user_id"] != "user-1" || scope["project_id"] != "proj-1" {
		t.Errorf("scope not filled from client config: %v", scope)
	}

	// response fidelity
	if len(resp.Hits) != 1 {
		t.Fatalf("hits = %d", len(resp.Hits))
	}
	h := resp.Hits[0]
	if h.MemoryID != "m1" || h.Score != 0.9 || h.Version != 3 || !h.Pinned {
		t.Errorf("hit decode mismatch: %+v", h)
	}
	if h.Metadata.MatchedBy != "vector" || h.Metadata.TokenCostEstimate != 12 {
		t.Errorf("metadata decode mismatch: %+v", h.Metadata)
	}
	if resp.Trace == nil || resp.Trace.CacheLevel != "l1" || resp.Trace.ConsistencyLevel != "bounded" {
		t.Errorf("trace decode mismatch: %+v", resp.Trace)
	}
}

func TestRecallDefaultConsistencyOmitted(t *testing.T) {
	var body map[string]any
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body = decodeBody[map[string]any](t, r)
		_, _ = w.Write([]byte(`{"hits":[]}`))
	})

	if _, err := c.Recall(context.Background(), RecallRequest{Query: "q"}); err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if _, ok := body["consistency_level"]; ok {
		t.Errorf("consistency_level should be omitted when default")
	}
	if _, ok := body["allow_stale_cache"]; ok {
		t.Errorf("allow_stale_cache should be omitted when false")
	}
}
