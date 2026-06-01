package memoryclient

import "context"

// CloseSession closes a session via POST /memory/sessions/{session_id}/close.
// mode is passed through verbatim as the request's mode field (the gateway
// interprets it, e.g. to choose how outstanding work is finalized).
func (g *GatewayMemory) CloseSession(ctx context.Context, sessionID, mode string) (SessionStatus, error) {
	req := sessionCloseRequest{Scope: g.scopePayload(), Mode: mode}
	var resp SessionStatus
	if err := g.do(ctx, "POST", sessionPath(sessionID, "close"), req, &resp); err != nil {
		return SessionStatus{}, err
	}
	return resp, nil
}

// Heartbeat refreshes a session's liveness via POST
// /memory/sessions/{session_id}/heartbeat.
func (g *GatewayMemory) Heartbeat(ctx context.Context, sessionID string) (SessionStatus, error) {
	req := sessionHeartbeatRequest{Scope: g.scopePayload()}
	var resp SessionStatus
	if err := g.do(ctx, "POST", sessionPath(sessionID, "heartbeat"), req, &resp); err != nil {
		return SessionStatus{}, err
	}
	return resp, nil
}
