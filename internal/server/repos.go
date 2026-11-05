package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/repos"
	"github.com/nathanaday/hikari/internal/topology"
)

// repoRoutes serve the repository views and their actions (rules RP6 and
// RP7 of the repositories, forges, and issues spec). The actions are
// yours: an agent reads the views and changes nothing.
func (s *Server) repoRoutes(mux *http.ServeMux, ui func(http.HandlerFunc) http.HandlerFunc) {
	answer := func(w http.ResponseWriter, out any, err error) {
		switch {
		case errors.Is(err, repos.ErrNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		case err != nil:
			writeError(w, http.StatusBadRequest, err.Error())
		case out == nil:
			w.WriteHeader(http.StatusNoContent)
		default:
			writeJSON(w, http.StatusOK, out)
		}
	}
	mux.HandleFunc("GET /api/ui/repos", ui(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.repos.Views())
	}))
	mux.HandleFunc("GET /api/ui/repos/{name}", ui(func(w http.ResponseWriter, r *http.Request) {
		v, ok := s.repos.View(r.PathValue("name"))
		if !ok {
			writeError(w, http.StatusNotFound, "no repository "+r.PathValue("name"))
			return
		}
		writeJSON(w, http.StatusOK, v)
	}))
	mux.HandleFunc("POST /api/ui/repos/{name}/refresh", ui(func(w http.ResponseWriter, r *http.Request) {
		err := s.repos.Refresh(r.Context(), r.PathValue("name"))
		v, _ := s.repos.View(r.PathValue("name"))
		if errors.Is(err, repos.ErrNotFound) {
			answer(w, nil, err)
			return
		}
		// A forge error is in the view; the refresh itself went through.
		answer(w, v, nil)
	}))
	mux.HandleFunc("POST /api/ui/repos/{name}/push", ui(func(w http.ResponseWriter, r *http.Request) {
		err := s.repos.Push(r.Context(), r.PathValue("name"))
		if err == nil {
			s.userDid("pushed " + r.PathValue("name"))
		}
		answer(w, nil, err)
	}))
	mux.HandleFunc("POST /api/ui/repos/{name}/pulls", ui(func(w http.ResponseWriter, r *http.Request) {
		var req api.NewPullRequestRequest
		if err := decode(r, &req); err != nil {
			answer(w, nil, err)
			return
		}
		pr, err := s.repos.OpenPullRequest(r.Context(), r.PathValue("name"), req)
		if err == nil {
			s.userDid(fmt.Sprintf("opened pull request #%d of %s: %s", pr.Number, r.PathValue("name"), pr.Title))
		}
		answer(w, pr, err)
	}))
	mux.HandleFunc("POST /api/ui/repos/{name}/pulls/{number}/merge", ui(func(w http.ResponseWriter, r *http.Request) {
		n, err := strconv.Atoi(r.PathValue("number"))
		if err != nil || n <= 0 {
			answer(w, nil, errors.New("the pull request number is not a number"))
			return
		}
		var req api.MergeRequest
		if err := decode(r, &req); err != nil && r.ContentLength > 0 {
			answer(w, nil, err)
			return
		}
		err = s.repos.Merge(r.Context(), r.PathValue("name"), n, req.Method)
		if err == nil {
			s.userDid(fmt.Sprintf("merged pull request #%d of %s", n, r.PathValue("name")))
		}
		answer(w, nil, err)
	}))

	// The orchestration agent reads the views (rule RP7).
	mux.HandleFunc("GET /api/agent/repos", s.agent(func(w http.ResponseWriter, r *http.Request, agent string) {
		if agent != s.topo.Orchestration {
			writeError(w, http.StatusForbidden, "only the orchestration agent reads the repositories")
			return
		}
		writeJSON(w, http.StatusOK, s.repos.Views())
	}))
}

// userDid adds a line of the feed for an action of yours on a repository.
func (s *Server) userDid(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.feed(api.Entry{Kind: api.EntryModule, Who: topology.User, Text: text})
}
