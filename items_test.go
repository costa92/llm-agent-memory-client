package memoryclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
)

// readRaw returns the raw request body bytes.
func readRaw(t *testing.T, r *http.Request) []byte {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return b
}

func TestPatchPointerSemantics(t *testing.T) {
	cases := []struct {
		name       string
		req        PatchRequest
		wantPatch  map[string]any // expected decoded "patch" object
		absentKeys []string       // keys that must NOT appear in "patch"
	}{
		{
			name:       "all unset omitted",
			req:        PatchRequest{ExpectedVersion: 4},
			wantPatch:  map[string]any{},
			absentKeys: []string{"content", "category", "tags", "importance"},
		},
		{
			name:      "empty content present as empty string",
			req:       PatchRequest{ExpectedVersion: 4}.SetContent(""),
			wantPatch: map[string]any{"content": ""},
		},
		{
			name:      "importance zero present as 0",
			req:       PatchRequest{ExpectedVersion: 4}.SetImportance(0),
			wantPatch: map[string]any{"importance": float64(0)},
		},
		{
			name:      "empty tags present as empty array",
			req:       PatchRequest{ExpectedVersion: 4}.SetTags([]string{}),
			wantPatch: map[string]any{"tags": []any{}},
		},
		{
			name:      "nil tags present as null",
			req:       PatchRequest{ExpectedVersion: 4}.SetTags(nil),
			wantPatch: map[string]any{"tags": nil},
		},
		{
			name:      "content + category + tags + importance",
			req:       PatchRequest{ExpectedVersion: 7}.SetContent("hi").SetCategory("c").SetTags([]string{"a", "b"}).SetImportance(0.5),
			wantPatch: map[string]any{"content": "hi", "category": "c", "tags": []any{"a", "b"}, "importance": 0.5},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var raw []byte
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				raw = readRaw(t, r)
				_, _ = w.Write([]byte(`{"memory_id":"m1","version":8}`))
			})

			if _, err := c.Patch(context.Background(), "m1", tc.req); err != nil {
				t.Fatalf("Patch: %v", err)
			}

			var top map[string]json.RawMessage
			if err := json.Unmarshal(raw, &top); err != nil {
				t.Fatalf("unmarshal top: %v", err)
			}
			patchRaw, ok := top["patch"]
			if !ok {
				t.Fatalf("patch key absent in %s", raw)
			}
			var patch map[string]any
			if err := json.Unmarshal(patchRaw, &patch); err != nil {
				t.Fatalf("unmarshal patch: %v", err)
			}

			for _, k := range tc.absentKeys {
				if _, present := patch[k]; present {
					t.Errorf("key %q must be absent, body=%s", k, raw)
				}
			}
			for k, want := range tc.wantPatch {
				got, present := patch[k]
				if !present {
					// nil-valued keys still count as present in JSON
					if _, rawHas := keyPresent(patchRaw, k); !rawHas {
						t.Errorf("key %q expected present, body=%s", k, raw)
						continue
					}
				}
				if !jsonEqual(got, want) {
					t.Errorf("patch[%q] = %#v, want %#v (body=%s)", k, got, want, raw)
				}
			}
		})
	}
}

// keyPresent reports whether key k exists in the raw JSON object (even if null).
func keyPresent(raw json.RawMessage, k string) (any, bool) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, false
	}
	_, ok := m[k]
	return nil, ok
}

func jsonEqual(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}

func TestPatchRouteHeadersAndResponse(t *testing.T) {
	var method, path, ct string
	var scope map[string]any
	var expectedVersion any
	var idem string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		ct = r.Header.Get("Content-Type")
		body := decodeBody[map[string]any](t, r)
		scope, _ = body["scope"].(map[string]any)
		expectedVersion = body["expected_version"]
		idem, _ = body["idempotency_key"].(string)
		_, _ = w.Write([]byte(`{"memory_id":"m1","version":9}`))
	}, WithProject("proj-1"))

	res, err := c.Patch(context.Background(), "m1", PatchRequest{ExpectedVersion: 4, IdempotencyKey: "idem-1"}.SetContent("x"))
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if method != "PATCH" {
		t.Errorf("method = %q", method)
	}
	if path != "/memory/items/m1" {
		t.Errorf("path = %q", path)
	}
	if ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	if expectedVersion != float64(4) {
		t.Errorf("expected_version = %v", expectedVersion)
	}
	if idem != "idem-1" {
		t.Errorf("idempotency_key = %q", idem)
	}
	if scope["tenant_id"] != "tenant-1" || scope["project_id"] != "proj-1" {
		t.Errorf("scope = %v", scope)
	}
	if res.MemoryID != "m1" || res.Version != 9 {
		t.Errorf("result = %+v", res)
	}
}

