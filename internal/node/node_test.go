package node

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/api"
)

func TestSystemPromptStatesRolePerHub(t *testing.T) {
	self := api.Self{
		Name:        "lead",
		Description: "leads engineering",
		Hubs: []api.HubInfo{
			{Name: "company", Participants: []api.Peer{
				{Name: "ceo", Role: api.RoleOrchestrator},
				{Name: "lead", Role: api.RoleMember},
				{Name: "user", Role: api.RoleOperator, Description: "The human operator."},
			}},
			{Name: "eng", Description: "builds things", Participants: []api.Peer{
				{Name: "lead", Role: api.RoleOrchestrator},
				{Name: "dev", Role: api.RoleMember, Description: "writes code"},
				{Name: "user", Role: api.RoleOperator, Description: "The human operator."},
			}},
		},
	}
	p := systemPrompt(self)
	t.Log(p)
	for _, want := range []string{
		"In #company you are a member. ceo coordinates the hub.",
		"In #eng you are the orchestrator.",
		"- lead (you), orchestrator",
		"- dev, member: writes code",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
}

var owner = api.Self{
	Name: "orders",
	Hubs: []api.HubInfo{{Name: "eng", Participants: []api.Peer{
		{Name: "eng-lead", Role: api.RoleOrchestrator},
		{Name: "orders", Role: api.RoleMember},
		{Name: "user", Role: api.RoleOperator},
	}}},
	System: &api.SelfSystem{Repo: "shop", Branch: "hikari/work", Scope: "services/orders", Read: []string{"proto"}, WorkDir: "/workspace/services/orders"},
}

func TestOwnerSystemPrompt(t *testing.T) {
	p := systemPrompt(owner)
	for _, want := range []string{
		`You own the system "orders": the directory services/orders of the repo "shop".`,
		"Your working directory, /workspace/services/orders, is your scope.",
		"You can also read, but not change, /workspace/proto.",
		"/hikari/public/export/map.yaml, which you change only through the map tools",
		"/hikari/private/layout.yaml and /hikari/private/test.yaml, which you edit directly",
		"run the release-docs skill",
		"mcp__hikari__publish_release",
		"/hikari/public/readonly/shared holds what every agent",
		"load the hikari-system skill",
		"mcp__hikari__get_vocabulary",
		`Commits go to the branch "hikari/work".`,
		"no trailers such as Co-Authored-By",
		"as your test plan says",
		"tell your orchestrator what you need",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("D4: owner prompt lacks %q", want)
		}
	}
	if strings.Contains(p, "Your vocabularies") {
		t.Error("an owner must not get the orchestrator section")
	}
}

func TestOrchestratorSystemPrompt(t *testing.T) {
	lead := api.Self{Name: "eng-lead", Orchestrates: []string{"eng"}, VisibleSystems: []string{"inventory", "orders"}}
	p := systemPrompt(lead)
	for _, want := range []string{
		"The members of #eng own these systems: inventory, orders. You have no code and cannot read any.",
		"mcp__hikari__list_systems",
		"When a map does not answer a question, ask the owner.",
		"You keep the vocabulary of #eng",
		"get_system flags interface names",
		"/hikari/public/readonly/systems/<system>/",
		"When a system announces a release, check it",
		"only through that system's published releases",
		"No member of your hubs is granted another system's releases",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("D5: orchestrator prompt lacks %q", want)
		}
	}
	if strings.Contains(p, "Your system\n") {
		t.Error("an orchestrator must not get the owner section")
	}
	plain := systemPrompt(api.Self{Name: "lead", Orchestrates: []string{"product"}})
	if strings.Contains(plain, "Your systems") || !strings.Contains(plain, "Your vocabularies") {
		t.Errorf("an orchestrator without systems gets only the vocabulary section:\n%s", plain)
	}
}

