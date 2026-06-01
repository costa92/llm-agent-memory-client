package memoryclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestParseErrorResponseMapsSentinels(t *testing.T) {
	cases := []struct {
		status       int
		code         string
		retryable    bool
		wantSentinel error
	}{
		{http.StatusBadRequest, "bad_request", false, ErrBadRequest},
		{http.StatusUnauthorized, "unauthorized", false, ErrUnauthorized},
		{http.StatusForbidden, "forbidden", false, ErrForbidden},
		{http.StatusForbidden, "session_expired", false, ErrForbidden},
		{http.StatusNotFound, "not_found", false, ErrNotFound},
		{http.StatusConflict, "memory_conflict", false, ErrConflict},
		{http.StatusConflict, "idempotency_conflict", false, ErrIdempotencyConflict},
		{http.StatusServiceUnavailable, "read_only_mode", true, ErrUnavailable},
		{http.StatusServiceUnavailable, "upstream_unavailable", true, ErrUnavailable},
	}

	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			body := fmt.Appendf(nil, `{"error":{"code":%q,"message":"boom","request_id":"req-7","retryable":%t,"details":{"k":"v"}}}`, tc.code, tc.retryable)
			err := parseErrorResponse(tc.status, body)

			if !errors.Is(err, tc.wantSentinel) {
				t.Fatalf("errors.Is(%v) = false", tc.wantSentinel)
			}

			var ge *GatewayError
			if !errors.As(err, &ge) {
				t.Fatalf("errors.As(*GatewayError) = false")
			}
			if ge.Code != tc.code {
				t.Errorf("Code = %q, want %q", ge.Code, tc.code)
			}
			if ge.RequestID != "req-7" {
				t.Errorf("RequestID = %q", ge.RequestID)
			}
			if ge.Retryable() != tc.retryable {
				t.Errorf("Retryable() = %v, want %v", ge.Retryable(), tc.retryable)
			}
			if ge.HTTPStatus != tc.status {
				t.Errorf("HTTPStatus = %d, want %d", ge.HTTPStatus, tc.status)
			}
			if ge.Details["k"] != "v" {
				t.Errorf("Details not decoded: %v", ge.Details)
			}
		})
	}
}

func TestConflictSentinelsAreDistinct(t *testing.T) {
	memErr := parseErrorResponse(http.StatusConflict, []byte(`{"error":{"code":"memory_conflict","message":"x"}}`))
	idemErr := parseErrorResponse(http.StatusConflict, []byte(`{"error":{"code":"idempotency_conflict","message":"x"}}`))

	if !errors.Is(memErr, ErrConflict) || errors.Is(memErr, ErrIdempotencyConflict) {
		t.Errorf("memory_conflict should match ErrConflict only")
	}
	if !errors.Is(idemErr, ErrIdempotencyConflict) || errors.Is(idemErr, ErrConflict) {
		t.Errorf("idempotency_conflict should match ErrIdempotencyConflict only")
	}
}

func TestParseErrorResponseUndecodableBody(t *testing.T) {
	err := parseErrorResponse(http.StatusBadGateway, []byte(`<html>nope</html>`))
	var ge *GatewayError
	if !errors.As(err, &ge) {
		t.Fatalf("expected *GatewayError even for garbage body")
	}
	if ge.HTTPStatus != http.StatusBadGateway {
		t.Errorf("HTTPStatus = %d", ge.HTTPStatus)
	}
}

func TestEndToEndErrorThroughClient(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"code":"idempotency_conflict","message":"dup","request_id":"r1","retryable":false}}`))
	})

	_, err := c.Write(context.Background(), WriteRequest{IdempotencyKey: "k", Record: WriteRecordPayload{Content: "x"}})
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected ErrIdempotencyConflict, got %v", err)
	}
	var ge *GatewayError
	if !errors.As(err, &ge) || ge.RequestID != "r1" {
		t.Errorf("GatewayError not exposed: %v", err)
	}
}
