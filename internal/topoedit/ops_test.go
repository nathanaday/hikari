package topoedit

import (
	"errors"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/topology"
)

func ptr[T any](v T) *T { return &v }

func readExample(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", name, "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func apply(t *testing.T, text string, ops ...Op) (*Result, *Model) {
	t.Helper()
	res, err := Apply([]byte(text), ops, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := ReadModel(res.Text, t.TempDir())
	if m == nil {
		t.Fatalf("the result is not YAML:\n%s", res.Text)
	}
	return res, m
}

func agentOf(m *Model, id string) *Agent {
	for i := range m.Agents {
		if m.Agents[i].ID == id {
			return &m.Agents[i]
		}
	}
	return nil
}

func hubOf(m *Model, id string) *Hub {
	for i := range m.Hubs {
		if m.Hubs[i].ID == id {
			return &m.Hubs[i]
		}
	}
	return nil
}

// parses checks that compose and the topology checks accept the text.
func parses(t *testing.T, text []byte) {
	t.Helper()
	if exec.Command("docker", "compose", "version").Run() != nil {
		return
	}
	file := filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(file, text, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := topology.Load(t.Context(), file); err != nil {
		t.Errorf("the result fails its checks: %v\n%s", err, text)
	}
}

func TestOperations(t *testing.T) {
	team := readExample(t, "team")
	cases := []struct {
		name  string
		ops   []Op
		check func(t *testing.T, m *Model, res *Result)
		lines int // the most changed lines, of TE1
	}{
		{"name the project", []Op{{Op: "set_project", Name: ptr("Team Planning")}}, func(t *testing.T, m *Model, _ *Result) {
			if m.Project.Name != "Team Planning" || m.Project.ID != "team" {
				t.Errorf("%+v", m.Project)
			}
		}, 1},
		{"rename a hub", []Op{{Op: "rename", Kind: "hub", ID: "eng", Name: ptr("platform")}}, func(t *testing.T, m *Model, res *Result) {
			if h := hubOf(m, "eng"); h.Name != "platform" {
				t.Errorf("%+v", h)
			}
			if !slices.Equal(res.Renames, []Rename{{"hub", "eng", "platform"}}) || res.Summary[0] != "rename hub #eng to #platform" {
				t.Errorf("%+v %v", res.Renames, res.Summary)
			}
		}, 1},
		{"rename back to the id removes the field", []Op{
			{Op: "rename", Kind: "agent", ID: "dev", Name: ptr("builder")},
			{Op: "rename", Kind: "agent", ID: "dev", Name: ptr("dev")},
		}, func(t *testing.T, m *Model, res *Result) {
			if a := agentOf(m, "dev"); a.Name != "dev" || strings.Contains(string(res.Text), "builder") {
				t.Errorf("%+v", a)
			}
		}, 0},
		{"add a member", []Op{{Op: "add_member", Hub: "product", Agent: "dev"}}, func(t *testing.T, m *Model, _ *Result) {
			if h := hubOf(m, "product"); !slices.Equal(h.Members, []string{"writer", "eng-orchestrator", "dev"}) {
				t.Errorf("%+v", h)
			}
		}, 1},
		{"remove a member", []Op{{Op: "remove_member", Hub: "eng", Agent: "tester"}, {Op: "add_member", Hub: "product", Agent: "tester"}}, func(t *testing.T, m *Model, _ *Result) {
			if h := hubOf(m, "eng"); !slices.Equal(h.Members, []string{"dev"}) {
				t.Errorf("%+v", h)
			}
		}, 2},
		{"add a hub", []Op{{Op: "add_hub", ID: "ops", Orchestrator: ptr("orchestrator"), Description: ptr("Runs it."), Members: &[]string{"dev"}}}, func(t *testing.T, m *Model, _ *Result) {
			if h := hubOf(m, "ops"); h == nil || h.Orchestrator != "orchestrator" || h.Description != "Runs it." || !slices.Equal(h.Members, []string{"dev"}) {
				t.Errorf("%+v", h)
			}
		}, 3},
		{"set a hub", []Op{{Op: "set_hub", ID: "eng", Description: ptr("Costs $5."), Orchestrator: ptr("orchestrator")}}, func(t *testing.T, m *Model, res *Result) {
			if h := hubOf(m, "eng"); h.Description != "Costs $5." || h.Orchestrator != "orchestrator" {
				t.Errorf("%+v", h)
			}
			if !strings.Contains(string(res.Text), "Costs $$5.") {
				t.Errorf("a $ is written as $$, so that compose keeps it:\n%s", res.Text)
			}
		}, 2},
		{"add an agent from a template", []Op{{Op: "add_agent", ID: "reviewer", Template: "dev", Description: ptr("Reviews."), Hubs: &[]string{"eng"}}}, func(t *testing.T, m *Model, res *Result) {
			a := agentOf(m, "reviewer")
			if a == nil || a.Template != "agent" || a.Description != "Reviews." || a.Instructions != "" {
				t.Errorf("TE5: %+v\n%s", a, res.Text)
			}
			if h := hubOf(m, "eng"); !slices.Contains(h.Members, "reviewer") {
				t.Errorf("%+v", h)
			}
		}, 6},
		{"add an agent without a template", []Op{{Op: "add_agent", ID: "solo", Hubs: &[]string{"eng"}}}, func(t *testing.T, m *Model, _ *Result) {
			if a := agentOf(m, "solo"); a == nil || a.Image != "hikari-agent" {
				t.Errorf("TE5: %+v", a)
			}
		}, 7},
		{"set an agent", []Op{{Op: "set_agent", ID: "dev", Model: ptr("haiku"), Instructions: ptr("Line one.\nLine two.\n"), Description: ptr("")}}, func(t *testing.T, m *Model, _ *Result) {
			if a := agentOf(m, "dev"); a.Model != "haiku" || a.Instructions != "Line one.\nLine two.\n" || a.Description != "" {
				t.Errorf("TE2: %+v", a)
			}
		}, 5},
		{"remove an agent", []Op{{Op: "remove_agent", ID: "tester"}}, func(t *testing.T, m *Model, _ *Result) {
			if agentOf(m, "tester") != nil || slices.Contains(hubOf(m, "eng").Members, "tester") {
				t.Errorf("TE6: %+v", m)
			}
		}, 8},
		{"change an id", []Op{{Op: "change_id", Kind: "agent", ID: "eng-orchestrator", NewID: "eng-lead"}}, func(t *testing.T, m *Model, res *Result) {
			if agentOf(m, "eng-lead") == nil || hubOf(m, "eng").Orchestrator != "eng-lead" || !slices.Contains(hubOf(m, "product").Members, "eng-lead") {
				t.Errorf("TE4: %+v", m.Hubs)
			}
			if !slices.Equal(res.Renames, []Rename{{"agent", "eng-orchestrator", "eng-lead"}}) {
				t.Errorf("an agent named by its id is renamed too: %+v", res.Renames)
			}
		}, 3},
		{"change a hub id", []Op{{Op: "change_id", Kind: "hub", ID: "eng", NewID: "build"}}, func(t *testing.T, m *Model, _ *Result) {
			if hubOf(m, "build") == nil || hubOf(m, "eng") != nil {
				t.Errorf("%+v", m.Hubs)
			}
		}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, m := apply(t, team, c.ops...)
			c.check(t, m, res)
			parses(t, res.Text)
			removed, added := changedLines(team, string(res.Text))
			if len(removed) > c.lines || len(added) > c.lines+1 {
				t.Errorf("TE1: %d lines removed, %d added, want at most %d:\n-%q\n+%q", len(removed), len(added), c.lines, removed, added)
			}
		})
	}
}

func TestSystemsAndGrants(t *testing.T) {
	shop := readExample(t, "shop")
	res, m := apply(t, shop,
		Op{Op: "set_system", ID: "orders", Scope: ptr("services/intake"), Read: &[]string{"proto", "docs"}},
		Op{Op: "set_grants", ID: "orders", Releases: &[]string{"inventory"}},
		Op{Op: "rename", Kind: "repo", ID: "shop", Name: ptr("storefront")},
	)
	a := agentOf(m, "orders")
	if a.System.Scope != "services/intake" || !slices.Equal(a.System.Read, []string{"proto", "docs"}) || !slices.Equal(a.Releases, []string{"inventory"}) {
		t.Errorf("%+v %+v\n%s", a, a.System, res.Text)
	}
	if m.Repos[0].Name != "storefront" {
		t.Errorf("%+v", m.Repos)
	}
	res, m = apply(t, string(res.Text), Op{Op: "set_grants", ID: "orders", Releases: &[]string{}}, Op{Op: "clear_system", ID: "orders"})
	if a := agentOf(m, "orders"); a.System != nil || len(a.Releases) != 0 {
		t.Errorf("%+v\n%s", a, res.Text)
	}
}

func TestRepoAndProject(t *testing.T) {
	team := readExample(t, "team")
	res, m := apply(t, team,
		Op{Op: "enable_project"},
		Op{Op: "add_repo", ID: "app", URL: ptr("https://example.com/app.git")},
		Op{Op: "set_system", ID: "dev", Repo: ptr("app")},
	)
	if !m.Project.Repo || m.Project.Branch != "${HIKARI_BRANCH:-hikari}" || len(m.Repos) != 1 || m.Repos[0].URL != "https://example.com/app.git" {
		t.Errorf("%+v %+v\n%s", m.Project, m.Repos, res.Text)
	}
	if !regexp.MustCompile(`\n  repos: \{app: \{url: '?https://example.com/app.git'?\}\}\n  hubs:\n`).Match(res.Text) {
		t.Errorf("a new repos key goes before the hubs:\n%s", res.Text)
	}
	if len(res.Effects) != 1 || !strings.Contains(res.Effects[0], "hikari sync") {
		t.Errorf("TA4: %v", res.Effects)
	}
	if !IsExpression(m.Project.Branch) {
		t.Error("TM2: an interpolated value is an expression")
	}
	res, m = apply(t, string(res.Text), Op{Op: "change_id", Kind: "repo", ID: "app", NewID: "web"})
	if agentOf(m, "dev").System.Repo != "web" {
		t.Errorf("TE4: system.repo follows: %s", res.Text)
	}
}

func TestRefusals(t *testing.T) {
	team := readExample(t, "team")
	shop := readExample(t, "shop")
	cases := []struct {
		name string
		text string
		ops  []Op
		want string
	}{
		{"unknown op", team, []Op{{Op: "explode"}}, "unknown operation"},
		{"taken id", team, []Op{{Op: "add_agent", ID: "dev"}}, "taken"},
		{"taken by a hub", team, []Op{{Op: "add_agent", ID: "eng"}}, "taken"},
		{"bad id", team, []Op{{Op: "add_hub", ID: "Eng", Orchestrator: ptr("dev")}}, "not a valid id"},
		{"bad name", team, []Op{{Op: "rename", Kind: "hub", ID: "eng", Name: ptr("Platform")}}, "not a valid name"},
		{"name taken", team, []Op{{Op: "rename", Kind: "hub", ID: "eng", Name: ptr("dev")}}, "taken"},
		{"already a member", team, []Op{{Op: "add_member", Hub: "eng", Agent: "dev"}}, "already in #eng"},
		{"not a member", team, []Op{{Op: "remove_member", Hub: "eng", Agent: "writer"}}, "not a member"},
		{"orchestrates", team, []Op{{Op: "remove_agent", ID: "eng-orchestrator"}}, "orchestrates #eng"},
		{"left in no hub", team, []Op{{Op: "remove_hub", ID: "eng"}}, "would be in no hub"},
		{"repo in use", shop, []Op{{Op: "remove_repo", ID: "shop"}}, "is in the repo"},
		{"no such agent", team, []Op{{Op: "set_agent", ID: "ghost"}}, "no agent"},
		{"project off", team, []Op{{Op: "set_project", Branch: ptr("x")}}, "needs the project repository"},
		{"the second fails", team, []Op{{Op: "add_member", Hub: "product", Agent: "dev"}, {Op: "add_member", Hub: "product", Agent: "dev"}}, "operation 2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Apply([]byte(c.text), c.ops, t.TempDir())
			var oe *OpError
			if err == nil || !errors.As(err, &oe) || !strings.Contains(err.Error(), c.want) {
				t.Errorf("TE7: %v, want %q", err, c.want)
			}
		})
	}
}

func TestIDLocked(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "store", "hubs", "eng"), 0o755)
	_, err := Apply([]byte(readExample(t, "team")), []Op{{Op: "change_id", Kind: "hub", ID: "eng", NewID: "build"}}, dir)
	if err == nil || !strings.Contains(err.Error(), "cannot change") {
		t.Errorf("I4: %v", err)
	}
	m := ReadModel([]byte(readExample(t, "team")), dir)
	if !hubOf(m, "eng").IDLocked || hubOf(m, "product").IDLocked {
		t.Errorf("TM3: %+v", m.Hubs)
	}
	res, err := Apply([]byte(readExample(t, "team")), []Op{{Op: "remove_member", Hub: "eng", Agent: "tester"}, {Op: "add_member", Hub: "product", Agent: "tester"},
		{Op: "remove_member", Hub: "eng", Agent: "dev"}, {Op: "add_member", Hub: "product", Agent: "dev"},
		{Op: "add_member", Hub: "product", Agent: "eng-orchestrator"}}, dir)
	if err == nil {
		t.Fatalf("eng-orchestrator is a member of product already: %s", res.Text)
	}
}

