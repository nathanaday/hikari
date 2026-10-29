package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/module"
	"github.com/nathanaday/hikari/internal/modules"
	"github.com/nathanaday/hikari/internal/topology"
)

// openModules opens the host half of each module that the topology turns
// on (rule MO4 of the task pipeline spec). A module that fails to open is
// logged and left out, so that it never stops a run.
func (s *Server) openModules() {
	for _, m := range modules.Of(s.topo) {
		inst, err := m.Open(&moduleHost{s: s, name: m.Name()})
		if err != nil {
			log.Printf("module %s: %v", m.Name(), err)
			continue
		}
		s.modules = append(s.modules, openModule{name: m.Name(), inst: inst})
	}
}

type openModule struct {
	name string
	inst module.Instance
}

// WatchModules follows each module's files until ctx ends.
func (s *Server) WatchModules(ctx context.Context) {
	for _, m := range s.modules {
		go m.inst.Watch(ctx)
	}
}

// moduleRoutes serves each module's agent routes and UI routes.
func (s *Server) moduleRoutes(mux *http.ServeMux, ui func(http.HandlerFunc) http.HandlerFunc) {
	for _, m := range s.modules {
		for pattern, h := range m.inst.AgentRoutes() {
			method, p, _ := strings.Cut(pattern, " ")
			mux.HandleFunc(method+" /api/agent/"+m.name+p, s.agent(func(w http.ResponseWriter, r *http.Request, agent string) {
				answer(w, r, agent, h)
			}))
		}
		for pattern, h := range m.inst.UIRoutes() {
			method, p, _ := strings.Cut(pattern, " ")
			mux.HandleFunc(method+" /api/ui/"+m.name+p, ui(func(w http.ResponseWriter, r *http.Request) {
				answer(w, r, topology.User, h)
			}))
		}
	}
}

func answer(w http.ResponseWriter, r *http.Request, caller string, h module.Handler) {
	out, err := h(r, caller)
	switch {
	case errors.Is(err, module.ErrForbidden):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, module.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case err != nil:
		writeError(w, http.StatusBadRequest, err.Error())
	case out == nil:
		w.WriteHeader(http.StatusNoContent)
	default:
		writeJSON(w, http.StatusOK, out)
	}
}

// moduleNotices are the modules' lines for agent's turn.
func (s *Server) moduleNotices(agent string) []string {
	var out []string
	for _, m := range s.modules {
		out = append(out, m.inst.Notices(agent)...)
	}
	return out
}

// moduleViews are the modules' parts of the snapshot, by name.
func (s *Server) moduleViews() map[string]any {
	out := map[string]any{}
	for _, m := range s.modules {
		out[m.name] = m.inst.View()
	}
	return out
}

// moduleHost is the Host of one module.
type moduleHost struct {
	s    *Server
	name string
}

func (h *moduleHost) Topology() *topology.Topology { return h.s.topo }
func (h *moduleHost) VaultDir() string             { return h.s.topo.Vault }

func (h *moduleHost) Health(partition, id string) (string, bool) {
	ref := api.OrchestrationWikiRef
	if partition != "" {
		ref = api.FeatureMapRef(partition)
	}
	w, ok := h.s.sys.WikiView(ref)
	if !ok {
		return "", false
	}
	for _, d := range w.Documents {
		if d.ID == id {
			if d.Health != "" {
				return d.Health, true
			}
			return d.Support, true
		}
	}
	return "", false
}

func (h *moduleHost) Commit(author, message string, paths ...string) {
	if author == topology.User {
		author = ""
	}
	h.s.sys.CommitVault(author, message, paths...)
}

// Notify sends a direct message from hikari, which wakes an agent (rule
// MO2, and decision 3 of the task pipeline spec).
func (h *moduleHost) Notify(to, body string) {
	if _, err := h.s.direct(topology.Hikari, to, body); err != nil {
		log.Printf("module %s: notify %s: %v", h.name, to, err)
	}
}

func (h *moduleHost) Feed(text, task string) {
	h.s.mu.Lock()
	defer h.s.mu.Unlock()
	h.s.feed(api.Entry{Kind: api.EntryModule, Who: topology.Hikari, Text: text, Task: task})
}

func (h *moduleHost) Changed() {
	view := h.s.moduleView(h.name)
	h.s.mu.Lock()
	defer h.s.mu.Unlock()
	h.s.publish(Event{Type: "module", Module: h.name, View: view})
}

func (s *Server) moduleView(name string) any {
	for _, m := range s.modules {
		if m.name == name {
			return m.inst.View()
		}
	}
	return nil
}
