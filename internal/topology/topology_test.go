package topology

import (
	"slices"
	"testing"
)

// company: ceo orchestrates eng-lead. eng: eng-lead orchestrates dev.
const tree = `
agents:
  ceo: {description: runs the company}
  eng-lead: {}
  dev: {}
hubs:
  company: {orchestrator: ceo, members: [eng-lead]}
  eng: {orchestrator: eng-lead, members: [dev]}
`

func TestParse(t *testing.T) {
	topo, err := Parse([]byte(tree))
	if err != nil {
		t.Fatal(err)
	}
	if got := topo.Participants("eng"); !slices.Equal(got, []string{"eng-lead", "dev", "user"}) {
		t.Errorf("Participants(eng) = %v", got)
	}
	if got := topo.HubsOf("eng-lead"); !slices.Equal(got, []string{"company", "eng"}) {
		t.Errorf("HubsOf(eng-lead) = %v", got)
	}
	if got := topo.HubsOf(User); !slices.Equal(got, []string{"company", "eng"}) {
		t.Errorf("the user must be in every hub, got %v", got)
	}
	if got := topo.Contacts("eng-lead"); !slices.Equal(got, []string{"ceo", "dev", "user"}) {
		t.Errorf("Contacts(eng-lead) = %v", got)
	}
	if topo.CanDirect("dev", "ceo") {
		t.Error("dev and ceo share no hub")
	}
	if !topo.CanDirect(User, "dev") {
		t.Error("the user can reach every agent")
	}
	if topo.InHub("dev", "company") {
		t.Error("dev is not in company")
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"empty":            ``,
		"old format":       "nodes: {a: {}}\nedges: []",
		"no hubs":          "agents: {a: {}}",
		"reserved name":    "agents: {user: {}}\nhubs: {h: {orchestrator: user}}",
		"bad name":         "agents: {Alice: {}}\nhubs: {h: {orchestrator: Alice}}",
		"no orchestrator":  "agents: {a: {}}\nhubs: {h: {members: [a]}}",
		"unknown member":   "agents: {a: {}}\nhubs: {h: {orchestrator: a, members: [b]}}",
		"duplicate member": "agents: {a: {}}\nhubs: {h: {orchestrator: a, members: [a]}}",
		"name clash":       "agents: {a: {}}\nhubs: {a: {orchestrator: a}}",
		"unreachable":      "agents: {a: {}, b: {}}\nhubs: {h: {orchestrator: a}}",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(src)); err == nil {
				t.Errorf("expected an error for %q", src)
			}
		})
	}
}
