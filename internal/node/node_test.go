package node

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/featurewiki"
	"github.com/nathanaday/hikari/internal/sysmap"
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
		"/hikari/public/export/map.yaml, from them",
		"/hikari/public/export/features/",
		"mcp__hikari__change_features",
		"/hikari/private/test.yaml, which you edit directly",
		"Releases follow your deliverables",
		"You hold no deliverable, so you cannot publish a release",
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
		"/hikari/public/readonly/systems/<system>/features/",
		"get_feature returns the document",
		"When a system announces a release, check it",
		"the deliverable's definition",
		"only through that system's published releases",
		"No member of your hubs is granted another system's releases",
		"No member of your hubs holds a deliverable",
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
	for _, want := range []string{"not built yet", "Load the hikari-system skill", "Build procedure", "set_system_info", "change_features", "hikari-feature-survey", "layout.yaml", "test.yaml", "record_test_results", "mark_map_current", "Send no messages"} {
		if !strings.Contains(build, want) {
			t.Errorf("D8: build prompt lacks %q", want)
		}
	}
	update := indexingPrompt(owner, api.TurnContext{MapState: "stale",
		StaleFeatures:       []sysmap.StaleFeature{{ID: "create-order", Paths: []string{"create.py", "tests/test_create.py"}}},
		UndescribedFeatures: []string{"cancel-order"},
		UnmappedPaths:       []string{"README.md"},
	})
	for _, want := range []string{"- create-order: create.py, tests/test_create.py\n", "since the last mark_map_current: cancel-order.", "belong to no feature:\n- README.md\n", "Keep true procedure"} {
		if !strings.Contains(update, want) {
			t.Errorf("D8: update prompt lacks %q:\n%s", want, update)
		}
	}
}

func TestTurnNotices(t *testing.T) {
	if got := turnNotices(owner, api.TurnContext{MapState: "current"}); got != "" {
		t.Errorf("no notices expected, got %q", got)
	}
	got := turnNotices(owner, api.TurnContext{MapState: "stale", StaleFeatures: []sysmap.StaleFeature{{ID: "create-order", Paths: []string{"create.py"}}}, UnmappedPaths: []string{"README.md"}, VocabularyChanged: []string{"eng"}})
	for _, want := range []string{"Stale features: create-order (create.py).", "belong to no feature: README.md.", "Keep true procedure", "The vocabulary of #eng changed.", "Rename entries in your map"} {
		if !strings.Contains(got, want) {
			t.Errorf("D9: notices lack %q:\n%s", want, got)
		}
	}
	member := turnNotices(api.Self{Name: "writer"}, api.TurnContext{MapState: "stale", VocabularyChanged: []string{"product"}})
	if strings.Contains(member, "system documents") || strings.Contains(member, "Rename") || !strings.Contains(member, "#product") {
		t.Errorf("D9: a member without a system got %q", member)
	}
}

