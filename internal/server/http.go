package server

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/sysmap"
	"github.com/nathanaday/hikari/internal/systems"
	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/web"
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
	// The UI posts the token from the URL that hikari printed, and gets a
	// cookie for the later requests.
	mux.HandleFunc("POST /api/ui/login", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Token string `json:"token"`
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil || !equal(req.Token, uiToken) {
			writeError(w, http.StatusUnauthorized, "the token is not valid for this run of hikari")
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name: uiCookie, Value: uiToken, Path: "/",
			HttpOnly: true, SameSite: http.SameSiteStrictMode,
		})
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/ui/session", ui(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("GET /api/ui/events", ui(s.events))
	mux.HandleFunc("POST /api/ui/send", ui(func(w http.ResponseWriter, r *http.Request) {
		s.send(w, r, topology.User)
	}))

	// The UI files hold no data, so they need no token.
	if web.Built() {
		mux.Handle("GET /", http.FileServerFS(web.Assets()))
	} else {
		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "The web UI is not built. Run `make ui`, then rebuild hikari.", http.StatusNotFound)
		})
	}

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
	handle("PUT /api/node/map/features", func(r *http.Request, node string) (any, error) {
		var fs []sysmap.FeatureInput
		if err := decode(r, &fs); err != nil {
			return nil, err
		}
		return nil, sys.UpsertFeatures(node, fs)
	})
	handle("DELETE /api/node/map/features/{id}", func(r *http.Request, node string) (any, error) {
		return nil, sys.RemoveFeature(node, r.PathValue("id"))
	})
	handle("POST /api/node/map/tests", func(r *http.Request, node string) (any, error) {
		var results []sysmap.TestResult
		if err := decode(r, &results); err != nil {
			return nil, err
		}
		return nil, sys.RecordTests(node, results)
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
	handle("POST /api/node/releases", func(r *http.Request, node string) (any, error) {
		var req api.PublishRequest
		if err := decode(r, &req); err != nil {
			return nil, err
		}
		if strings.TrimSpace(req.Notes) == "" {
			return nil, errors.New("give release notes: what changed in this version")
		}
		rel, hubs, err := sys.PublishRelease(node, req.Version, req.Notes)
		if err != nil {
			return nil, err
		}
		for _, hub := range hubs {
			body := fmt.Sprintf("Released %s %s: %s", node, rel.Version, req.Notes)
			if _, err := s.Send(node, api.SendRequest{Hub: hub, Body: body}); err != nil {
				return nil, err
			}
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
	handle("GET /api/node/systems/{name}/features/{id}", func(r *http.Request, node string) (any, error) {
		return sys.GetFeature(node, r.PathValue("name"), r.PathValue("id"))
	})
	handle("GET /api/node/features", func(r *http.Request, node string) (any, error) {
		return sys.SearchFeatures(node, r.URL.Query().Get("q"))
	})

	handle("GET /api/node/vocabulary/{hub}", func(r *http.Request, node string) (any, error) {
		return sys.GetVocabulary(node, r.PathValue("hub"))
	})
	handle("POST /api/node/vocabulary/{hub}", func(r *http.Request, node string) (any, error) {
		var ch api.VocabularyChange
		if err := decode(r, &ch); err != nil {
			return nil, err
		}
		hub := r.PathValue("hub")
		switch {
		case ch.Op == "define_term" && ch.Term != nil:
			return nil, sys.DefineTerm(node, hub, *ch.Term)
		case ch.Op == "define_interface" && ch.Interface != nil:
			return nil, sys.DefineInterface(node, hub, *ch.Interface)
		case ch.Op == "remove":
			return nil, sys.RemoveEntry(node, hub, ch.Name)
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
		writeError(w, http.StatusBadRequest, "body must be {\"hub\" or \"to\": ..., \"body\": ...}")
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
			data, _ := json.Marshal(e)
			w.Write([]byte("data: "))
			w.Write(data)
			w.Write([]byte("\n\n"))
		case <-keepalive.C:
			w.Write([]byte(": keepalive\n\n"))
		case <-r.Context().Done():
			return
		}
		flusher.Flush()
	}
}

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
