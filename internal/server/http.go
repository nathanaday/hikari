package server

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/sysmap"
	"github.com/nathanaday/hikari/internal/systems"
	"github.com/nathanaday/hikari/internal/topology"
)

const (
	uiCookie = "hikari_ui"
	maxWait  = 60 * time.Second
)

// Handler serves the node API and the web UI. Node requests authenticate with
// a node token. The UI authenticates with uiToken, so that agents, which can
// reach the server, cannot read the full log or speak as the user.
func (s *Server) Handler(uiToken string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/node/self", s.node(func(w http.ResponseWriter, r *http.Request, node string) {
		writeJSON(w, http.StatusOK, s.Self(node))
	}))
	mux.HandleFunc("POST /api/node/send", s.node(func(w http.ResponseWriter, r *http.Request, node string) {
		s.send(w, r, node)
	}))
	mux.HandleFunc("GET /api/node/inbox", s.node(func(w http.ResponseWriter, r *http.Request, node string) {
		wait, err := time.ParseDuration(r.URL.Query().Get("wait"))
		if err != nil || wait < 0 {
			wait = 0
		}
		msgs := s.Take(r.Context(), node, min(wait, maxWait))
		if r.Context().Err() != nil {
			s.Requeue(node, msgs)
			return
		}
		if msgs == nil {
			msgs = []api.Message{}
		}
		writeJSON(w, http.StatusOK, msgs)
	}))
	mux.HandleFunc("PUT /api/node/status", s.node(func(w http.ResponseWriter, r *http.Request, node string) {
		var st api.Status
		if err := json.NewDecoder(r.Body).Decode(&st); err != nil || st.State == "" {
			writeError(w, http.StatusBadRequest, "body must be a status with a state")
			return
		}
		s.SetStatus(node, st)
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("POST /api/node/activity", s.node(func(w http.ResponseWriter, r *http.Request, node string) {
		var a api.Activity
		if err := json.NewDecoder(r.Body).Decode(&a); err != nil || a.ID == 0 || a.Kind == "" {
			writeError(w, http.StatusBadRequest, "body must be an activity entry with an id and a kind")
			return
		}
		s.AddActivity(node, a)
		w.WriteHeader(http.StatusNoContent)
	}))

	s.systemRoutes(mux)

	ui := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(uiCookie)
			if err != nil || !equal(c.Value, uiToken) {
				writeError(w, http.StatusUnauthorized, "open the URL that hikari printed at startup")
				return
			}
			next(w, r)
		}
	}
	login(mux, uiToken)
	mux.HandleFunc("GET /api/ui/session", ui(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("GET /api/ui/events", ui(s.events))
	s.storeRoutes(mux, ui)
	// The user's changes to the wikis (rules U6, U8, and C4 of the wikis
	// spec). {ref} is public or system:<name>.
	userWrite := func(fn func(r *http.Request, ref string) error) http.HandlerFunc {
		return ui(func(w http.ResponseWriter, r *http.Request) {
			if err := fn(r, r.PathValue("ref")); err != nil {
				writeSystemError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	}
	mux.HandleFunc("PUT /api/ui/wiki/{ref}/docs/{id}", userWrite(func(r *http.Request, ref string) error {
		var req struct {
			Text string `json:"text"`
		}
		if err := decode(r, &req); err != nil {
			return err
		}
		return s.sys.WriteDocument(ref, r.PathValue("id"), req.Text)
	}))
	mux.HandleFunc("DELETE /api/ui/wiki/{ref}/docs/{id}", userWrite(func(r *http.Request, ref string) error {
		return s.sys.RemoveDocument(ref, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /api/ui/send", ui(func(w http.ResponseWriter, r *http.Request) {
		s.send(w, r, topology.User)
	}))

	static(mux)
	return mux
}

// systemRoutes serves the owner, orchestrator, and vocabulary endpoints. The
// systems manager decides who may call each one.
func (s *Server) systemRoutes(mux *http.ServeMux) {
	sys := s.sys
	handle := func(pattern string, fn func(r *http.Request, node string) (any, error)) {
		mux.HandleFunc(pattern, s.node(func(w http.ResponseWriter, r *http.Request, node string) {
			out, err := fn(r, node)
			if err != nil {
				writeSystemError(w, err)
				return
			}
			if out == nil {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			writeJSON(w, http.StatusOK, out)
		}))
	}

	handle("GET /api/node/map", func(r *http.Request, node string) (any, error) {
		return sys.OwnView(node)
	})
	handle("PATCH /api/node/map/system", func(r *http.Request, node string) (any, error) {
		var p sysmap.SystemPatch
		if err := decode(r, &p); err != nil {
			return nil, err
		}
		return nil, sys.SetSystemInfo(node, p)
	})
	handle("POST /api/node/wiki", func(r *http.Request, node string) (any, error) {
		var req api.ChangeRequest
		if err := decode(r, &req); err != nil {
			return nil, err
		}
		return sys.ChangeWiki(node, req.Public, req.Writes)
	})
	handle("GET /api/node/wiki/docs/{id}", func(r *http.Request, node string) (any, error) {
		return sys.GetDocument(node, r.PathValue("id"))
	})
	handle("GET /api/node/wiki/search", func(r *http.Request, node string) (any, error) {
		return sys.SearchWiki(node, r.URL.Query().Get("q"))
	})
	handle("POST /api/node/map/tests", func(r *http.Request, node string) (any, error) {
		var req api.RunRequest
		if err := decode(r, &req); err != nil {
			return nil, err
		}
		return sys.RecordRun(node, req.Suite)
	})
	handle("POST /api/node/map/current", func(r *http.Request, node string) (any, error) {
		commit, err := sys.MarkCurrent(node)
		return api.CommitResponse{Commit: commit}, err
	})
	handle("POST /api/node/commit", func(r *http.Request, node string) (any, error) {
		var req api.CommitRequest
		if err := decode(r, &req); err != nil {
			return nil, err
		}
		commit, err := sys.Commit(node, req.Message)
		return api.CommitResponse{Commit: commit}, err
	})
	handle("POST /api/node/releases/check", func(r *http.Request, node string) (any, error) {
		var req api.CheckRequest
		if err := decode(r, &req); err != nil {
			return nil, err
		}
		return sys.CheckRelease(node, req.Deliverable)
	})
	handle("POST /api/node/releases", func(r *http.Request, node string) (any, error) {
		var req api.PublishRequest
		if err := decode(r, &req); err != nil {
			return nil, err
		}
		rel, err := sys.PublishRelease(node, req)
		if err != nil {
			return nil, err
		}
		body := fmt.Sprintf("Released %s %s %s: %s", node, rel.Deliverable, rel.Version, rel.Notes)
		if _, err := s.Send(node, api.SendRequest{Body: body}); err != nil {
			return nil, err
		}
		return rel, nil
	})
	handle("GET /api/node/turn-context", func(r *http.Request, node string) (any, error) {
		return sys.TurnContext(node), nil
	})

	handle("GET /api/node/systems", func(r *http.Request, node string) (any, error) {
		return sys.ListSystems(node)
	})
	handle("GET /api/node/systems/{name}", func(r *http.Request, node string) (any, error) {
		return sys.GetSystem(node, r.PathValue("name"))
	})

	handle("GET /api/node/vocabulary", func(r *http.Request, node string) (any, error) {
		return sys.Vocabulary(), nil
	})
	handle("POST /api/node/vocabulary", func(r *http.Request, node string) (any, error) {
		var ch api.VocabularyChange
		if err := decode(r, &ch); err != nil {
			return nil, err
		}
		switch {
		case ch.Op == "define_term" && ch.Term != nil:
			return nil, sys.DefineTerm(node, *ch.Term)
		case ch.Op == "define_interface" && ch.Interface != nil:
			return nil, sys.DefineInterface(node, *ch.Interface)
		case ch.Op == "remove":
			return nil, sys.RemoveEntry(node, ch.Name)
		}
		return nil, errors.New(`op must be "define_term" with a term, "define_interface" with an interface, or "remove" with a name`)
	})
}

func decode(r *http.Request, v any) error {
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(v); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}

func writeSystemError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, systems.ErrForbidden):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, systems.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		writeError(w, http.StatusBadRequest, err.Error())
	}
}

func (s *Server) node(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		node, ok := s.nodeForToken(token)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unknown node token")
			return
		}
		next(w, r, node)
	}
}

func (s *Server) send(w http.ResponseWriter, r *http.Request, from string) {
	var req api.SendRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2*maxBodyBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "body must be {\"body\": ..., and \"to\" for a direct message}")
		return
	}
	m, err := s.Send(from, req)
	switch {
	case errors.Is(err, errForbidden):
		writeError(w, http.StatusForbidden, err.Error())
	case err != nil:
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeJSON(w, http.StatusOK, m)
	}
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming is not supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

	events, cancel := s.Subscribe()
	defer cancel()
	keepalive := time.NewTicker(20 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case e, ok := <-events:
			if !ok {
				return
			}
			writeEvent(w, e)
		case <-keepalive.C:
			w.Write([]byte(": keepalive\n\n"))
		case <-r.Context().Done():
			return
		}
		flusher.Flush()
	}
}

func writeEvent(w http.ResponseWriter, e Event) {
	data, _ := json.Marshal(e)
	w.Write([]byte("data: "))
	w.Write(data)
	w.Write([]byte("\n\n"))
}

func readFile(path string) ([]byte, error) { return os.ReadFile(path) }

func equal(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
