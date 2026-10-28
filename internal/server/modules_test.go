package server

import (
	"slices"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/topology"
)

var tasksTopology = strings.Replace(testTopology, "  orchestration: lead\n", "  orchestration: lead\n  modules: [tasks]\n", 1)

// TestModulesWired checks MO4 and MO5 of the task pipeline spec, and
// decision 3: the hikari sender.
func TestModulesWired(t *testing.T) {
	s, _ := newServerOf(t, tasksTopology)
	if got := s.Self("writer").Modules; !slices.Equal(got, []string{"tasks"}) {
		t.Errorf("MO4 Self names the modules: %v", got)
	}
	var names []string
	for _, p := range s.participants() {
		names = append(names, p.Name)
	}
	if !slices.Equal(names, []string{"lead", "critic", "writer", topology.Hikari, topology.User}) {
		t.Errorf("decision 3: hikari comes before the user: %v", names)
	}
	if _, ok := s.snapshot().Modules["tasks"]; !ok {
		t.Error("MO4 the snapshot holds the module's view")
	}
	h := &moduleHost{s: s, name: "tasks"}
	h.Notify("lead", "The queue api started [[x]].")
	msgs := s.Take(t.Context(), "lead", 0)
	if len(msgs) != 1 || msgs[0].From != topology.Hikari || msgs[0].To != "lead" {
		t.Errorf("decision 3: hikari's message wakes lead: %+v", msgs)
	}
	if _, err := s.Send("writer", apiSend(topology.Hikari, "hi")); err == nil {
		t.Error("decision 3: no agent sends to hikari")
	}

	plain, _ := newTestServer(t)
	if len(plain.Self("writer").Modules) != 0 || len(plain.snapshot().Modules) != 0 || slices.ContainsFunc(plain.participants(), func(p apiPeer) bool { return p.Name == topology.Hikari }) {
		t.Error("MO5 a project without modules has none of them")
	}
}

type apiPeer = api.Peer

func apiSend(to, body string) api.SendRequest { return api.SendRequest{To: to, Body: body} }