func TestBuiltinSkills(t *testing.T) {
	for name, wants := range map[string][]string{
		"skills/hikari-system/SKILL.md":                             {"name: hikari-system", "## Build", "## Keep true", "change_features", "hikari-feature-survey", "hikari-feature-review", "mark_map_current", "## Publish a release", "check_release", "publish_release", "/hikari/public/export/staging/<name>/", "references/features.md", "references/layout.md", "references/tests.md"},
		"skills/hikari-system/references/features.md":               {"## Purpose", "W1.", "W4.", "K1.", "K2.", "## What a feature is", "## How to link", "create", "modify", "rename", "remove"},
		"skills/hikari-system/references/layout.md":                 {"P2", "P3", "code:"},
		"skills/hikari-system/references/tests.md":                  {"P4", "covers", "gaps"},
		"skills/hikari-system/references/examples/reserve-stock.md": {"---\nid: reserve-stock\n", "## Purpose", "[[release-stock]]"},
		"agents/hikari-feature-survey.md":                           {"name: hikari-feature-survey", "tools: Read, Grep, Glob", "candidates", "evidence"},
		"agents/hikari-feature-review.md":                           {"name: hikari-feature-review", "tools: Read, Grep, Glob", "findings", "change_features"},
	} {
		data, err := builtin.ReadFile(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, want := range wants {
			if !strings.Contains(string(data), want) {
				t.Errorf("%s lacks %q", name, want)
			}
		}
	}
	// Every example parses as a feature document.
	examples, _ := fs.ReadDir(builtin, "skills/hikari-system/references/examples")
	if len(examples) != 3 {
		t.Errorf("want three examples, got %d", len(examples))
	}
	for _, e := range examples {
		data, _ := builtin.ReadFile("skills/hikari-system/references/examples/" + e.Name())
		d, err := featurewiki.Parse(string(data))
		if err != nil || d.ID+".md" != e.Name() {
			t.Errorf("example %s: %v", e.Name(), err)
		}
	}
}

func newTestShared(t *testing.T, owner bool) (*sharedArea, string) {
	t.Helper()
	root := t.TempDir()
	shared := filepath.Join(root, "shared")
	for _, d := range []string{"skills", "agents", "policies", "vocabulary", "deliverables"} {
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
	if _, err := os.Stat(filepath.Join(s.skillsDir(), "hikari-system", "references", "features.md")); err != nil {
		t.Errorf("H11: the skill package is installed with its references: %v", err)
	}
	for _, a := range []string{"hikari-feature-survey.md", "hikari-feature-review.md"} {
		if _, err := os.Stat(filepath.Join(s.agentsDir(), a)); err != nil {
			t.Errorf("H11: the agent %s is installed: %v", a, err)
		}
	}
	writeFile(t, filepath.Join(shared, "agents", "hikari-fake.md"), "fake")
	s.sync()
	if _, err := os.Lstat(filepath.Join(s.agentsDir(), "hikari-fake.md")); err == nil {
		t.Error("H12: a shared agent with the hikari- prefix is skipped")
	}
	data, _ := os.ReadFile(filepath.Join(s.skillsDir(), "hikari-system", "SKILL.md"))
	if !strings.HasPrefix(string(data), "---\nname: hikari-system") {
		t.Errorf("H4: the built-in hikari-system skill was replaced: %.40q", data)
	}
	if _, err := os.Lstat(filepath.Join(s.skillsDir(), "release-docs")); err == nil {
		t.Error("H7: there is no built-in release-docs skill")
	}

	// The skill of a held deliverable is linked, and it wins over a shared
	// skill of the same name.
	writeFile(t, filepath.Join(shared, "deliverables", "python-library", "SKILL.md"), "---\nname: python-library\n---\n")
	writeFile(t, filepath.Join(shared, "skills", "python-library", "SKILL.md"), "shared")
	s.deliverables = []string{"python-library"}
	s.sync()
	if target, _ := os.Readlink(filepath.Join(s.skillsDir(), "python-library")); target != filepath.Join(shared, "deliverables", "python-library") {
		t.Errorf("H8, H9: deliverable skill link = %q", target)
	}
	os.RemoveAll(filepath.Join(shared, "deliverables", "python-library"))
	s.sync()
	if _, err := os.Lstat(filepath.Join(s.skillsDir(), "python-library")); err == nil {
		t.Error("H10: the link of a removed deliverable must go")
	}
	os.RemoveAll(filepath.Join(shared, "skills", "python-library"))
	os.RemoveAll(filepath.Join(shared, "skills", "style"))
	os.Remove(filepath.Join(shared, "agents", "reviewer.md"))
	s.sync()
	if _, err := os.Lstat(filepath.Join(s.skillsDir(), "style")); err == nil {
		t.Error("H6: the link of a removed skill must go")
	}
	if _, err := os.Lstat(filepath.Join(s.agentsDir(), "reviewer.md")); err == nil {
		t.Error("H6: the link of a removed agent must go")
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
	if _, err := os.Stat(filepath.Join(s.agentsDir(), "hikari-feature-survey.md")); err == nil {
		t.Error("only owners get the built-in agents")
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

func TestDeliverablesPrompt(t *testing.T) {
	holder := owner
	holder.Deliverables = []api.DeliverableInfo{{Name: "python-library", Description: "A Python library."}}
	p := systemPrompt(holder)
	for _, want := range []string{
		"## Your deliverables",
		"- python-library: A Python library. Its definition is in /hikari/public/readonly/shared/deliverables/python-library/, and you build its product in /hikari/public/export/staging/python-library/.",
		"load the skill with the deliverable's name",
		"mcp__hikari__check_release",
		"mcp__hikari__publish_release",
		"one certificate for each",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("P12: prompt lacks %q", want)
		}
	}
	if strings.Contains(p, "You hold no deliverable") {
		t.Error("a holder must not be told it holds nothing")
	}
	if got := deliverableNames(holder); len(got) != 1 || got[0] != "python-library" {
		t.Errorf("deliverableNames = %v", got)
	}
	lead := api.Self{Name: "lead", Orchestrates: []string{"shop"}, VisibleSystems: []string{"orders"}, MemberDeliverables: map[string][]api.DeliverableInfo{"orders": {{Name: "python-library", Description: "A Python library."}}}}
	if p := systemPrompt(lead); !strings.Contains(p, "orders publishes python-library (A Python library.)") {
		t.Errorf("P14: the orchestrator must know its members' deliverables:\n%s", p)
	}
}

func TestMemberGrantsPrompt(t *testing.T) {
	lead := api.Self{Name: "lead", Orchestrates: []string{"shop"}, VisibleSystems: []string{"inventory", "orders"}, MemberGrants: map[string][]string{"orders": {"inventory"}}}
	if p := systemPrompt(lead); !strings.Contains(p, "orders reads the releases of inventory") {
		t.Errorf("the orchestrator must know its members' grants:\n%s", p)
	}
}