func TestIndexingPrompt(t *testing.T) {
	build := indexingPrompt(owner, api.TurnContext{MapState: "unbuilt"})
	for _, want := range []string{"not built yet", "Load the hikari-system skill", "set_system_info", "layout.yaml", "test.yaml", "record_test_results", "mark_map_current", "Send no messages"} {
		if !strings.Contains(build, want) {
			t.Errorf("D2: build prompt lacks %q", want)
		}
	}
	update := indexingPrompt(owner, api.TurnContext{MapState: "stale", Changed: []string{"create.py", "cancel.py"}})
	if !strings.Contains(update, "- create.py\n- cancel.py\n") || !strings.Contains(update, "affect") {
		t.Errorf("D2: update prompt:\n%s", update)
	}
}

func TestTurnNotices(t *testing.T) {
	if got := turnNotices(owner, api.TurnContext{MapState: "current"}); got != "" {
		t.Errorf("no notices expected, got %q", got)
	}
	got := turnNotices(owner, api.TurnContext{MapState: "stale", Changed: []string{"create.py"}, VocabularyChanged: []string{"eng"}})
	for _, want := range []string{"These paths changed since it was last marked current: create.py.", "The vocabulary of #eng changed.", "Rename entries in your map"} {
		if !strings.Contains(got, want) {
			t.Errorf("D3: notices lack %q:\n%s", want, got)
		}
	}
	member := turnNotices(api.Self{Name: "writer"}, api.TurnContext{MapState: "stale", VocabularyChanged: []string{"product"}})
	if strings.Contains(member, "system map") || strings.Contains(member, "Rename") || !strings.Contains(member, "#product") {
		t.Errorf("D3: a member without a system got %q", member)
	}
}

func TestBuiltinSkills(t *testing.T) {
	for name, wants := range map[string][]string{
		"hikari-system": {"## layout.yaml", "## test.yaml", "P1:", "P4:", "## Publish a release", "/hikari/private/layout.yaml", "publish_release"},
		"release-docs":  {"context: fork\nbackground: false\n", "/hikari/public/export/staging/docs/", "Change no file outside that directory"},
	} {
		data, err := builtinSkills.ReadFile("skills/" + name + ".md")
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if !strings.HasPrefix(text, "---\nname: "+name+"\ndescription: ") {
			t.Errorf("%s must start with its front matter:\n%.120s", name, text)
		}
		for _, want := range wants {
			if !strings.Contains(text, want) {
				t.Errorf("%s lacks %q", name, want)
			}
		}
	}
}

