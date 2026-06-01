package memoryclient

import (
	"context"
	"net/http"
)

// CredentialProvider supplies per-request auth headers (e.g. a bearer token).
// AuthHeaders is called on every request; it may refresh or mint credentials.
type CredentialProvider interface {
	AuthHeaders(ctx context.Context) (map[string]string, error)
}

type staticCredential map[string]string

func (s staticCredential) AuthHeaders(context.Context) (map[string]string, error) {
	return map[string]string(s), nil
}

// StaticCredential returns a CredentialProvider that always emits the same
// headers. A copy is taken so later mutation of the input map has no effect.
func StaticCredential(headers map[string]string) CredentialProvider {
	cp := make(staticCredential, len(headers))
	for k, v := range headers {
		cp[k] = v
	}
	return cp
}

type config struct {
	baseURL    string
	tenant     string
	user       string
	project    string
	session    string
	credential CredentialProvider
	httpClient *http.Client
	userAgent  string
}

// Option configures a GatewayMemory at construction time.
type Option func(*config)

// WithBaseURL sets the gateway base URL (required). Trailing slashes are trimmed.
func WithBaseURL(baseURL string) Option {
	return func(c *config) { c.baseURL = baseURL }
}

// WithTenant sets the X-Tenant-Id header value (required).
func WithTenant(tenant string) Option {
	return func(c *config) { c.tenant = tenant }
}

// WithUser sets the X-User-Id header value (required).
func WithUser(user string) Option {
	return func(c *config) { c.user = user }
}

// WithProject sets the optional X-Project-Id header value.
func WithProject(project string) Option {
	return func(c *config) { c.project = project }
}

// WithSession sets the optional X-Session-Id header value.
func WithSession(session string) Option {
	return func(c *config) { c.session = session }
}

// WithCredential sets the CredentialProvider used to merge auth headers.
func WithCredential(cp CredentialProvider) Option {
	return func(c *config) { c.credential = cp }
}

// WithHTTPClient overrides the underlying *http.Client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *config) { c.httpClient = hc }
}

// WithUserAgent overrides the User-Agent header.
func WithUserAgent(ua string) Option {
	return func(c *config) { c.userAgent = ua }
}
