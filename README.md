# llm-agent-memory-client

A stdlib-only Go HTTP client SDK for the `llm-agent-memory-gateway` service.

[English](./README.md) | [简体中文](./README.zh-CN.md)

`llm-agent-memory-client` lets an application talk to the memory gateway over its
HTTP/JSON contract: recall memories, write new ones, manage the item lifecycle
(patch, pin/unpin, disable/enable, delete, get), and keep sessions alive. It
depends only on the Go standard library and ships as a standalone module.

## Design

- **Transport only.** The client never imports `llm-agent`, the memory gateway,
  or the memory contract module. It re-declares the gateway's wire DTOs so it can
  ship as a standalone dependency with no third-party packages.
- **No client-side cache (v1).** Consistency, caching, staleness, and re-ranking
  are the gateway's concern. The client only passes `ConsistencyLevel`,
  `AllowStaleCache`, and `Debug` through, and returns what the gateway sends back.
- **Scope in headers.** Every request carries `X-Tenant-Id` / `X-User-Id` (and
  optional `X-Project-Id` / `X-Session-Id`). The gateway treats these auth-scope
  headers as authoritative.

## Install

```sh
go get github.com/costa92/llm-agent-memory-client
```

```go
import memoryclient "github.com/costa92/llm-agent-memory-client"
```

## Connection / configuration

Construct a client with `New`, pointing it at a gateway base URL. `WithBaseURL`,
`WithTenant`, and `WithUser` are required; the rest are optional.

```go
client, err := memoryclient.New(
    memoryclient.WithBaseURL("https://memory-gateway.example.com"),
    memoryclient.WithTenant("acme"),
    memoryclient.WithUser("user-123"),
    // optional:
    memoryclient.WithProject("project-abc"),
    memoryclient.WithSession("session-xyz"),
    memoryclient.WithCredential(memoryclient.StaticCredential(map[string]string{
        "Authorization": "Bearer " + token,
    })),
    memoryclient.WithHTTPClient(&http.Client{Timeout: 10 * time.Second}),
    memoryclient.WithUserAgent("my-app/1.0"),
)
if err != nil {
    log.Fatal(err)
}
```

| Option | Required | Purpose |
|---|---|---|
| `WithBaseURL` | yes | Gateway base URL (trailing slashes trimmed) |
| `WithTenant` | yes | `X-Tenant-Id` header |
| `WithUser` | yes | `X-User-Id` header |
| `WithProject` | no | `X-Project-Id` header |
| `WithSession` | no | `X-Session-Id` header |
| `WithCredential` | no | `CredentialProvider` that merges per-request auth headers |
| `WithHTTPClient` | no | Override the underlying `*http.Client` (default timeout 30s) |
| `WithUserAgent` | no | Override the `User-Agent` header |

`CredentialProvider.AuthHeaders` is called on every request, so a custom provider
may refresh or mint credentials. `StaticCredential` returns a provider that always
emits the same headers.

## Usage

### Write a memory

`Write` writes a memory record via `POST /memory/write`. The gateway requires a
non-empty idempotency key; when `IdempotencyKey` is empty the client generates a
random one, so writes are retry-safe by default.

```go
result, err := client.Write(ctx, memoryclient.WriteRequest{
    Record: memoryclient.WriteRecordPayload{
        Kind:       "semantic",
        Source:     "chat",
        Category:   "preference",
        Content:    "User prefers dark mode.",
        Tags:       []string{"ui", "preference"},
        Importance: 0.8,
    },
})
if err != nil {
    log.Fatal(err)
}
fmt.Println(result.MemoryID, result.Version, result.Status)
```

### Recall memories

`Recall` performs a unified recall via `POST /memory/recall/unified`. The client
fills the request scope from its configured tenant/user/project/session.

