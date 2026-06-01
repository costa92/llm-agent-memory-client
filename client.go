package memoryclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultUserAgent = "llm-agent-memory-client/0.1"
	defaultTimeout   = 30 * time.Second
)

// TransportError wraps a client-side / network failure that happened before a
// gateway response was received (or while reading it). It is always retryable
// and is distinct from *GatewayError, which carries a decoded gateway envelope.
type TransportError struct {
	Op  string
	Err error
}

func (e *TransportError) Error() string {
	return fmt.Sprintf("memoryclient: transport error during %s: %v", e.Op, e.Err)
}

func (e *TransportError) Unwrap() error { return e.Err }

// Retryable reports that the request may be safely retried.
func (e *TransportError) Retryable() bool { return true }

// MemoryClient is the full set of gateway operations implemented by
// *GatewayMemory. It covers all eleven endpoints: recall + write, the item
// lifecycle (patch, pin, unpin, disable, enable, delete, get), and the session
// endpoints (close, heartbeat).
type MemoryClient interface {
	Recall(ctx context.Context, req RecallRequest) (RecallResponse, error)
	Write(ctx context.Context, req WriteRequest) (WriteResult, error)
	Patch(ctx context.Context, memoryID string, req PatchRequest) (PatchResult, error)
	Pin(ctx context.Context, memoryID string, expectedVersion int64) (PinResult, error)
	Unpin(ctx context.Context, memoryID string, expectedVersion int64) (PinResult, error)
	Disable(ctx context.Context, memoryID string, expectedVersion int64) (DisableResult, error)
	Enable(ctx context.Context, memoryID string, expectedVersion int64) (DisableResult, error)
	Delete(ctx context.Context, memoryID string, expectedVersion int64, consistencyLevel ConsistencyLevel) (DeleteResult, error)
	Get(ctx context.Context, memoryID string) (Item, error)
	CloseSession(ctx context.Context, sessionID, mode string) (SessionStatus, error)
	Heartbeat(ctx context.Context, sessionID string) (SessionStatus, error)
}

var _ MemoryClient = (*GatewayMemory)(nil)

// GatewayMemory is a client for the memory gateway HTTP API.
type GatewayMemory struct {
	baseURL    string
	tenant     string
	user       string
	project    string
	session    string
	credential CredentialProvider
	httpClient *http.Client
	userAgent  string
}

// New constructs a GatewayMemory. BaseURL, Tenant, and User are required.
func New(opts ...Option) (*GatewayMemory, error) {
	cfg := config{userAgent: defaultUserAgent}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	cfg.baseURL = strings.TrimRight(strings.TrimSpace(cfg.baseURL), "/")
	cfg.tenant = strings.TrimSpace(cfg.tenant)
	cfg.user = strings.TrimSpace(cfg.user)

	if cfg.baseURL == "" {
		return nil, errors.New("memoryclient: WithBaseURL is required")
	}
	if cfg.tenant == "" {
		return nil, errors.New("memoryclient: WithTenant is required")
	}
	if cfg.user == "" {
		return nil, errors.New("memoryclient: WithUser is required")
	}

	if cfg.httpClient == nil {
		cfg.httpClient = &http.Client{Timeout: defaultTimeout}
	}
	if cfg.userAgent == "" {
		cfg.userAgent = defaultUserAgent
	}

	return &GatewayMemory{
		baseURL:    cfg.baseURL,
		tenant:     cfg.tenant,
		user:       cfg.user,
		project:    strings.TrimSpace(cfg.project),
		session:    strings.TrimSpace(cfg.session),
		credential: cfg.credential,
		httpClient: cfg.httpClient,
		userAgent:  cfg.userAgent,
	}, nil
}

// scopePayload builds the request-body scope from the configured scope.
func (g *GatewayMemory) scopePayload() ScopePayload {
	return ScopePayload{
		TenantID:  g.tenant,
		UserID:    g.user,
		ProjectID: g.project,
		SessionID: g.session,
	}
}

// itemPath builds /memory/items/{memory_id}[/suffix...], escaping memoryID so a
// caller-supplied ID with odd characters cannot break out of its path segment.
// suffix segments (e.g. "pin") are appended verbatim as they are client-fixed.
func itemPath(memoryID string, suffix ...string) string {
	p := "/memory/items/" + url.PathEscape(memoryID)
	for _, s := range suffix {
		p += "/" + s
	}
	return p
}

// sessionPath builds /memory/sessions/{session_id}/{suffix}, escaping sessionID.
func sessionPath(sessionID, suffix string) string {
	return "/memory/sessions/" + url.PathEscape(sessionID) + "/" + suffix
}

// do performs an HTTP request against the gateway: it marshals reqBody (when
// non-nil), sets the exact required headers and scope/credential headers, and on
// a 2xx decodes the response into out (when non-nil). Non-2xx responses are
// returned as *GatewayError; pre-response failures as *TransportError.
func (g *GatewayMemory) do(ctx context.Context, method, path string, reqBody, out any) error {
	var body io.Reader
	if reqBody != nil {
		encoded, err := json.Marshal(reqBody)
		if err != nil {
			return &TransportError{Op: "marshal request", Err: err}
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, g.baseURL+path, body)
	if err != nil {
		return &TransportError{Op: "build request", Err: err}
	}

	// EXACT values — the gateway rejects "application/json; charset=utf-8".
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", g.userAgent)

	req.Header.Set("X-Tenant-Id", g.tenant)
	req.Header.Set("X-User-Id", g.user)
	if g.project != "" {
		req.Header.Set("X-Project-Id", g.project)
	}
	if g.session != "" {
		req.Header.Set("X-Session-Id", g.session)
	}

	if g.credential != nil {
		headers, err := g.credential.AuthHeaders(ctx)
		if err != nil {
			return &TransportError{Op: "resolve credentials", Err: err}
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return &TransportError{Op: "round trip", Err: err}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return &TransportError{Op: "read response body", Err: err}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseErrorResponse(resp.StatusCode, respBody)
	}

	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return &TransportError{Op: "decode response", Err: err}
		}
	}

	return nil
}
