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

// room returns a project with the orchestrator lead and the owners dev
// and qa, in a repo that it makes.
func room(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	for _, d := range []string{".git", "dev", "qa", "reviewer", "solo"} {
		os.MkdirAll(filepath.Join(repo, d), 0o755)
	}
	return `# The orchestrator lead, and the owners dev and qa.

name: room

x-hikari:
  repos:
    app: {path: ` + repo + `, branch: hikari}
  orchestration: lead

x-agent: &agent
  image: hikari-agent
  environment: [CLAUDE_CODE_OAUTH_TOKEN, ANTHROPIC_API_KEY]

services:
  lead:
    <<: *agent
    x-hikari:
      description: Leads the work. Turns requests into tasks.
  dev:
    <<: *agent
    x-hikari:
      description: Developer. Builds the app.
      instructions: Answer with a short plan in plain text.
      system: {repo: app, scope: dev}
  qa:
    <<: *agent
    x-hikari:
      description: QA engineer. Writes the tests.
      system: {repo: app, scope: qa}
`
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
	text := room(t)
	cases := []struct {
		name  string
		ops   []Op
		check func(t *testing.T, m *Model, res *Result)
		lines int // the most changed lines, of TE1
	}{
		{"name the project", []Op{{Op: "set_project", Name: ptr("Team Planning")}}, func(t *testing.T, m *Model, _ *Result) {
			if m.Project.Name != "Team Planning" || m.Project.ID != "room" || m.Project.Orchestration != "lead" {
				t.Errorf("%+v", m.Project)
			}
		}, 1},
		{"rename an agent", []Op{{Op: "rename", Kind: "agent", ID: "dev", Name: ptr("builder")}}, func(t *testing.T, m *Model, res *Result) {
			if a := agentOf(m, "dev"); a.Name != "builder" {
				t.Errorf("%+v", a)
			}
			if !slices.Equal(res.Renames, []Rename{{"agent", "dev", "builder"}}) || res.Summary[0] != "rename agent dev to builder" {
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
		{"set the orchestrator", []Op{
			{Op: "clear_system", ID: "qa"},
			{Op: "set_orchestration", Agent: "qa"},
			{Op: "remove_agent", ID: "lead"},
		}, func(t *testing.T, m *Model, res *Result) {
			if m.Project.Orchestration != "qa" || agentOf(m, "lead") != nil {
				t.Errorf("%+v", m.Project)
			}
			if got := strings.Join(res.Summary, "; "); got != "clear the system of qa; make qa the orchestrator; remove agent lead" {
				t.Errorf("%s", got)
			}
		}, 6},
		{"clear the orchestrator", []Op{{Op: "set_orchestration"}, {Op: "set_system", ID: "lead", Repo: ptr("app"), Scope: ptr("solo")}}, func(t *testing.T, m *Model, res *Result) {
			if m.Project.Orchestration != "" || strings.Contains(string(res.Text), "orchestration") {
				t.Errorf("%+v\n%s", m.Project, res.Text)
			}
		}, 2},
		{"remove the orchestrator", []Op{{Op: "remove_agent", ID: "lead"}}, func(t *testing.T, m *Model, res *Result) {
			if m.Project.Orchestration != "" || agentOf(m, "lead") != nil {
				t.Errorf("H18: removing the orchestrator clears the key: %+v\n%s", m.Project, res.Text)
			}
		}, 5},
		{"add an agent from a template", []Op{
			{Op: "add_agent", ID: "reviewer", Template: "dev", Description: ptr("Reviews.")},
			{Op: "set_system", ID: "reviewer", Repo: ptr("app"), Scope: ptr("reviewer")},
		}, func(t *testing.T, m *Model, res *Result) {
			a := agentOf(m, "reviewer")
			if a == nil || a.Template != "agent" || a.Description != "Reviews." || a.Instructions != "" {
				t.Errorf("TE5: %+v\n%s", a, res.Text)
			}
			if res.Summary[0] != "add agent reviewer" {
				t.Errorf("%v", res.Summary)
			}
		}, 6},
		{"add an agent without a template", []Op{
			{Op: "add_agent", ID: "solo"},
			{Op: "set_system", ID: "solo", Repo: ptr("app"), Scope: ptr("solo")},
		}, func(t *testing.T, m *Model, _ *Result) {
			if a := agentOf(m, "solo"); a == nil || a.Image != "hikari-agent" {
				t.Errorf("TE5: %+v", a)
			}
		}, 7},
		{"set an agent", []Op{{Op: "set_agent", ID: "dev", Model: ptr("haiku"), Instructions: ptr("Line one.\nLine two.\n"), Description: ptr("Costs $5.")}}, func(t *testing.T, m *Model, res *Result) {
			if a := agentOf(m, "dev"); a.Model != "haiku" || a.Instructions != "Line one.\nLine two.\n" || a.Description != "Costs $5." {
				t.Errorf("TE2: %+v", a)
			}
			if !strings.Contains(string(res.Text), "Costs $$5.") {
				t.Errorf("a $ is written as $$, so that compose keeps it:\n%s", res.Text)
			}
		}, 5},
		{"remove an agent", []Op{{Op: "remove_agent", ID: "qa"}}, func(t *testing.T, m *Model, _ *Result) {
			if agentOf(m, "qa") != nil || m.Project.Orchestration != "lead" {
				t.Errorf("TE6: %+v", m)
			}
		}, 5},
		{"change an id", []Op{{Op: "change_id", Kind: "agent", ID: "lead", NewID: "chief"}}, func(t *testing.T, m *Model, res *Result) {
			if agentOf(m, "chief") == nil || m.Project.Orchestration != "chief" {
				t.Errorf("TE4: %+v", m.Project)
			}
			if !slices.Equal(res.Renames, []Rename{{"agent", "lead", "chief"}}) {
				t.Errorf("an agent named by its id is renamed too: %+v", res.Renames)
			}
		}, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, m := apply(t, text, c.ops...)
			c.check(t, m, res)
			parses(t, res.Text)
			removed, added := changedLines(text, string(res.Text))
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

const solo = `name: solo

x-hikari:
  orchestration: lead

services:
  lead:
    image: hikari-agent
    x-hikari: {description: Leads.}
  dev:
    image: hikari-agent
    x-hikari: {description: Builds.}
`

func TestRepoAndProject(t *testing.T) {
	res, m := apply(t, solo,
		Op{Op: "enable_project"},
		Op{Op: "add_repo", ID: "app", URL: ptr("https://example.com/app.git")},
		Op{Op: "set_system", ID: "dev", Repo: ptr("app")},
	)
	if !m.Project.Repo || m.Project.Branch != "${HIKARI_BRANCH:-hikari}" || len(m.Repos) != 1 || m.Repos[0].URL != "https://example.com/app.git" {
		t.Errorf("%+v %+v\n%s", m.Project, m.Repos, res.Text)
	}
	if !regexp.MustCompile(`\n  repos: \{app: \{url: '?https://example.com/app.git'?\}\}\n  orchestration: lead\n`).Match(res.Text) {
		t.Errorf("a new repos key goes before the orchestrator:\n%s", res.Text)
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
	text := room(t)
	shop := readExample(t, "shop")
	cases := []struct {
		name string
		text string
		ops  []Op
		want string
	}{
		{"unknown op", text, []Op{{Op: "explode"}}, "unknown operation"},
		{"removed op", text, []Op{{Op: "add_member", Agent: "dev"}}, "unknown operation"},
		{"taken id", text, []Op{{Op: "add_agent", ID: "dev"}}, "taken"},
		{"taken by a sidecar", shop, []Op{{Op: "add_agent", ID: "orders-mock"}}, "taken"},
		{"bad id", text, []Op{{Op: "add_agent", ID: "Dev"}}, "not a valid id"},
		{"bad name", text, []Op{{Op: "rename", Kind: "agent", ID: "dev", Name: ptr("Builder")}}, "not a valid name"},
		{"name taken", text, []Op{{Op: "rename", Kind: "agent", ID: "dev", Name: ptr("qa")}}, "taken"},
		{"unknown kind", text, []Op{{Op: "rename", Kind: "team", ID: "dev", Name: ptr("x")}}, "kind must be agent or repo"},
		{"orchestrator unknown", text, []Op{{Op: "set_orchestration", Agent: "ghost"}}, "no agent"},
		{"orchestrator a sidecar", shop, []Op{{Op: "set_orchestration", Agent: "orders-mock"}}, "no agent"},
		{"repo in use", shop, []Op{{Op: "remove_repo", ID: "shop"}}, "is in the repo"},
		{"no such agent", text, []Op{{Op: "set_agent", ID: "ghost"}}, "no agent"},
		{"project off", text, []Op{{Op: "set_project", Branch: ptr("x")}}, "needs the project repository"},
		{"the second fails", text, []Op{{Op: "add_agent", ID: "solo"}, {Op: "add_agent", ID: "solo"}}, "operation 2"},
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
	os.MkdirAll(filepath.Join(dir, "store", "systems", "dev"), 0o755)
	text := room(t)
	_, err := Apply([]byte(text), []Op{{Op: "change_id", Kind: "agent", ID: "dev", NewID: "builder"}}, dir)
	if err == nil || !strings.Contains(err.Error(), "cannot change") {
		t.Errorf("I4: %v", err)
	}
	m := ReadModel([]byte(text), dir)
	if !agentOf(m, "dev").IDLocked || agentOf(m, "qa").IDLocked {
		t.Errorf("TM3: %+v", m.Agents)
	}
}

func TestRandomOperations(t *testing.T) {
	for name, start := range map[string]string{"room": room(t), "shop": readExample(t, "shop")} {
		t.Run(name, func(t *testing.T) {
			text := start
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
	var agents []string
	for _, a := range m.Agents {
		agents = append(agents, a.ID)
	}
	id := "n" + string(rune('a'+i%26)) + string(rune('a'+i/26))
	switch rng.Intn(8) {
	case 0:
		return Op{Op: "add_agent", ID: id, Template: pick(agents)}
	case 1:
		return Op{Op: "set_orchestration", Agent: pick(append(agents, ""))}
	case 2:
		return Op{Op: "set_system", ID: pick(agents), Repo: ptr("app"), Scope: ptr(id)}
	case 3:
		return Op{Op: "rename", Kind: "agent", ID: pick(agents), Name: ptr(id)}
	case 4:
		return Op{Op: "set_agent", ID: pick(agents), Description: ptr("Text with: a colon, $5, and \"quotes\".")}
	case 5:
		return Op{Op: "remove_agent", ID: pick(agents)}
	case 6:
		return Op{Op: "change_id", Kind: "agent", ID: pick(agents), NewID: id}
	}
	return Op{Op: "set_agent", ID: pick(agents), Instructions: ptr("First.\nSecond.\n")}
}
