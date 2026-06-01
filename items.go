package memoryclient

import "context"

// PatchRequest accumulates a partial update for Patch. Each Set* helper marks a
// field for inclusion; an unset field stays nil and is OMITTED from the request
// JSON, so the gateway leaves it unchanged. Setting a field to its zero value
// (e.g. SetContent("") or SetImportance(0)) sends that explicit zero, distinct
// from omitting it. Build one with PatchRequest{}.SetContent(...) chaining, or
// set fields directly.
type PatchRequest struct {
	content    *string
	category   *string
	tags       *[]string
	importance *float64

	// ExpectedVersion is the optimistic-concurrency guard sent as expected_version.
	ExpectedVersion int64

	// IdempotencyKey is optional; when non-empty it is sent so the gateway can
	// short-circuit a replay of the same logical patch.
	IdempotencyKey string
}

// SetContent marks the content field for update (empty string is sent explicitly).
func (p PatchRequest) SetContent(content string) PatchRequest {
	p.content = &content
	return p
}

// SetCategory marks the category field for update.
func (p PatchRequest) SetCategory(category string) PatchRequest {
	p.category = &category
	return p
}

// SetTags marks the tags field for update. A nil slice and an empty slice are
// both sent (as null and [] respectively) — both differ from leaving tags unset.
func (p PatchRequest) SetTags(tags []string) PatchRequest {
	p.tags = &tags
	return p
}

// SetImportance marks the importance field for update (0 is sent explicitly).
func (p PatchRequest) SetImportance(importance float64) PatchRequest {
	p.importance = &importance
	return p
}

// Patch applies a partial update to a memory item via PATCH
// /memory/items/{memory_id}. Only fields marked via PatchRequest.Set* are sent;
// unset fields are omitted so the gateway leaves them unchanged.
//
// Patch is retry-safe given a stable expectedVersion: the gateway short-circuits
// replays server-side (an optional IdempotencyKey further dedupes). The client
// does NOT auto-retry.
func (g *GatewayMemory) Patch(ctx context.Context, memoryID string, req PatchRequest) (PatchResult, error) {
	wire := patchMemoryRequest{
		IdempotencyKey:  req.IdempotencyKey,
		Scope:           g.scopePayload(),
		ExpectedVersion: req.ExpectedVersion,
		Patch: patchMemoryFields{
			Content:    req.content,
			Category:   req.category,
			Tags:       req.tags,
			Importance: req.importance,
		},
	}

	var resp PatchResult
	if err := g.do(ctx, "PATCH", itemPath(memoryID), wire, &resp); err != nil {
		return PatchResult{}, err
	}
	return resp, nil
}

// Pin pins a memory item via POST /memory/items/{memory_id}/pin.
//
// Pin is retry-safe given a stable expectedVersion: the gateway short-circuits
// replays server-side. The client does NOT auto-retry.
func (g *GatewayMemory) Pin(ctx context.Context, memoryID string, expectedVersion int64) (PinResult, error) {
	return g.pinUnpin(ctx, itemPath(memoryID, "pin"), expectedVersion)
}

// Unpin unpins a memory item via POST /memory/items/{memory_id}/unpin. It uses
// the same request/response shape as Pin.
//
// Unpin is retry-safe given a stable expectedVersion: the gateway short-circuits
// replays server-side. The client does NOT auto-retry.
func (g *GatewayMemory) Unpin(ctx context.Context, memoryID string, expectedVersion int64) (PinResult, error) {
	return g.pinUnpin(ctx, itemPath(memoryID, "unpin"), expectedVersion)
}

func (g *GatewayMemory) pinUnpin(ctx context.Context, path string, expectedVersion int64) (PinResult, error) {
	req := pinMemoryRequest{Scope: g.scopePayload(), ExpectedVersion: expectedVersion}
	var resp PinResult
	if err := g.do(ctx, "POST", path, req, &resp); err != nil {
		return PinResult{}, err
	}
	return resp, nil
}

// Disable disables a memory item via POST /memory/items/{memory_id}/disable.
//
// Disable is retry-safe given a stable expectedVersion: the gateway
// short-circuits replays server-side. The client does NOT auto-retry.
func (g *GatewayMemory) Disable(ctx context.Context, memoryID string, expectedVersion int64) (DisableResult, error) {
	return g.disableEnable(ctx, itemPath(memoryID, "disable"), expectedVersion)
}

// Enable enables a memory item via POST /memory/items/{memory_id}/enable. It
// uses the same request/response shape as Disable.
//
// Enable is retry-safe given a stable expectedVersion: the gateway
// short-circuits replays server-side. The client does NOT auto-retry.
func (g *GatewayMemory) Enable(ctx context.Context, memoryID string, expectedVersion int64) (DisableResult, error) {
	return g.disableEnable(ctx, itemPath(memoryID, "enable"), expectedVersion)
}

func (g *GatewayMemory) disableEnable(ctx context.Context, path string, expectedVersion int64) (DisableResult, error) {
	req := disableMemoryRequest{Scope: g.scopePayload(), ExpectedVersion: expectedVersion}
	var resp DisableResult
	if err := g.do(ctx, "POST", path, req, &resp); err != nil {
		return DisableResult{}, err
	}
	return resp, nil
}

// Delete deletes a memory item via DELETE /memory/items/{memory_id}. The gateway
// requires Content-Type application/json on DELETE, so a JSON body carrying the
// expected version (and optional consistency level) is always sent.
//
// Delete is retry-safe given a stable expectedVersion: the gateway
// short-circuits replays server-side. The client does NOT auto-retry.
func (g *GatewayMemory) Delete(ctx context.Context, memoryID string, expectedVersion int64, consistencyLevel ConsistencyLevel) (DeleteResult, error) {
	req := deleteMemoryRequest{
		Scope:            g.scopePayload(),
		ExpectedVersion:  expectedVersion,
		ConsistencyLevel: consistencyLevel,
	}
	var resp DeleteResult
	if err := g.do(ctx, "DELETE", itemPath(memoryID), req, &resp); err != nil {
		return DeleteResult{}, err
	}
	return resp, nil
}

// Get fetches a memory item via GET /memory/items/{memory_id}. It sends no body.
func (g *GatewayMemory) Get(ctx context.Context, memoryID string) (Item, error) {
	var resp Item
	if err := g.do(ctx, "GET", itemPath(memoryID), nil, &resp); err != nil {
		return Item{}, err
	}
	return resp, nil
}