```go
resp, err := client.Recall(ctx, memoryclient.RecallRequest{
    Query:            "what are the user's UI preferences?",
    TopK:             5,
    ConsistencyLevel: memoryclient.ConsistencyBounded,
})
if err != nil {
    log.Fatal(err)
}
for _, hit := range resp.Hits {
    fmt.Printf("%s (score %.3f): %s\n", hit.MemoryID, hit.Score, hit.Content)
}
```

### Manage an item

```go
// Optimistic concurrency: pass the expected version.
patched, err := client.Patch(ctx, memoryID, memoryclient.PatchRequest{
    ExpectedVersion: result.Version,
}.SetContent("User prefers dark mode and large fonts."))

pinned, err := client.Pin(ctx, memoryID, patched.Version)
_, err = client.Unpin(ctx, memoryID, pinned.Version)

_, err = client.Disable(ctx, memoryID, version)
_, err = client.Enable(ctx, memoryID, version)

item, err := client.Get(ctx, memoryID)

_, err = client.Delete(ctx, memoryID, version, memoryclient.ConsistencyEventual)
```

### Sessions

```go
_, err = client.Heartbeat(ctx, sessionID)
status, err := client.CloseSession(ctx, sessionID, "flush")
```

### Error handling

Non-2xx gateway responses decode into a typed `*GatewayError` that wraps stable
sentinel errors, so you can branch with `errors.Is`. Client-side / network
failures (before a gateway response) return a `*TransportError`, which is always
retryable. Both expose `Retryable() bool`.

```go
result, err := client.Write(ctx, req)
if errors.Is(err, memoryclient.ErrConflict) {
    // optimistic-concurrency conflict — refetch and retry
}

var retr interface{ Retryable() bool }
if errors.As(err, &retr) && retr.Retryable() {
    // safe to retry
}
```

Sentinel errors: `ErrBadRequest`, `ErrUnauthorized`, `ErrForbidden`,
`ErrNotFound`, `ErrConflict`, `ErrIdempotencyConflict`, `ErrUnavailable`.

## Core API

The `MemoryClient` interface (implemented by `*GatewayMemory`) covers all eleven
gateway endpoints:

| Method | Endpoint | Purpose |
|---|---|---|
| `Recall` | `POST /memory/recall/unified` | Unified recall across memory kinds |
| `Write` | `POST /memory/write` | Write a memory record |
| `Patch` | `PATCH /memory/items/{id}` | Partial update (only `Set*` fields are sent) |
| `Pin` | `POST /memory/items/{id}/pin` | Pin an item |
| `Unpin` | `POST /memory/items/{id}/unpin` | Unpin an item |
| `Disable` | `POST /memory/items/{id}/disable` | Disable an item |
| `Enable` | `POST /memory/items/{id}/enable` | Enable an item |
| `Delete` | `DELETE /memory/items/{id}` | Delete an item |
| `Get` | `GET /memory/items/{id}` | Fetch a single item |
| `CloseSession` | `POST /memory/sessions/{id}/close` | Close a session |
| `Heartbeat` | `POST /memory/sessions/{id}/heartbeat` | Refresh session liveness |

`Patch`, `Pin`, `Unpin`, `Disable`, `Enable`, and `Delete` take an
`expectedVersion` for optimistic concurrency. They are retry-safe given a stable
expected version (the gateway short-circuits replays server-side); the client
does **not** auto-retry.

## Relationship to the ecosystem

- **`llm-agent-memory-gateway`** — the HTTP service this client talks to. The
  client speaks the gateway's HTTP/JSON contract directly (endpoints, headers,
  and wire DTOs).
- **`llm-agent-memory-contract`** — the durable-memory contract module. The
  client deliberately does **not** import it; it re-declares the wire DTOs
  (notably `patchMemoryFields`, where every field is a pointer so unset fields are
  omitted) so it can ship as a standalone, stdlib-only dependency.
- **`llm-agent` (core)** — the client never imports it. An optional, read-only
  example bridge under `examples/coreadapter` (its own nested module, outside the
  client's dependency graph) maps gateway recall hits into core
  `memory.SearchResult` values for use in `llm-agent`'s context builder.
