// Package memoryclient is a stdlib-only Go HTTP client for the llm-agent
// memory gateway.
//
// It speaks the gateway's HTTP/JSON contract directly: scope is carried in
// X-Tenant-Id / X-User-Id (and optional X-Project-Id / X-Session-Id) headers,
// requests and responses use the gateway's wire DTOs, and errors decode into a
// typed [GatewayError] that wraps stable sentinel errors for errors.Is.
//
// North star:
//
//   - The client never imports llm-agent, the memory gateway, or the memory
//     contract module. It re-declares the wire DTOs (Option B) so it can ship
//     as a standalone dependency, and depends only on the standard library.
//   - There is no client-side cache in v1. Consistency, caching, staleness, and
//     re-ranking are the gateway's concern; the client only passes
//     ConsistencyLevel / AllowStaleCache / Debug through and returns what the
//     gateway sends back.
//
// Phase 1 covers the vertical slice: the auth/do machinery, typed errors,
// Recall, and Write. Items, sessions, and the higher-level adapter come later.
package memoryclient
