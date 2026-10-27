package server

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/editor"
	"github.com/nathanaday/hikari/internal/linked"
	"github.com/nathanaday/hikari/internal/topoedit"
	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/internal/version"
	"github.com/nathanaday/hikari/web"
)

// The modes of hikari up (section 8 of the topology API spec).
const (
	ModeEditing  = "editing"
	ModeStarting = "starting"
	ModeRunning  = "running"
	ModeStopping = "stopping"
)

// Host serves the UI for the life of hikari up. In every mode it serves
// sign-in, the web UI, the topology API, and the run endpoints; while
// agents run, it passes every other request to the run's server (rule
// UP3).
type Host struct {
	uiToken string
	editor  *editor.Editor
	// Start begins the start sequence and Stop the stop sequence. Each
	// runs in the background and ends with Running or Editing.
	Start func()
	Stop  func()

	mu       sync.Mutex
	mode     string
	run      *Server
	handler  http.Handler
	findings []topology.Finding
	// streams are closed when the mode changes (decision 7).
	streams map[chan struct{}]struct{}
	// revisions reach the event streams of the editing state.
	revisions map[chan string]struct{}
}

func NewHost(uiToken string, ed *editor.Editor) *Host {
	return &Host{uiToken: uiToken, editor: ed, mode: ModeEditing,
		streams: map[chan struct{}]struct{}{}, revisions: map[chan string]struct{}{}}
}

// Mode returns the current mode.
func (h *Host) Mode() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.mode
}

func (h *Host) setMode(mode string, run *Server, findings []topology.Finding) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.mode, h.run, h.findings = mode, run, findings
	h.handler = nil
	if run != nil {
		h.handler = run.Handler(h.uiToken)
	}
	for ch := range h.streams {
		close(ch)
		delete(h.streams, ch)
	}
}

// Starting and Stopping mark the moments between the modes.
func (h *Host) Starting() { h.setMode(ModeStarting, nil, nil) }
func (h *Host) Stopping() { h.setMode(ModeStopping, nil, nil) }

// Attach passes the node API to a run's server while it starts, so that
// the agents can reach it before the UI moves to the running mode.
func (h *Host) Attach(s *Server) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.run, h.handler = s, s.Handler(h.uiToken)
}

// Running hands the UI to a run's server. c commits the editor's saves
// while it runs (rule TC4).
func (h *Host) Running(s *Server, c editor.Committer) {
	h.editor.SetRun(true, c)
	h.setMode(ModeRunning, s, nil)
}

// Editing returns to the editing state, with the findings that stopped a
// start, if any (rule UP4).
func (h *Host) Editing(findings []topology.Finding) {
	h.editor.SetRun(false, nil)
	h.setMode(ModeEditing, nil, findings)
}

// TopologyChanged tells every open UI that the compose file changed on
// disk (rule TA8).
func (h *Host) TopologyChanged(revision string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.run != nil {
		h.run.PublishTopology(revision)
	}
	for ch := range h.revisions {
		select {
		case ch <- revision:
		default:
		}
	}
}

func (h *Host) Handler() http.Handler {
	mux := http.NewServeMux()
	ui := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(uiCookie)
			if err != nil || !equal(c.Value, h.uiToken) {
				writeError(w, http.StatusUnauthorized, "open the URL that hikari printed at startup")
				return
			}
			next(w, r)
		}
	}
	login(mux, h.uiToken)
	mux.HandleFunc("GET /api/ui/session", ui(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	h.editor.Routes(mux, ui)

	mux.HandleFunc("POST /api/ui/run/start", ui(func(w http.ResponseWriter, r *http.Request) {
		if mode := h.Mode(); mode != ModeEditing {
			writeError(w, http.StatusConflict, "hikari is "+mode)
			return
		}
		text, err := readFile(h.editor.File)
		if err != nil {
			writeError(w, http.StatusBadRequest, "the compose file does not exist; create it first")
			return
		}
		if fs := h.editor.Lint(r.Context(), text); editor.HasError(fs) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "the compose file has errors; fix them first", "findings": fs})
			return
		}
		h.Starting()
		go h.Start()
		w.WriteHeader(http.StatusAccepted)
	}))
	mux.HandleFunc("POST /api/ui/run/stop", ui(func(w http.ResponseWriter, r *http.Request) {
		if mode := h.Mode(); mode != ModeRunning {
			writeError(w, http.StatusConflict, "hikari is "+mode)
			return
		}
		h.Stopping()
		go h.Stop()
		w.WriteHeader(http.StatusAccepted)
	}))

	mux.HandleFunc("GET /api/ui/events", ui(h.events))

	// Every other API endpoint belongs to the run (rule UP3).
	forward := func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		handler := h.handler
		h.mu.Unlock()
		if handler == nil {
			writeError(w, http.StatusServiceUnavailable, "hikari is not running agents")
			return
		}
		handler.ServeHTTP(w, r)
	}
	for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		mux.HandleFunc(method+" /api/", forward)
	}
	static(mux)
	return mux
}

