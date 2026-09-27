// Package web serves the local "observatory" UI over loopback only, with
// escaped output and CSRF-protected writable endpoints.
package web

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/amphoze/asaman/internal/adapters"
	"github.com/amphoze/asaman/internal/core"
	"github.com/amphoze/asaman/internal/memory"
	"github.com/amphoze/asaman/internal/meta"
	"github.com/amphoze/asaman/internal/sessions"
)

// Server holds runtime handles and the per-instance CSRF token.
type Server struct {
	db        *core.DB
	ads       []adapters.Adapter
	metaStore string
	lock      string
	csrf      string
	refs      map[string]adapters.SessionRef
}

// NewServer builds a server and indexes the corpus once.
func NewServer(db *core.DB, ads []adapters.Adapter, facts []memory.Fact, metaStore, lock string) *Server {
	buf := make([]byte, 16)
	rand.Read(buf)
	s := &Server{db: db, ads: ads, metaStore: metaStore, lock: lock,
		csrf: hex.EncodeToString(buf), refs: map[string]adapters.SessionRef{}}
	sessions.Reindex(db, ads, facts)
	for _, a := range ads {
		if rs, err := a.Sessions(); err == nil {
			for _, r := range rs {
				s.refs[r.ID] = r
			}
		}
	}
	return s
}

// isLoopback reports whether addr binds only the loopback interface.
func isLoopback(addr string) bool {
	host := addr
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		host = addr[:i]
	}
	host = strings.Trim(host, "[]")
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}

// Serve refuses any non-loopback bind address, then serves.
func Serve(addr string, db *core.DB, ads []adapters.Adapter, facts []memory.Fact, metaStore, lock string) error {
	if !isLoopback(addr) {
		return fmt.Errorf("web: refusing non-loopback bind %q", addr)
	}
	s := NewServer(db, ads, facts, metaStore, lock)
	fmt.Printf("asaman observatory on http://%s (csrf ready)\n", addr)
	return http.ListenAndServe(addr, s.Handler())
}

// Handler returns the routed handler (exposed for tests).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/search", s.handleSearch)
	mux.HandleFunc("/session/", s.handleSession)
	mux.HandleFunc("/api/meta", s.handleMeta)
	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	page.Execute(w, map[string]any{"CSRF": s.csrf})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	hits, err := sessions.Search(s.db, q, sessions.Opts{Agent: r.URL.Query().Get("agent")})
	if err != nil {
		http.Error(w, "search error", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hits)
}

// handleSession renders a transcript with html/template auto-escaping so
// untrusted event text (which may contain <script>) is rendered inert.
func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/session/")
	ref, ok := s.refs[id]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	detail.Execute(w, ref)
}

// handleMeta accepts favourite/tag/note writes; requires same-origin + CSRF.
func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("X-CSRF-Token") != s.csrf {
		http.Error(w, "bad csrf token", http.StatusForbidden)
		return
	}
	// Reject cross-site requests: if an Origin/Sec-Fetch-Site is present it must
	// be same-origin. (Same-origin browser requests may omit Origin.)
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		http.Error(w, "cross-site rejected", http.StatusForbidden)
		return
	}
	var body struct{ Key, Field, Op, Value string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	if _, err := meta.Append(s.metaStore, meta.Record{
		Actor: "ui", Key: body.Key, Field: body.Field, Op: body.Op, Value: body.Value}); err != nil {
		http.Error(w, "write failed", 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CSRF exposes the token (tests).
func (s *Server) CSRF() string { return s.csrf }

var detail = template.Must(template.New("d").Parse(
	`<!doctype html><meta charset="utf-8"><title>{{.Title}}</title>
<body style="background:#0E1526;color:#E9ECF5;font-family:Inter,system-ui">
<h1>{{.Title}}</h1>
{{range .Events}}<div class="evt"><b>{{.Role}}</b>: {{.Text}}</div>{{end}}
</body>`))

var page = template.Must(template.New("p").Parse(indexHTML))
