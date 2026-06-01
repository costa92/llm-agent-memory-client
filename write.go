package memoryclient

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

// Write writes a memory record via POST /memory/write.
//
// The gateway requires a non-empty idempotency_key (it returns 400 otherwise).
// When the caller leaves IdempotencyKey empty, the client generates a random one
// so writes are retry-safe by default; callers may supply their own to dedupe
// retries of the same logical write.
func (g *GatewayMemory) Write(ctx context.Context, req WriteRequest) (WriteResult, error) {
	if req.IdempotencyKey == "" {
		key, err := newIdempotencyKey()
		if err != nil {
			return WriteResult{}, &TransportError{Op: "generate idempotency key", Err: err}
		}
		req.IdempotencyKey = key
	}
	req.Scope = g.scopePayload()

	var resp WriteResponse
	if err := g.do(ctx, "POST", "/memory/write", req, &resp); err != nil {
		return WriteResult{}, err
	}
	return resp.Memory, nil
}

// newIdempotencyKey returns a random 128-bit hex string.
func newIdempotencyKey() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}