func newTestShared(t *testing.T, owner bool) (*sharedArea, string) {
	t.Helper()
	root := t.TempDir()
	shared := filepath.Join(root, "shared")
	for _, d := range []string{"skills", "agents", "policies", "vocabulary"} {
		os.MkdirAll(filepath.Join(shared, d), 0o755)
	}
	return &sharedArea{home: filepath.Join(root, "home"), dir: shared, owner: owner, warned: map[string]bool{}}, shared
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSharedAreaSync(t *testing.T) {
	s, shared := newTestShared(t, true)
	writeFile(t, filepath.Join(shared, "skills", "style", "SKILL.md"), "---\nname: style\n---\n")
	writeFile(t, filepath.Join(shared, "skills", "hikari-system", "SKILL.md"), "fake")
	writeFile(t, filepath.Join(shared, "skills", "empty-dir", "notes.txt"), "no skill here")
	writeFile(t, filepath.Join(shared, "agents", "reviewer.md"), "---\nname: reviewer\n---\n")
	writeFile(t, filepath.Join(shared, "policies", "b-security.md"), "Never log secrets.")
	writeFile(t, filepath.Join(shared, "policies", "a-style.md"), "Write short sentences.")

	policies := s.sync()
	if target, _ := os.Readlink(filepath.Join(s.skillsDir(), "style")); target != filepath.Join(shared, "skills", "style") {
		t.Errorf("H1: style skill link = %q", target)
	}
	if _, err := os.Lstat(filepath.Join(s.skillsDir(), "empty-dir")); err == nil {
		t.Error("H1: a directory without SKILL.md must not be linked")
	}
	if target, _ := os.Readlink(filepath.Join(s.agentsDir(), "reviewer.md")); target != filepath.Join(shared, "agents", "reviewer.md") {
		t.Errorf("H2: agent link = %q", target)
	}
	if !strings.Contains(policies, "# Policies") || strings.Index(policies, "## a-style") > strings.Index(policies, "## b-security") || !strings.Contains(policies, "Never log secrets.") {
		t.Errorf("H3: policies:\n%s", policies)
	}
	data, _ := os.ReadFile(filepath.Join(s.skillsDir(), "hikari-system", "SKILL.md"))
	if !strings.HasPrefix(string(data), "---\nname: hikari-system") {
		t.Errorf("H4: the built-in hikari-system skill was replaced: %.40q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(s.skillsDir(), "release-docs", "SKILL.md")); !strings.Contains(string(data), "context: fork") {
		t.Error("H5: owners get the built-in release-docs skill")
	}

	// A shared release-docs replaces the built-in one, and the built-in one
	// returns when the shared one is removed.
	writeFile(t, filepath.Join(shared, "skills", "release-docs", "SKILL.md"), "---\nname: release-docs\n---\nmine")
	s.sync()
	if target, _ := os.Readlink(filepath.Join(s.skillsDir(), "release-docs")); target != filepath.Join(shared, "skills", "release-docs") {
		t.Errorf("H5: shared release-docs link = %q", target)
	}
	os.RemoveAll(filepath.Join(shared, "skills", "release-docs"))
	os.RemoveAll(filepath.Join(shared, "skills", "style"))
	os.Remove(filepath.Join(shared, "agents", "reviewer.md"))
	s.sync()
	if _, err := os.Lstat(filepath.Join(s.skillsDir(), "style")); err == nil {
		t.Error("H6: the link of a removed skill must go")
	}
	if _, err := os.Lstat(filepath.Join(s.agentsDir(), "reviewer.md")); err == nil {
		t.Error("H6: the link of a removed agent must go")
	}
	if data, _ := os.ReadFile(filepath.Join(s.skillsDir(), "release-docs", "SKILL.md")); !strings.Contains(string(data), "context: fork") {
		t.Error("H5: the built-in release-docs must return")
	}
}

func TestSharedAreaForNonOwners(t *testing.T) {
	s, _ := newTestShared(t, false)
	if got := s.sync(); got != "" {
		t.Errorf("no policies, no section: %q", got)
	}
	if _, err := os.Stat(filepath.Join(s.skillsDir(), "hikari-system")); err == nil {
		t.Error("only owners get the built-in skills")
	}
}

func TestTurnNoticesReportPrivateErrors(t *testing.T) {
	got := turnNotices(owner, api.TurnContext{MapState: "current", PrivateErrors: []string{`P3: layout.yaml: path "x.py" does not exist in the scope`}})
	if !strings.Contains(got, "Your private documents have problems: P3:") {
		t.Errorf("notices: %q", got)
	}
}

func TestGrantsPrompt(t *testing.T) {
	consumer := owner
	consumer.Releases = []string{"inventory"}
	p := systemPrompt(consumer)
	for _, want := range []string{"## Releases you can use", "- inventory: /hikari/public/readonly/systems/inventory/releases", "name its system and version in your test plan"} {
		if !strings.Contains(p, want) {
			t.Errorf("P10: prompt lacks %q", want)
		}
	}
	lead := api.Self{Name: "lead", Orchestrates: []string{"eng"}, VisibleSystems: []string{"orders"}, Releases: []string{"orders"}}
	if strings.Contains(systemPrompt(lead), "## Releases you can use") {
		t.Error("an orchestrator's own systems are not grants")
	}
}

func TestMemberGrantsPrompt(t *testing.T) {
	lead := api.Self{Name: "lead", Orchestrates: []string{"shop"}, VisibleSystems: []string{"inventory", "orders"}, MemberGrants: map[string][]string{"orders": {"inventory"}}}
	if p := systemPrompt(lead); !strings.Contains(p, "orders reads the releases of inventory") {
		t.Errorf("the orchestrator must know its members' grants:\n%s", p)
	}
}
