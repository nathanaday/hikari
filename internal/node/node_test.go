package node

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/sysmap"
	"github.com/nathanaday/hikari/internal/testmap"
	"github.com/nathanaday/hikari/internal/wiki"
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
		"/hikari/public/export/map.yaml, from the features",
		"/hikari/public/export/wiki/",
		"mcp__hikari__change_wiki",
		"/hikari/private/tests.yaml, the test map",
		"mcp__hikari__record_test_run for each run",
		"Releases follow your deliverables",
		"You hold no deliverable, so you cannot publish a release",
		"/hikari/public/readonly/shared holds what every agent",
		"load the hikari-system skill",
		"mcp__hikari__get_vocabulary",
		`Commits go to the branch "hikari/work".`,
		"no trailers such as Co-Authored-By",
		"run the suites that touch the affected features",
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
		"/hikari/public/readonly/systems/<system>/wiki/",
		"get_document returns a document",
		"You keep the wiki of #eng",
		"mcp__hikari__change_hub_wiki",
		"load the hikari-hub skill",
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
	for _, want := range []string{"not built yet", "Load the hikari-system skill", "Build procedure", "set_system_info", "change_wiki", "hikari-feature-survey", "layout.yaml", "tests.yaml", "/hikari/private/reports/<suite>.xml", "record_test_run", "hikari-test-survey", "mark_map_current", "Send no messages"} {
		if !strings.Contains(build, want) {
			t.Errorf("D8: build prompt lacks %q", want)
		}
	}
	update := indexingPrompt(owner, api.TurnContext{MapState: "stale",
		StaleFeatures:       []sysmap.StaleFeature{{ID: "create-order", Paths: []string{"create.py", "tests/test_create.py"}}},
		UndescribedFeatures: []string{"cancel-order"},
		UnmappedPaths:       []string{"README.md"},
		FailingFeatures:     []api.FailingFeature{{ID: "create-order", Tests: []string{"t > a"}}},
		PendingFeatures:     []string{"cancel-order"},
	})
	for _, want := range []string{"- create-order: create.py, tests/test_create.py\n", "since the last mark_map_current: cancel-order.", "belong to no feature:\n- README.md\n", "Failing features, with their failing tests:\n- create-order: t > a\n", "Pending features, whose tests have not run since their code changed: cancel-order.", "Keep true procedure", "record_test_run"} {
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
		"skills/hikari-system/SKILL.md":                             {"name: hikari-system", "## Build", "## Keep true", "## Test", "record_test_run", "tests.yaml", "hikari-test-survey", "change_wiki", "hikari-feature-survey", "hikari-feature-review", "mark_map_current", "## Publish a release", "check_release", "publish_release", "/hikari/public/export/staging/<name>/", "references/wiki.md", "references/layout.md", "references/tests.md"},
		"skills/hikari-system/references/wiki.md":                   {"## Purpose", "W1.", "W4.", "K1.", "K2.", "## The kinds", "principle", "## What a feature is", "## How to link", "## The hub wikis", "create", "modify", "rename", "remove"},
		"skills/hikari-hub/SKILL.md":                                {"name: hikari-hub", "change_hub_wiki", "story", "rule", "principle", "key", "## Plans and notes", "## Carry out a plan"},
		"skills/hikari-system/references/layout.md":                 {"P2", "P3", "code:"},
		"skills/hikari-system/references/tests.md":                  {"X1.", "X3.", "gaps", "/hikari/private/reports/<suite>.xml", "junit_family=xunit1", "--reporter=junit", "untested", "failing", "pending", "passing", "record_test_run"},
		"skills/hikari-system/references/examples/tests.yaml":       {"version: 1", "tests/test_stock.py", "--junitxml=/hikari/private/reports/all.xml"},
		"agents/hikari-test-survey.md":                              {"name: hikari-test-survey", "tools: Read, Grep, Glob", "keys", "gaps"},
		"skills/hikari-system/references/examples/reserve-stock.md": {"---\nid: reserve-stock\n", "## Purpose", "[[release-stock]]"},
		"agents/hikari-feature-survey.md":                           {"name: hikari-feature-survey", "tools: Read, Grep, Glob", "candidates", "evidence"},
		"agents/hikari-feature-review.md":                           {"name: hikari-feature-review", "tools: Read, Grep, Glob", "findings", "change_wiki"},
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
	// Every markdown example parses as a feature document, and the test
	// map example is valid against them.
	examples, _ := fs.ReadDir(builtin, "skills/hikari-system/references/examples")
	ids := map[string]bool{}
	for _, e := range examples {
		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		ids[strings.TrimSuffix(e.Name(), ".md")] = true
	}
	if len(ids) != 3 {
		t.Errorf("want three feature examples, got %d", len(ids))
	}
	data, _ := builtin.ReadFile("skills/hikari-system/references/examples/tests.yaml")
	tm, err := testmap.Parse(data)
	if err == nil {
		err = tm.Validate(map[string]bool{"reserve-stock": true, "check-availability": true, "restock-sku": true})
	}
	if err != nil {
		t.Errorf("examples/tests.yaml: %v", err)
	}
	for _, e := range examples {
		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, _ := builtin.ReadFile("skills/hikari-system/references/examples/" + e.Name())
		d, err := wiki.Parse(string(data))
		if err != nil || d.ID+".md" != e.Name() || d.KindOf() != wiki.Feature {
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
	var builtins []string
	if owner {
		builtins = []string{api.SystemSkill}
	}
	return &sharedArea{home: filepath.Join(root, "home"), dir: shared, builtins: builtins, warned: map[string]bool{}}, shared
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
	if _, err := os.Stat(filepath.Join(s.skillsDir(), "hikari-system", "references", "wiki.md")); err != nil {
		t.Errorf("H11: the skill package is installed with its references: %v", err)
	}
	for _, a := range []string{"hikari-feature-survey.md", "hikari-feature-review.md", "hikari-test-survey.md"} {
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

func TestHealthNotice(t *testing.T) {
	var failing []api.FailingFeature
	for i := range 22 {
		failing = append(failing, api.FailingFeature{ID: fmt.Sprintf("f%02d", i), Tests: []string{"a", "b", "c", "d", "e", "f", "g"}})
	}
	got := turnNotices(owner, api.TurnContext{MapState: "current", FailingFeatures: failing, PendingFeatures: []string{"p1", "p2"}})
	for _, want := range []string{"Failing features: f00 (a, b, c, d, e, and 2 more); ", "f19 (", "; and 2 more.", "Pending features: p1, p2.", "Run the suites that touch them, call record_test_run for each run"} {
		if !strings.Contains(got, want) {
			t.Errorf("D10: notice lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "f20 (") {
		t.Errorf("D10: at most 20 features:\n%s", got)
	}
	if got := turnNotices(owner, api.TurnContext{MapState: "current"}); got != "" {
		t.Errorf("D10: no notice when every feature is passing or untested: %q", got)
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
	for _, want := range []string{"## Releases you can use", "- inventory: /hikari/public/readonly/systems/inventory/releases", "name its system and version in the description of the suite in tests.yaml"} {
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

func TestHubSkillAndWikiPrompts(t *testing.T) {
	lead := api.Self{Name: "lead", Orchestrates: []string{"shop"}, HubWikis: []string{"shop"}}
	if got := builtinSkills(lead); !slices.Equal(got, []string{api.HubSkill}) {
		t.Errorf("S2: an orchestrator gets %v", got)
	}
	if got := builtinSkills(owner); !slices.Equal(got, []string{api.SystemSkill}) {
		t.Errorf("an owner gets %v", got)
	}
	s, shared := newTestShared(t, false)
	s.builtins = []string{api.HubSkill}
	writeFile(t, filepath.Join(shared, "skills", "hikari-hub", "SKILL.md"), "fake")
	s.sync()
	if data, err := os.ReadFile(filepath.Join(s.skillsDir(), "hikari-hub", "SKILL.md")); err != nil || !strings.Contains(string(data), "change_hub_wiki") {
		t.Errorf("S2, S4: the hub skill comes from hikari: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.agentsDir(), "hikari-feature-survey.md")); err == nil {
		t.Error("the owner's agents are not installed for an orchestrator")
	}

	member := api.Self{Name: "orders", HubWikis: []string{"shop"}, System: owner.System}
	p := systemPrompt(member)
	for _, want := range []string{"## The wikis of your hubs", "- #shop: /hikari/public/readonly/hubs/shop/wiki", "act on a plan only when the user asks", "The rules and principles of your hubs apply to your system"} {
		if !strings.Contains(p, want) {
			t.Errorf("A1, E2: prompt lacks %q", want)
		}
	}
	n := turnNotices(member, api.TurnContext{WikiChanged: []api.WikiChange{
		{Wiki: "#shop", Docs: []api.DocChange{{ID: "order-items", Kind: "story"}, {ID: "old", Removed: true}}},
		{Wiki: "orders", Docs: []api.DocChange{{ID: "Roadmap", Kind: "plan"}}},
	}})
	for _, want := range []string{"The wiki of #shop changed: order-items (story), old (removed).", "Your wiki changed: Roadmap (plan)."} {
		if !strings.Contains(n, want) {
			t.Errorf("E1: notices lack %q:\n%s", want, n)
		}
	}
}

func TestRestorePrompt(t *testing.T) {
	tests := restorePrompt(api.TurnContext{NoLocalResults: true})
	for _, want := range []string{"fresh clone", "another machine recorded", "record_test_run", "Send no messages"} {
		if !strings.Contains(tests, want) {
			t.Errorf("SY10 restore prompt lacks %q:\n%s", want, tests)
		}
	}
	n := turnNotices(owner, api.TurnContext{MapState: "current", NoLocalResults: true})
	if !strings.Contains(n, "Run each suite") {
		t.Errorf("SY10 notice lacks the restore step:\n%s", n)
	}
}
