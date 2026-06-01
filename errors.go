package memoryclient

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Sentinel errors for each gateway error class. They are wrapped by
// *GatewayError so callers can branch with errors.Is while still recovering the
// full error via errors.As.
var (
	ErrBadRequest          = errors.New("memoryclient: bad request")
	ErrUnauthorized        = errors.New("memoryclient: unauthorized")
	ErrForbidden           = errors.New("memoryclient: forbidden")
	ErrNotFound            = errors.New("memoryclient: not found")
	ErrConflict            = errors.New("memoryclient: conflict")
	ErrIdempotencyConflict = errors.New("memoryclient: idempotency conflict")
	ErrUnavailable         = errors.New("memoryclient: service unavailable")
)

// GatewayError is a typed, decoded gateway error envelope. It implements error,
// reports Retryable via a method, and unwraps to the matching sentinel error.
type GatewayError struct {
	Code       string
	Message    string
	RequestID  string
	Details    map[string]any
	HTTPStatus int

	retryable bool
	sentinel  error
}

// Retryable reports whether the gateway flagged this error as safe to retry.
//
// Both *GatewayError and *TransportError expose Retryable() bool, so a caller
// can branch on retryability for either with a single predicate:
//
//	var r interface{ Retryable() bool }
//	if errors.As(err, &r) && r.Retryable() { ... }
func (e *GatewayError) Retryable() bool { return e.retryable }

func (e *GatewayError) Error() string {
	if e.RequestID != "" {
		return fmt.Sprintf("memoryclient: gateway error %s (status %d, request %s): %s", e.Code, e.HTTPStatus, e.RequestID, e.Message)
	}
	return fmt.Sprintf("memoryclient: gateway error %s (status %d): %s", e.Code, e.HTTPStatus, e.Message)
}

// Unwrap returns the sentinel error matching this gateway error's code so that
// errors.Is(err, ErrConflict) and friends work.
func (e *GatewayError) Unwrap() error {
	return e.sentinel
}

// errorEnvelope mirrors the gateway's ErrorResponse / ErrorBody JSON shape.
type errorEnvelope struct {
	Error struct {
		Code      string         `json:"code"`
		Message   string         `json:"message"`
		RequestID string         `json:"request_id"`
		Retryable bool           `json:"retryable"`
		Details   map[string]any `json:"details,omitempty"`
	} `json:"error"`
}

// sentinelForCode maps a gateway error code to its client sentinel.
func sentinelForCode(code string) error {
	switch code {
	case "bad_request":
		return ErrBadRequest
	case "unauthorized":
		return ErrUnauthorized
	case "forbidden", "session_expired":
		return ErrForbidden
	case "not_found":
		return ErrNotFound
	case "memory_conflict":
		return ErrConflict
	case "idempotency_conflict":
		return ErrIdempotencyConflict
	case "read_only_mode", "upstream_unavailable":
		return ErrUnavailable
	default:
		return nil
	}
}

// parseErrorResponse decodes the gateway error envelope from a non-2xx response
// and returns a *GatewayError wrapping the matching sentinel. If the body is not
// a decodable envelope, it still returns a *GatewayError carrying the status so
// callers always get a typed error.
func parseErrorResponse(status int, body []byte) error {
	var env errorEnvelope
	ge := &GatewayError{HTTPStatus: status}

	if err := json.Unmarshal(body, &env); err == nil && env.Error.Code != "" {
		ge.Code = env.Error.Code
		ge.Message = env.Error.Message
		ge.RequestID = env.Error.RequestID
		ge.retryable = env.Error.Retryable
		ge.Details = env.Error.Details
	} else {
		ge.Message = fmt.Sprintf("unexpected status %d", status)
	}

	ge.sentinel = sentinelForCode(ge.Code)
	return ge
}