// events serves the run's event stream while agents run, and the editing
// state's otherwise. Each stream ends when the mode changes, and the
// browser opens it again (decision 7).
func (h *Host) events(w http.ResponseWriter, r *http.Request) {
	closed := make(chan struct{})
	h.mu.Lock()
	h.streams[closed] = struct{}{}
	handler, mode, findings := h.handler, h.mode, h.findings
	h.mu.Unlock()
	if mode != ModeRunning {
		handler = nil
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		select {
		case <-closed:
			cancel()
		case <-ctx.Done():
		}
	}()
	if handler != nil {
		handler.ServeHTTP(w, r.WithContext(ctx))
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming is not supported")
		return
	}
	revisions := make(chan string, 8)
	h.mu.Lock()
	h.revisions[revisions] = struct{}{}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.revisions, revisions)
		h.mu.Unlock()
	}()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	snap := h.editingSnapshot(mode, findings)
	writeEvent(w, Event{Type: "snapshot", Snapshot: &snap})
	flusher.Flush()
	keepalive := time.NewTicker(20 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case rev := <-revisions:
			writeEvent(w, Event{Type: "topology", Revision: rev})
		case <-keepalive.C:
			w.Write([]byte(": keepalive\n\n"))
		case <-ctx.Done():
			return
		}
		flusher.Flush()
	}
}

// editingSnapshot is the snapshot of a mode without agents: no agents, no
// participants, and no messages.
func (h *Host) editingSnapshot(mode string, findings []topology.Finding) Snapshot {
	snap := Snapshot{
		Version: version.Version, Mode: mode, Me: topology.User, File: linked.Shown(h.editor.File),
		Agents: []NodeView{}, Participants: []api.Peer{}, Messages: []api.Message{}, Systems: []api.SystemView{},
		Repos: []api.RepoInfo{}, Deliverables: []deliverableEntry{}, Wikis: []api.WikiView{},
		Activity: map[string][]api.Activity{}, StartFindings: findings,
	}
	if text, err := readFile(h.editor.File); err == nil {
		snap.Revision = editor.Revision(text, true)
		if m := topoedit.ReadModel(text, ""); m != nil {
			snap.Project = m.Project.Name
		}
	}
	return snap
}

// login serves the sign-in: the UI posts the token from the URL that
// hikari printed, and gets a cookie for the later requests.
func login(mux *http.ServeMux, uiToken string) {
	mux.HandleFunc("POST /api/ui/login", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Token string `json:"token"`
		}
		if decode(r, &req) != nil || !equal(req.Token, uiToken) {
			writeError(w, http.StatusUnauthorized, "the token is not valid for this run of hikari")
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name: uiCookie, Value: uiToken, Path: "/",
			HttpOnly: true, SameSite: http.SameSiteStrictMode,
		})
		w.WriteHeader(http.StatusNoContent)
	})
}

// static serves the UI files, which hold no data and need no token.
func static(mux *http.ServeMux) {
	if web.Built() {
		mux.Handle("GET /", http.FileServerFS(web.Assets()))
		return
	}
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "The web UI is not built. Run `make ui`, then rebuild hikari.", http.StatusNotFound)
	})
}
