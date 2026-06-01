package memoryclient

import "context"

// Recall performs a unified recall against POST /memory/recall/unified.
//
// The client fills the request scope from its configured tenant/user/project/
// session. ConsistencyLevel, AllowStaleCache, and Debug are passed through
// verbatim — the client does no caching or re-ranking of its own.
func (g *GatewayMemory) Recall(ctx context.Context, req RecallRequest) (RecallResponse, error) {
	req.Scope = g.scopePayload()

	var resp RecallResponse
	if err := g.do(ctx, "POST", "/memory/recall/unified", req, &resp); err != nil {
		return RecallResponse{}, err
	}
	return resp, nil
}
