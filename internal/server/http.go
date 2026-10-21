package server

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/nathanaday/hikari/internal/api"
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