func TestPatchOmitsIdempotencyKeyWhenEmpty(t *testing.T) {
	var body map[string]any
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body = decodeBody[map[string]any](t, r)
		_, _ = w.Write([]byte(`{"memory_id":"m","version":1}`))
	})
	if _, err := c.Patch(context.Background(), "m", PatchRequest{ExpectedVersion: 1}.SetContent("x")); err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if _, ok := body["idempotency_key"]; ok {
		t.Errorf("idempotency_key should be omitted when empty")
	}
}

func TestPinUnpin(t *testing.T) {
	cases := []struct {
		name     string
		call     func(c *GatewayMemory) (PinResult, error)
		wantPath string
		respPin  bool
	}{
		{"pin", func(c *GatewayMemory) (PinResult, error) { return c.Pin(context.Background(), "m1", 3) }, "/memory/items/m1/pin", true},
		{"unpin", func(c *GatewayMemory) (PinResult, error) { return c.Unpin(context.Background(), "m1", 3) }, "/memory/items/m1/unpin", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var method, path string
			var ev any
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				method, path = r.Method, r.URL.Path
				ev = decodeBody[map[string]any](t, r)["expected_version"]
				_, _ = w.Write([]byte(`{"memory_id":"m1","version":4,"pinned":` + boolStr(tc.respPin) + `}`))
			})
			res, err := tc.call(c)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if method != "POST" || path != tc.wantPath {
				t.Errorf("method/path = %q %q, want POST %q", method, path, tc.wantPath)
			}
			if ev != float64(3) {
				t.Errorf("expected_version = %v", ev)
			}
			if res.Pinned != tc.respPin || res.Version != 4 {
				t.Errorf("result = %+v", res)
			}
		})
	}
}

func TestDisableEnable(t *testing.T) {
	cases := []struct {
		name        string
		call        func(c *GatewayMemory) (DisableResult, error)
		wantPath    string
		respDisable bool
	}{
		{"disable", func(c *GatewayMemory) (DisableResult, error) { return c.Disable(context.Background(), "m2", 5) }, "/memory/items/m2/disable", true},
		{"enable", func(c *GatewayMemory) (DisableResult, error) { return c.Enable(context.Background(), "m2", 5) }, "/memory/items/m2/enable", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var method, path string
			var ev any
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				method, path = r.Method, r.URL.Path
				ev = decodeBody[map[string]any](t, r)["expected_version"]
				_, _ = w.Write([]byte(`{"memory_id":"m2","version":6,"disabled":` + boolStr(tc.respDisable) + `}`))
			})
			res, err := tc.call(c)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if method != "POST" || path != tc.wantPath {
				t.Errorf("method/path = %q %q, want POST %q", method, path, tc.wantPath)
			}
			if ev != float64(5) {
				t.Errorf("expected_version = %v", ev)
			}
			if res.Disabled != tc.respDisable || res.Version != 6 {
				t.Errorf("result = %+v", res)
			}
		})
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func TestDeleteRouteBodyAndConsistency(t *testing.T) {
	t.Run("with consistency level", func(t *testing.T) {
		var method, path, ct string
		var body map[string]any
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			method, path, ct = r.Method, r.URL.Path, r.Header.Get("Content-Type")
			body = decodeBody[map[string]any](t, r)
			_, _ = w.Write([]byte(`{"memory_id":"m3","deleted":true,"version":7}`))
		})
		res, err := c.Delete(context.Background(), "m3", 6, ConsistencyBounded)
		if err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if method != "DELETE" || path != "/memory/items/m3" {
			t.Errorf("method/path = %q %q", method, path)
		}
		if ct != "application/json" {
			t.Errorf("DELETE must send Content-Type application/json, got %q", ct)
		}
		if body["expected_version"] != float64(6) {
			t.Errorf("expected_version = %v", body["expected_version"])
		}
		if body["consistency_level"] != "bounded" {
			t.Errorf("consistency_level = %v", body["consistency_level"])
		}
		if !res.Deleted || res.Version != 7 {
			t.Errorf("result = %+v", res)
		}
	})

	t.Run("omits consistency level when empty", func(t *testing.T) {
		var body map[string]any
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			body = decodeBody[map[string]any](t, r)
			_, _ = w.Write([]byte(`{"memory_id":"m3","deleted":true,"version":7}`))
		})
		if _, err := c.Delete(context.Background(), "m3", 6, ConsistencyDefault); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, ok := body["consistency_level"]; ok {
			t.Errorf("consistency_level should be omitted when empty")
		}
	})
}

