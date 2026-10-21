package hub

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/topology"
)

//go:embed web
var web embed.FS

const (
	uiCookie = "hikari_ui"
	maxWait  = 60 * time.Second
)

// Handler serves the node API and the web UI. Node requests authenticate with
// a node token. The UI authenticates with uiToken, so that agents, which can
// reach the hub, cannot read the full log or speak as the user.
func (h *Hub) Handler(uiToken string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/node/self", h.node(func(w http.ResponseWriter, r *http.Request, node string) {
		writeJSON(w, http.StatusOK, h.Self(node))
	}))
	mux.HandleFunc("POST /api/node/send", h.node(func(w http.ResponseWriter, r *http.Request, node string) {
		h.send(w, r, node)
	}))
	mux.HandleFunc("GET /api/node/inbox", h.node(func(w http.ResponseWriter, r *http.Request, node string) {
		wait, err := time.ParseDuration(r.URL.Query().Get("wait"))
		if err != nil || wait < 0 {
			wait = 0
		}
		msgs := h.Take(r.Context(), node, min(wait, maxWait))
		if r.Context().Err() != nil {
			h.Requeue(node, msgs)
			return
		}
		if msgs == nil {
			msgs = []api.Message{}
		}
		writeJSON(w, http.StatusOK, msgs)
	}))
	mux.HandleFunc("PUT /api/node/status", h.node(func(w http.ResponseWriter, r *http.Request, node string) {
		var s api.Status
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil || s.State == "" {
			writeError(w, http.StatusBadRequest, "body must be a status with a state")
			return
		}
		h.SetStatus(node, s)
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
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		if t := r.URL.Query().Get("token"); t != "" && equal(t, uiToken) {
			http.SetCookie(w, &http.Cookie{
				Name: uiCookie, Value: uiToken, Path: "/",
				HttpOnly: true, SameSite: http.SameSiteStrictMode,
			})
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		ui(func(w http.ResponseWriter, r *http.Request) {
			http.ServeFileFS(w, r, web, "web/index.html")
		})(w, r)
	})
	mux.HandleFunc("GET /api/ui/events", ui(h.events))
	mux.HandleFunc("POST /api/ui/send", ui(func(w http.ResponseWriter, r *http.Request) {
		h.send(w, r, topology.User)
	}))

	return mux
}

func (h *Hub) node(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		node, ok := h.nodeForToken(token)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unknown node token")
			return
		}
		next(w, r, node)
	}
}

func (h *Hub) send(w http.ResponseWriter, r *http.Request, from string) {
	var req api.SendRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2*maxBodyBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "body must be {\"to\": ..., \"body\": ...}")
		return
	}
	m, err := h.Send(from, req.To, req.Body)
	switch {
	case errors.Is(err, errNotNeighbor):
		writeError(w, http.StatusForbidden, err.Error())
	case err != nil:
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeJSON(w, http.StatusOK, m)
	}
}

func (h *Hub) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming is not supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

	events, cancel := h.Subscribe()
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
