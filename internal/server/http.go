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
	"github.com/nathanaday/hikari/internal/featuremap"
	"github.com/nathanaday/hikari/internal/partitions"
	"github.com/nathanaday/hikari/internal/topology"
)

const (
	// UICookie carries the UI's token; hikari send sets it too.
	UICookie = "hikari_ui"
	maxWait  = 60 * time.Second
)

// Handler serves the agent API and the web UI. Node requests authenticate with
// an agent token. The UI authenticates with uiToken, so that agents, which can
// reach the server, cannot read the full log or speak as the user.
func (s *Server) Handler(uiToken string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/agent/self", s.agent(func(w http.ResponseWriter, r *http.Request, agent string) {
		writeJSON(w, http.StatusOK, s.Self(agent))
	}))
	mux.HandleFunc("POST /api/agent/send", s.agent(func(w http.ResponseWriter, r *http.Request, agent string) {
		s.send(w, r, agent)
	}))
	mux.HandleFunc("GET /api/agent/inbox", s.agent(func(w http.ResponseWriter, r *http.Request, agent string) {
		wait, err := time.ParseDuration(r.URL.Query().Get("wait"))
		if err != nil || wait < 0 {
			wait = 0
		}
		msgs := s.Take(r.Context(), agent, min(wait, maxWait))
		if r.Context().Err() != nil {
			s.Requeue(agent, msgs)
			return
		}
		if msgs == nil {
			msgs = []api.Message{}
		}
		writeJSON(w, http.StatusOK, msgs)
	}))
	mux.HandleFunc("PUT /api/agent/status", s.agent(func(w http.ResponseWriter, r *http.Request, agent string) {
		var st api.Status
		if err := json.NewDecoder(r.Body).Decode(&st); err != nil || st.State == "" {
			writeError(w, http.StatusBadRequest, "body must be a status with a state")
			return
		}
		s.SetStatus(agent, st)
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("POST /api/agent/activity", s.agent(func(w http.ResponseWriter, r *http.Request, agent string) {
		var a api.Activity
		if err := json.NewDecoder(r.Body).Decode(&a); err != nil || a.ID == 0 || a.Kind == "" {
			writeError(w, http.StatusBadRequest, "body must be an activity entry with an id and a kind")
			return
		}
		s.AddActivity(agent, a)
		w.WriteHeader(http.StatusNoContent)
	}))

	s.partitionRoutes(mux)

	ui := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(UICookie)
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
	s.vaultRoutes(mux, ui)
	// The user's changes to the wikis (rules U6, U8, and C4 of the wikis
	// spec). {ref} is public or partition:<name>.
	userWrite := func(fn func(r *http.Request, ref string) error) http.HandlerFunc {
		return ui(func(w http.ResponseWriter, r *http.Request) {
			if err := fn(r, r.PathValue("ref")); err != nil {
				writePartitionError(w, err)
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

// partitionRoutes serves the partition agent, orchestration agent, and vocabulary endpoints. The
// partitions manager decides who may call each one.
func (s *Server) partitionRoutes(mux *http.ServeMux) {
	sys := s.sys
	handle := func(pattern string, fn func(r *http.Request, agent string) (any, error)) {
		mux.HandleFunc(pattern, s.agent(func(w http.ResponseWriter, r *http.Request, agent string) {
			out, err := fn(r, agent)
			if err != nil {
				writePartitionError(w, err)
				return
			}
			if out == nil {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			writeJSON(w, http.StatusOK, out)
		}))
	}

	handle("GET /api/agent/map", func(r *http.Request, agent string) (any, error) {
		return sys.OwnView(agent)
	})
	handle("PATCH /api/agent/map/partition", func(r *http.Request, agent string) (any, error) {
		var p featuremap.PartitionPatch
		if err := decode(r, &p); err != nil {
			return nil, err
		}
		return nil, sys.SetPartitionInfo(agent, p)
	})
	handle("POST /api/agent/wiki", func(r *http.Request, agent string) (any, error) {
		var req api.ChangeRequest
		if err := decode(r, &req); err != nil {
			return nil, err
		}
		return sys.ChangeWiki(agent, req.Public, req.Writes)
	})
	handle("GET /api/agent/wiki/docs/{id}", func(r *http.Request, agent string) (any, error) {
		return sys.GetDocument(agent, r.PathValue("id"))
	})
	handle("GET /api/agent/wiki/search", func(r *http.Request, agent string) (any, error) {
		return sys.SearchWiki(agent, r.URL.Query().Get("q"))
	})
	handle("POST /api/agent/map/tests", func(r *http.Request, agent string) (any, error) {
		var req api.RunRequest
		if err := decode(r, &req); err != nil {
			return nil, err
		}
		return sys.RecordRun(agent, req.Suite)
	})
	handle("POST /api/agent/map/current", func(r *http.Request, agent string) (any, error) {
		commits, err := sys.MarkCurrent(agent)
		return api.CommitResponse{Commits: commits}, err
	})
	handle("POST /api/agent/commit", func(r *http.Request, agent string) (any, error) {
		var req api.CommitRequest
		if err := decode(r, &req); err != nil {
			return nil, err
		}
		commits, err := sys.Commit(agent, req.Message)
		return api.CommitResponse{Commits: commits}, err
	})
	handle("POST /api/agent/releases/check", func(r *http.Request, agent string) (any, error) {
		var req api.CheckRequest
		if err := decode(r, &req); err != nil {
			return nil, err
		}
		return sys.CheckRelease(agent, req.Deliverable)
	})
	handle("POST /api/agent/releases", func(r *http.Request, agent string) (any, error) {
		var req api.PublishRequest
		if err := decode(r, &req); err != nil {
			return nil, err
		}
		rel, err := sys.PublishRelease(agent, req)
		if err != nil {
			return nil, err
		}
		body := fmt.Sprintf("Released %s %s %s: %s", agent, rel.Deliverable, rel.Version, rel.Notes)
		if _, err := s.Send(agent, api.SendRequest{Body: body}); err != nil {
			return nil, err
		}
		return rel, nil
	})
	handle("GET /api/agent/turn-context", func(r *http.Request, agent string) (any, error) {
		return sys.TurnContext(agent), nil
	})

	handle("GET /api/agent/partitions", func(r *http.Request, agent string) (any, error) {
		return sys.ListPartitions(agent)
	})
	handle("GET /api/agent/partitions/{name}", func(r *http.Request, agent string) (any, error) {
		return sys.GetPartition(agent, r.PathValue("name"))
	})

	handle("GET /api/agent/vocabulary", func(r *http.Request, agent string) (any, error) {
		return sys.Vocabulary(), nil
	})
	handle("POST /api/agent/vocabulary", func(r *http.Request, agent string) (any, error) {
		var ch api.VocabularyChange
		if err := decode(r, &ch); err != nil {
			return nil, err
		}
		switch {
		case ch.Op == "define_term" && ch.Term != nil:
			return nil, sys.DefineTerm(agent, *ch.Term)
		case ch.Op == "define_interface" && ch.Interface != nil:
			return nil, sys.DefineInterface(agent, *ch.Interface)
		case ch.Op == "remove":
			return nil, sys.RemoveEntry(agent, ch.Name)
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

func writePartitionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, partitions.ErrForbidden):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, partitions.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		writeError(w, http.StatusBadRequest, err.Error())
	}
}

func (s *Server) agent(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		agent, ok := s.agentForToken(token)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unknown agent token")
			return
		}
		next(w, r, agent)
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