func TestGetNoBodyNoContentType(t *testing.T) {
	var method, path string
	var hasCT, hasBody bool
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		_, hasCT = r.Header["Content-Type"]
		b := readRaw(t, r)
		hasBody = len(b) > 0
		_, _ = w.Write([]byte(`{"memory_id":"m4","kind":"fact","version":2,"content":"hi","source":"chat","category":"general","importance":0.3,"pinned":true,"disabled":false,"tags":["x"]}`))
	})
	item, err := c.Get(context.Background(), "m4")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if method != "GET" || path != "/memory/items/m4" {
		t.Errorf("method/path = %q %q", method, path)
	}
	if hasCT {
		t.Errorf("GET must not set Content-Type (no body)")
	}
	if hasBody {
		t.Errorf("GET must not send a body")
	}
	if item.MemoryID != "m4" || item.Version != 2 || item.Content != "hi" || !item.Pinned || item.Importance != 0.3 {
		t.Errorf("item decode mismatch: %+v", item)
	}
	if len(item.Tags) != 1 || item.Tags[0] != "x" {
		t.Errorf("tags decode mismatch: %+v", item.Tags)
	}
}

func TestItemPathEscaping(t *testing.T) {
	var rawPath, escapedPath string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		rawPath = r.URL.EscapedPath()
		escapedPath = r.URL.Path // server-decoded path
		_, _ = w.Write([]byte(`{"memory_id":"x","kind":"f","version":1,"content":"c","source":"s","category":"g","pinned":false,"disabled":false}`))
	})
	if _, err := c.Get(context.Background(), "mem/with space"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	// The wire path must carry the escaped segment, not a raw slash/space.
	if rawPath != "/memory/items/mem%2Fwith%20space" {
		t.Errorf("escaped wire path = %q, want /memory/items/mem%%2Fwith%%20space", rawPath)
	}
	// And the server decodes it back to the original single segment.
	if escapedPath != "/memory/items/mem/with space" {
		t.Errorf("decoded path = %q", escapedPath)
	}
}

func TestRetryableInterfaceAcrossErrorTypes(t *testing.T) {
	type retryabler interface{ Retryable() bool }

	// 503 retryable gateway error
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"code":"upstream_unavailable","message":"down","retryable":true}}`))
	})
	_, gerr := c.Get(context.Background(), "m1")
	var r1 retryabler
	if !errors.As(gerr, &r1) {
		t.Fatalf("errors.As(retryabler) failed for GatewayError: %v", gerr)
	}
	if !r1.Retryable() {
		t.Errorf("GatewayError from 503 should be retryable")
	}
	var ge *GatewayError
	if !errors.As(gerr, &ge) || ge.HTTPStatus != http.StatusServiceUnavailable {
		t.Errorf("expected *GatewayError 503, got %v", gerr)
	}

	// transport error
	srv.Close()
	c2, srv2 := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {})
	srv2.Close()
	_, terr := c2.Get(context.Background(), "m1")
	var r2 retryabler
	if !errors.As(terr, &r2) {
		t.Fatalf("errors.As(retryabler) failed for TransportError: %v", terr)
	}
	if !r2.Retryable() {
		t.Errorf("TransportError should be retryable")
	}
}

func TestContextCancellationIsTransportError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Get(ctx, "m1")
	var te *TransportError
	if !errors.As(err, &te) {
		t.Fatalf("expected *TransportError on cancellation, got %T: %v", err, err)
	}
}

func TestTwoXXNonJSONBodyIsTransportError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html>not json</html>`))
	})
	_, err := c.Get(context.Background(), "m1")
	var te *TransportError
	if !errors.As(err, &te) {
		t.Fatalf("expected *TransportError decoding non-JSON 2xx, got %T: %v", err, err)
	}
	if te.Op != "decode response" {
		t.Errorf("Op = %q, want decode response", te.Op)
	}
}

func TestInternalErrorCodeIsGatewayErrorNoSentinel(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":"internal_error","message":"boom","request_id":"r9","retryable":false}}`))
	})
	_, err := c.Get(context.Background(), "m1")

	var ge *GatewayError
	if !errors.As(err, &ge) {
		t.Fatalf("expected *GatewayError, got %T", err)
	}
	if ge.Code != "internal_error" || ge.RequestID != "r9" {
		t.Errorf("GatewayError = %+v", ge)
	}
	// matches no sentinel
	for _, s := range []error{ErrBadRequest, ErrUnauthorized, ErrForbidden, ErrNotFound, ErrConflict, ErrIdempotencyConflict, ErrUnavailable} {
		if errors.Is(err, s) {
			t.Errorf("internal_error should not match sentinel %v", s)
		}
	}
}