func TestRandomOperations(t *testing.T) {
	for _, name := range []string{"team", "shop", "studio"} {
		t.Run(name, func(t *testing.T) {
			text := readExample(t, name)
			rng := rand.New(rand.NewSource(1))
			for i := range 60 {
				m := ReadModel([]byte(text), t.TempDir())
				op := randomOp(rng, m, i)
				res, err := Apply([]byte(text), []Op{op}, t.TempDir())
				if err != nil {
					var oe *OpError
					if !errors.As(err, &oe) {
						t.Fatalf("%+v: %v", op, err)
					}
					continue
				}
				if ReadModel(res.Text, t.TempDir()) == nil {
					t.Fatalf("after %+v the file is not YAML:\n%s", op, res.Text)
				}
				text = string(res.Text)
			}
		})
	}
}

func randomOp(rng *rand.Rand, m *Model, i int) Op {
	pick := func(ids []string) string {
		if len(ids) == 0 {
			return "none"
		}
		return ids[rng.Intn(len(ids))]
	}
	var agents, hubs []string
	for _, a := range m.Agents {
		agents = append(agents, a.ID)
	}
	for _, h := range m.Hubs {
		hubs = append(hubs, h.ID)
	}
	id := "n" + string(rune('a'+i%26)) + string(rune('a'+i/26))
	switch rng.Intn(9) {
	case 0:
		return Op{Op: "add_agent", ID: id, Template: pick(agents), Hubs: &[]string{pick(hubs)}}
	case 1:
		return Op{Op: "add_member", Hub: pick(hubs), Agent: pick(agents)}
	case 2:
		h := hubOf(m, pick(hubs))
		if h == nil {
			return Op{Op: "noop"}
		}
		return Op{Op: "remove_member", Hub: h.ID, Agent: pick(h.Members)}
	case 3:
		return Op{Op: "rename", Kind: "hub", ID: pick(hubs), Name: ptr(id)}
	case 4:
		return Op{Op: "set_agent", ID: pick(agents), Description: ptr("Text with: a colon, $5, and \"quotes\".")}
	case 5:
		return Op{Op: "add_hub", ID: id, Orchestrator: ptr(pick(agents))}
	case 6:
		return Op{Op: "remove_agent", ID: pick(agents)}
	case 7:
		return Op{Op: "change_id", Kind: "agent", ID: pick(agents), NewID: id}
	}
	return Op{Op: "set_agent", ID: pick(agents), Instructions: ptr("First.\nSecond.\n")}
}
