// Package coreadapter is an OPTIONAL, READ-ONLY example bridge that maps memory
// gateway recall hits into llm-agent's core memory.SearchResult values, for use
// in llm-agent's context builder.
//
// # Why this lives in its own module
//
// The llm-agent-memory-client module must NEVER import llm-agent (north star:
// the client is a transport-only gateway SDK with no dependency on the core
// agent framework). This adapter deliberately sits in a separate nested module
// — examples/coreadapter — with its own go.mod so that importing both the
// client AND llm-agent stays out of the client module's dependency graph. The
// nested module is intentionally NOT part of the umbrella go.work.
//
// # Why this is NOT a memory.Memory implementation
//
// memory.Memory's Type() returns a single memory.Kind and Stats() returns a
// single Stats — neither can express a UNIFIED gateway recall that spans
// multiple memory kinds. memory.Memory is also a read/write contract
// (Add/Get/Update/Remove); none of those make sense on a remote, read-only
// recall view. So this bridge exposes ONLY a Search method and is not a Memory.
//
// # Lossy, read-only mapping
//
// The bridge is a thin, lossy projection. It performs NO local caching and NO
// re-ranking — consistency and staleness are passed through to the gateway via
// SearchOptions. Each gateway RecallHit is mapped to memory.SearchResult using
// ONLY the fields llm-agent's context builder actually consumes:
//
//	RecallHit.Content  -> SearchResult.Item.Content
//	RecallHit.MemoryID -> SearchResult.Item.ID
//	RecallHit.Tags     -> SearchResult.Item.Tags
//	RecallHit.Score    -> SearchResult.Score
//
// RecallHit carries NO CreatedAt, so SearchResult.Item.CreatedAt is left as the
// zero time.Time. The context builder treats a zero timestamp as "fresh"
// (recency score 1.0), which is the sane default for a remote recall view.
// Everything else on MemoryItem (Importance, AccessedAt, Metadata) is left at
// its zero value because the consumer does not read it.
package coreadapter

import (
	"context"

	memclient "github.com/costa92/llm-agent-memory-client"
	"github.com/costa92/llm-agent/memory"
)

// RecallSearchBridge is a READ-ONLY recall bridge: it turns gateway recall
// results into core memory.SearchResult values for llm-agent's context builder.
// It is deliberately NOT a memory.Memory implementation — a unified gateway
// recall spans multiple memory kinds and memory.Memory's Type()/Stats() cannot
// express that, and Add/Get/Update/Remove have no place on a remote recall view.
type RecallSearchBridge struct {
	client *memclient.GatewayMemory
}

// NewRecallSearchBridge wraps a configured gateway client in a read-only bridge.
func NewRecallSearchBridge(c *memclient.GatewayMemory) *RecallSearchBridge {
	return &RecallSearchBridge{client: c}
}

// SearchOption configures a single Search call. The options map directly onto
// the client's RecallRequest fields; the bridge adds no behavior of its own.
type SearchOption func(*memclient.RecallRequest)

// WithTopK sets the maximum number of recall hits to request.
func WithTopK(k int) SearchOption {
	return func(r *memclient.RecallRequest) { r.TopK = k }
}

// WithConsistencyLevel passes a read-consistency level through to the gateway.
func WithConsistencyLevel(level memclient.ConsistencyLevel) SearchOption {
	return func(r *memclient.RecallRequest) { r.ConsistencyLevel = level }
}

// WithAllowStaleCache lets the gateway serve a stale cached recall.
func WithAllowStaleCache(allow bool) SearchOption {
	return func(r *memclient.RecallRequest) { r.AllowStaleCache = allow }
}

// Search recalls from the gateway and maps hits to core SearchResults.
// Consistency/staleness are passed through via the options; the bridge does no
// local caching or re-ranking. The returned slice is always non-nil; it is
// empty when the gateway returns no hits.
func (b *RecallSearchBridge) Search(ctx context.Context, query string, opts ...SearchOption) ([]memory.SearchResult, error) {
	req := memclient.RecallRequest{Query: query}
	for _, opt := range opts {
		opt(&req)
	}

	resp, err := b.client.Recall(ctx, req)
	if err != nil {
		return nil, err
	}

	results := make([]memory.SearchResult, 0, len(resp.Hits))
	for _, hit := range resp.Hits {
		results = append(results, memory.SearchResult{
			Item: memory.MemoryItem{
				ID:      hit.MemoryID,
				Content: hit.Content,
				Tags:    hit.Tags,
				// CreatedAt: RecallHit carries no timestamp; left zero
				// (the context builder treats zero as "fresh").
			},
			Score: hit.Score,
		})
	}
	return results, nil
}
