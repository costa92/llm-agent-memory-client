package memoryclient

// ConsistencyLevel selects the gateway's read-consistency behaviour for a
// recall. It maps to the recall request's consistency_level field.
type ConsistencyLevel string

const (
	// ConsistencyDefault leaves consistency_level unset, letting the gateway
	// apply its own default.
	ConsistencyDefault ConsistencyLevel = ""
	// ConsistencyBounded requests bounded-staleness reads.
	ConsistencyBounded ConsistencyLevel = "bounded"
	// ConsistencyEventual requests eventually-consistent reads.
	ConsistencyEventual ConsistencyLevel = "eventual"
)

// ScopePayload is the request-body scope. The gateway treats the auth-scope
// headers as authoritative and merges them over this payload, so callers do not
// normally need to populate it; the client fills it from its configured scope.
type ScopePayload struct {
	TenantID  string `json:"tenant_id"`
	UserID    string `json:"user_id"`
	ProjectID string `json:"project_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

// RecallRequest is the body for POST /memory/recall/unified.
type RecallRequest struct {
	Scope             ScopePayload     `json:"scope"`
	Query             string           `json:"query"`
	TopK              int              `json:"top_k,omitempty"`
	TokenBudget       int              `json:"token_budget,omitempty"`
	MemoryTokenBudget int              `json:"memory_token_budget,omitempty"`
	ConsistencyLevel  ConsistencyLevel `json:"consistency_level,omitempty"`
	AllowStaleCache   bool             `json:"allow_stale_cache,omitempty"`
	Debug             bool             `json:"debug,omitempty"`
}

// RecallResponse is the body for POST /memory/recall/unified.
type RecallResponse struct {
	Hits  []RecallHit  `json:"hits"`
	Trace *RecallTrace `json:"trace,omitempty"`
}

// RecallHit is a single recalled memory.
type RecallHit struct {
	MemoryID string            `json:"memory_id"`
	Kind     string            `json:"kind"`
	Score    float64           `json:"score"`
	Version  int64             `json:"version"`
	Content  string            `json:"content"`
	Tags     []string          `json:"tags,omitempty"`
	Source   string            `json:"source"`
	Category string            `json:"category"`
	Pinned   bool              `json:"pinned"`
	Disabled bool              `json:"disabled"`
	Metadata RecallHitMetadata `json:"metadata"`
}

// RecallHitMetadata carries per-hit diagnostic metadata.
type RecallHitMetadata struct {
	MatchedBy         string `json:"matched_by,omitempty"`
	TokenCostEstimate int    `json:"token_cost_estimate,omitempty"`
}

// RecallTrace is the debug trace returned when Debug is set on the request.
type RecallTrace struct {
	CacheLevel            string `json:"cache_level,omitempty"`
	ConsistencyLevel      string `json:"consistency_level,omitempty"`
	StaleServed           bool   `json:"stale_served"`
	MemoryTokenBudget     int    `json:"memory_token_budget,omitempty"`
	ReturnedTokenEstimate int    `json:"returned_token_estimate,omitempty"`
}

// WriteRequest is the body for POST /memory/write. The gateway requires a
// non-empty idempotency_key; the client auto-generates one when empty.
type WriteRequest struct {
	IdempotencyKey string             `json:"idempotency_key"`
	Scope          ScopePayload       `json:"scope"`
	Record         WriteRecordPayload `json:"record"`
}

// WriteRecordPayload is the memory record being written.
type WriteRecordPayload struct {
	Kind       string   `json:"kind"`
	Source     string   `json:"source"`
	Category   string   `json:"category"`
	Content    string   `json:"content"`
	Tags       []string `json:"tags,omitempty"`
	Importance float64  `json:"importance,omitempty"`
	Pinned     bool     `json:"pinned"`
}

// WriteResponse is the body for POST /memory/write.
type WriteResponse struct {
	Memory WriteResult `json:"memory"`
}

// WriteResult is the written-memory descriptor.
type WriteResult struct {
	MemoryID string `json:"memory_id"`
	Version  int64  `json:"version"`
	Status   string `json:"status"`
}

// patchMemoryRequest is the wire body for PATCH /memory/items/{memory_id}.
// It is built by the client from a PatchRequest; callers do not construct it.
type patchMemoryRequest struct {
	IdempotencyKey  string            `json:"idempotency_key,omitempty"`
	Scope           ScopePayload      `json:"scope"`
	ExpectedVersion int64             `json:"expected_version"`
	Patch           patchMemoryFields `json:"patch"`
}

// patchMemoryFields mirrors the gateway's PatchMemoryFields. Every field is a
// pointer so an unset field is OMITTED from JSON (field-not-present), while an
// explicit zero value (e.g. "" or 0) is sent. This is why the client re-declares
// the type instead of reusing the durable contract.
type patchMemoryFields struct {
	Content    *string   `json:"content,omitempty"`
	Category   *string   `json:"category,omitempty"`
	Tags       *[]string `json:"tags,omitempty"`
	Importance *float64  `json:"importance,omitempty"`
}

// PatchResult is the body for PATCH /memory/items/{memory_id}.
type PatchResult struct {
	MemoryID string `json:"memory_id"`
	Version  int64  `json:"version"`
}

// pinMemoryRequest is the wire body for the pin and unpin endpoints (they share
// the same request shape).
type pinMemoryRequest struct {
	Scope           ScopePayload `json:"scope"`
	ExpectedVersion int64        `json:"expected_version"`
}

// PinResult is the response for the pin and unpin endpoints.
type PinResult struct {
	MemoryID string `json:"memory_id"`
	Version  int64  `json:"version"`
	Pinned   bool   `json:"pinned"`
}

// disableMemoryRequest is the wire body for the disable and enable endpoints
// (they share the same request shape).
type disableMemoryRequest struct {
	Scope           ScopePayload `json:"scope"`
	ExpectedVersion int64        `json:"expected_version"`
}

// DisableResult is the response for the disable and enable endpoints.
type DisableResult struct {
	MemoryID string `json:"memory_id"`
	Version  int64  `json:"version"`
	Disabled bool   `json:"disabled"`
}

// deleteMemoryRequest is the wire body for DELETE /memory/items/{memory_id}.
// The gateway requires Content-Type application/json on DELETE, so the body is
// always sent.
type deleteMemoryRequest struct {
	Scope            ScopePayload     `json:"scope"`
	ExpectedVersion  int64            `json:"expected_version"`
	ConsistencyLevel ConsistencyLevel `json:"consistency_level,omitempty"`
}

// DeleteResult is the body for DELETE /memory/items/{memory_id}.
type DeleteResult struct {
	MemoryID string `json:"memory_id"`
	Deleted  bool   `json:"deleted"`
	Version  int64  `json:"version"`
}

// Item is the body for GET /memory/items/{memory_id}.
type Item struct {
	MemoryID   string   `json:"memory_id"`
	Kind       string   `json:"kind"`
	Version    int64    `json:"version"`
	Content    string   `json:"content"`
	Tags       []string `json:"tags,omitempty"`
	Source     string   `json:"source"`
	Category   string   `json:"category"`
	Importance float64  `json:"importance,omitempty"`
	Pinned     bool     `json:"pinned"`
	Disabled   bool     `json:"disabled"`
}

// sessionCloseRequest is the wire body for POST /memory/sessions/{id}/close.
type sessionCloseRequest struct {
	Scope ScopePayload `json:"scope"`
	Mode  string       `json:"mode"`
}

// sessionHeartbeatRequest is the wire body for POST /memory/sessions/{id}/heartbeat.
type sessionHeartbeatRequest struct {
	Scope ScopePayload `json:"scope"`
}

// SessionStatus is the response for the session close and heartbeat endpoints.
type SessionStatus struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
}
