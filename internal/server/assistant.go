package server

import (
	"net/http"
	"strings"

	"github.com/nathanaday/hikari/internal/assistant"
)

// assistantRoutes serve the panel of the hikari agent (rule HA9 of the
// hikari agent spec). Each needs the UI's cookie, and answers 503 when the
// run has no hikari agent.
func (s *Server) assistantRoutes(mux *http.ServeMux, ui func(http.HandlerFunc) http.HandlerFunc) {
	with := func(fn func(w http.ResponseWriter, r *http.Request, a *assistant.Assistant)) http.HandlerFunc {
		return ui(func(w http.ResponseWriter, r *http.Request) {
			s.mu.Lock()
			a, off := s.assistant, s.assistantOff
			s.mu.Unlock()
			if a == nil {
				writeError(w, http.StatusServiceUnavailable, off)
				return
			}
			fn(w, r, a)
		})
	}
	// refused answers 409 when the hikari agent is busy, and 400 otherwise.
	refused := func(w http.ResponseWriter, err error) {
		if strings.Contains(err.Error(), "working") {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
	}
	mux.HandleFunc("POST /api/ui/assistant/send", with(func(w http.ResponseWriter, r *http.Request, a *assistant.Assistant) {
		var req struct {
			Text string `json:"text"`
		}
		if err := decode(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := a.Send(req.Text); err != nil {
			refused(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("POST /api/ui/assistant/answer", with(func(w http.ResponseWriter, r *http.Request, a *assistant.Assistant) {
		var req struct {
			ID      string `json:"id"`
			Allow   bool   `json:"allow"`
			Message string `json:"message"`
		}
		if err := decode(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := a.Answer(req.ID, req.Allow, req.Message); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("POST /api/ui/assistant/stop", with(func(w http.ResponseWriter, r *http.Request, a *assistant.Assistant) {
		a.Stop()
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("POST /api/ui/assistant/new", with(func(w http.ResponseWriter, r *http.Request, a *assistant.Assistant) {
		if err := a.NewChat(); err != nil {
			refused(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("PUT /api/ui/assistant/options", with(func(w http.ResponseWriter, r *http.Request, a *assistant.Assistant) {
		var o assistant.Options
		if err := decode(r, &o); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := a.SetOptions(o); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}
