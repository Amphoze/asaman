package web

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amphoze/asaman/internal/adapters"
	"github.com/amphoze/asaman/internal/core"
	"github.com/amphoze/asaman/internal/meta"
)

type fakeAdapter struct{ refs []adapters.SessionRef }

func (f *fakeAdapter) Name() string                             { return "fake" }
func (f *fakeAdapter) Sessions() ([]adapters.SessionRef, error) { return f.refs, nil }
func (f *fakeAdapter) TimeLog() []string                        { return nil }
func (f *fakeAdapter) ContextPath() string                      { return "" }
func (f *fakeAdapter) MemoryMode() adapters.MemoryMode          { return adapters.ModeNone }
func (f *fakeAdapter) SyncIndex(string) error                   { return nil }

func testServer(t *testing.T, store string) *Server {
	t.Helper()
	db, _ := core.Open(":memory:")
	db.InitSchema()
	ad := &fakeAdapter{refs: []adapters.SessionRef{{
		ID: "claude:x", Agent: "claude", Title: "danger", Kind: "session",
		Events: []adapters.Event{{Idx: 0, Role: "user", Text: "<script>alert(1)</script>"}},
	}}}
	s, err := NewServer(db, []adapters.Adapter{ad}, nil, store, store+".lock")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// F5: unknown field/op is rejected with 400 (not silently folded).
func TestMetaValidation(t *testing.T) {
	store := filepath.Join(t.TempDir(), "m.jsonl")
	s := testServer(t, store)
	h := s.Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/meta",
		strings.NewReader(`{"Key":"claude:x","Field":"bogus","Op":"set","Value":"1"}`))
	req.Header.Set("X-CSRF-Token", s.CSRF())
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid field POST = %d want 400", rec.Code)
	}
}

// GATE: loopback bind only.
func TestServeRefusesNonLoopback(t *testing.T) {
	db, _ := core.Open(":memory:")
	db.InitSchema()
	err := Serve("0.0.0.0:0", db, nil, nil, filepath.Join(t.TempDir(), "m.jsonl"), "")
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("expected loopback refusal, got %v", err)
	}
}

// GATE: transcript rendering escapes untrusted content.
func TestSessionEscapesXSS(t *testing.T) {
	s := testServer(t, filepath.Join(t.TempDir(), "m.jsonl"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/session/claude:x", nil)
	s.Handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Errorf("raw script tag leaked (XSS):\n%s", body)
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Errorf("script not escaped:\n%s", body)
	}
}

// GATE: writable endpoint requires CSRF + same-origin.
func TestMetaCSRF(t *testing.T) {
	store := filepath.Join(t.TempDir(), "m.jsonl")
	s := testServer(t, store)
	h := s.Handler()

	// no token → 403
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/meta", strings.NewReader(`{"Key":"claude:x","Field":"fav","Op":"set","Value":"1"}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("no-token POST = %d want 403", rec.Code)
	}

	// cross-site with token → 403
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/meta", strings.NewReader(`{"Key":"claude:x","Field":"fav","Op":"set","Value":"1"}`))
	req.Header.Set("X-CSRF-Token", s.CSRF())
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-site POST = %d want 403", rec.Code)
	}

	// valid token + same-origin → 204 and persisted
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/meta", strings.NewReader(`{"Key":"claude:x","Field":"fav","Op":"set","Value":"1"}`))
	req.Header.Set("X-CSRF-Token", s.CSRF())
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("valid POST = %d want 204", rec.Code)
	}
	st, _ := meta.Fold(store)
	if !st["claude:x"].Fav {
		t.Error("favourite not persisted via UI endpoint")
	}
}
