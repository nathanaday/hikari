package topology

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestFindingPaths(t *testing.T) {
	cases := map[string]struct{ hikari, services, path string }{
		"orchestrator": {"  orchestration: ghost", "  a: {x-hikari: {}}", "x-hikari.orchestration"},
		"name":         {"  orchestration: a", "  a: {x-hikari: {name: A}}", "services.a.x-hikari.name"},
		"unreachable":  {"  orchestration: a", "  a: {x-hikari: {}}\n  b: {x-hikari: {}}", "services.b"},
		"system repo":  {"  orchestration: a", "  a: {x-hikari: {}}\n  b: {x-hikari: {system: {repo: ghost}}}", "services.b.x-hikari.system.repo"},
		"service key":  {"  orchestration: a", "  a: {privileged: true, x-hikari: {}}", "services.a.privileged"},
		"repo url":     {"  repos: {r: {url: -x}}\n  orchestration: a", "  a: {x-hikari: {}}", "x-hikari.repos.r.url"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			src := compose(c.hikari, c.services)
			_, err := Parse([]byte(src), t.TempDir())
			fs := Findings(err, []byte(src))
			found := false
			for _, f := range fs {
				if f.Path == c.path {
					found = f.Line > 0 && f.Severity == SeverityError && f.Stage == StageTopology
				}
			}
			if !found {
				t.Errorf("TF2: no finding at %s with a line: %+v", c.path, fs)
			}
		})
	}
}

func TestLineOf(t *testing.T) {
	src := `name: t
x-agent: &agent
  image: hikari-agent
services:
  dev:
    <<: *agent
    x-hikari:
      description: Developer.
      system:
        read:
          - proto
          - docs
`
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatal(err)
	}
	root := doc.Content[0]
	for path, want := range map[string]int{
		"services.dev":                         5,
		"services.dev.x-hikari.description":    8,
		"services.dev.x-hikari.system.read":    10,
		"services.dev.x-hikari.system.read[1]": 12,
		"services.dev.image":                   5, // from the merge key: the deepest node the source writes
		"services.ghost":                       4,
		"":                                     1,
	} {
		if got := LineOf(root, path); got != want {
			t.Errorf("TF3: LineOf(%q) = %d, want %d", path, got, want)
		}
	}
	if LineOf(nil, "services") != 0 {
		t.Error("no root, no line")
	}
}

func TestSyntaxFinding(t *testing.T) {
	if f := SyntaxFinding([]byte("name: t\n")); f != nil {
		t.Errorf("valid YAML: %+v", f)
	}
	f := SyntaxFinding([]byte("name: t\nservices:\n  a: [b\n  c: d\n"))
	if f == nil || f.Line == 0 || f.Stage != StageYAML || f.Message == "" {
		t.Errorf("TF4: %+v", f)
	}
}
