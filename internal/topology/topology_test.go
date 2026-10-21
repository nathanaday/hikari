package topology

import (
	"slices"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	topo, err := Parse([]byte(`
nodes:
  alice: {description: lead}
  bob: {}
edges:
  - [user, alice]
  - [alice, bob]
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := topo.Agents(); !slices.Equal(got, []string{"alice", "bob"}) {
		t.Errorf("Agents() = %v", got)
	}
	if got := topo.Neighbors("alice"); !slices.Equal(got, []string{"bob", "user"}) {
		t.Errorf("Neighbors(alice) = %v", got)
	}
	if !topo.Connected("bob", "alice") {
		t.Error("edges must be undirected")
	}
	if topo.Connected("user", "bob") {
		t.Error("user and bob must not be connected")
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"no nodes":      `edges: []`,
		"reserved name": "nodes: {user: {}}",
		"bad name":      "nodes: {Alice: {}}",
		"unknown node":  "nodes: {alice: {}}\nedges: [[alice, carol]]",
		"self edge":     "nodes: {alice: {}}\nedges: [[alice, alice]]",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(src)); err == nil {
				t.Errorf("expected error for %q", strings.TrimSpace(src))
			}
		})
	}
}
