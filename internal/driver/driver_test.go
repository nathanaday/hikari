package driver

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/featuremap"
	"github.com/nathanaday/hikari/internal/modules"
	"github.com/nathanaday/hikari/internal/testmap"
	"github.com/nathanaday/hikari/internal/wiki"
)

var room = []api.Peer{
	{Name: "lead", Role: api.RoleOrchestration, Description: "leads engineering"},
	{Name: "dev", Role: api.RolePartition, Description: "writes code"},
	{Name: "orders", Role: api.RolePartition},
	{Name: "user", Role: api.RoleOperator, Description: "The human operator."},
}

func TestSystemPromptStatesTheRoom(t *testing.T) {
	for name, c := range map[string]struct {
		self api.Self
		want []string
	}{
		"orchestration": {api.Self{Name: "lead", Orchestration: true, Participants: room}, []string{
			"## The room",
			"- lead (you), orchestration: leads engineering",
			"- dev, partition: writes code",
			"- user, operator: The human operator.",
			"mcp__hikari__post sends a message to the room",
			"mcp__hikari__list_agents",
			"You are the orchestration agent.",
			"for every partition agent when you post without mentions",
		}},
		"partition agent": {api.Self{Name: "dev", Participants: room}, []string{
			"- dev (you), partition: writes code",
			"You are a partition agent. lead coordinates the work.",
			"A post from you starts a turn for lead and for each agent you mention.",
		}},
		"no orchestration agent": {api.Self{Name: "dev", Participants: room[1:]}, []string{
			"The project has no orchestration agent",
			"for every agent when it mentions nobody",
		}},
	} {
		t.Run(name, func(t *testing.T) {
			p := systemPrompt(c.self)
			for _, want := range c.want {
				if !strings.Contains(p, want) {
					t.Errorf("H19: system prompt lacks %q:\n%s", want, p)
				}
			}
		})
	}
}

var partitionAgent = api.Self{
	Name:         "orders",
	Participants: room,
	Partition: &api.SelfPartition{
		Folders: []api.Folder{
			{Place: "shop/services/orders", Repo: "shop", Path: "services/orders"},
			{Place: "shop/proto", Repo: "shop", Path: "proto", ReadOnly: true},
			{Place: "protos", Repo: "protos", Path: "orders"},
		},
		Repos:   []api.RepoState{{Name: "shop", Branch: "hikari/work"}, {Name: "protos", Branch: "hikari/work"}},
		WorkDir: "/workspace",
	},
}

func TestPartitionAgentSystemPrompt(t *testing.T) {
	p := systemPrompt(partitionAgent)
	for _, want := range []string{
		`You are the partition agent of the partition "orders". Your working directory, /workspace, is your workspace`,
		"- /workspace/shop/services/orders: services/orders of the repo shop; you can change it\n",
		"- /workspace/shop/proto: proto of the repo shop; read-only\n",
		"- /workspace/protos: orders of the repo protos; you can change it\n",
		"workspace paths, relative to /workspace, such as shop/services/orders/app.py",
		`on its branch: "hikari/work" in shop, "hikari/work" in protos`,
		"/hikari/self/features/map.yaml, from the features",
		"/hikari/self/features/",
		"/hikari/private/code-map.yaml, the code map",
		"/hikari/self/releases/",
		"/hikari/can-edit/ files that the user gives you to change",
		"mcp__hikari__change_wiki",
		"/hikari/private/tests.yaml, the test map",
		"mcp__hikari__record_test_run for each run",
		"Releases follow your deliverables",
		"You hold no deliverable, so you cannot publish a release",
		"/hikari/public holds what every agent",
		"load the hikari-map-features skill",
		"load the hikari-plan-work skill first",
		"Hikari gives you these skills, for the tasks of your role: hikari-plan-work, hikari-do-work, hikari-validate-work, hikari-map-features.",
		"mcp__hikari__get_vocabulary",
		"no trailers such as Co-Authored-By",
		"run the suites that touch the affected features",
		"tell the orchestration agent, or the partition agent of that partition, what you need",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("D4: partition agent prompt lacks %q", want)
		}
	}
	if strings.Contains(p, "You keep the vocabulary") {
		t.Error("a partition agent must not get the orchestration agent section")
	}
}

func TestOrchestrationSystemPrompt(t *testing.T) {
	lead := api.Self{Name: "lead", Orchestration: true, Participants: room, VisiblePartitions: []string{"inventory", "orders"}}
	p := systemPrompt(lead)
	for _, want := range []string{
		"These are the project's partitions, each named after its partition agent: inventory, orders. You have no code and cannot read any.",
		"mcp__hikari__list_partitions",
		"When a map does not answer a question, ask the partition agent.",
		"You keep the vocabulary",
		"get_partition flags interface names",
		"/hikari/partitions/<partition>/: features/map.yaml",
		"/hikari/partitions/<partition>/features/",
		"/hikari/self/granted/ holds files",
		"get_document returns a document",
		"You keep the public wiki, /hikari/public/wiki",
		"mcp__hikari__change_public_wiki",
		"load the hikari-map-features skill",
		"load the hikari-plan-work skill",
		"load the hikari-health-check skill",
		"Your feature map, /hikari/self/features/",
		"Hikari gives you these skills, for the tasks of your role: hikari-plan-work, hikari-health-check, hikari-map-features.",
		"When a partition announces a release, check it",
		"the deliverable's definition",
		"only through that partition's published releases",
		"No partition agent is granted another partition's releases",
		"No partition agent holds a deliverable",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("D5: orchestration agent prompt lacks %q", want)
		}
	}
	if strings.Contains(p, "Your partition\n") {
		t.Error("an orchestration agent must not get the partition agent section")
	}
	plain := systemPrompt(api.Self{Name: "lead", Orchestration: true, Participants: room})
	if strings.Contains(plain, "Your partitions") || !strings.Contains(plain, "You keep the vocabulary") {
		t.Errorf("an orchestration agent without partitions gets only the vocabulary section:\n%s", plain)
	}
}

func TestIndexingPrompt(t *testing.T) {
	build := indexingPrompt(partitionAgent, api.TurnContext{MapState: "unbuilt"})
	for _, want := range []string{"not built yet", "Load the hikari-map-features skill", "Build procedure", "set_partition_info", "change_wiki", "hikari-feature-survey", "code-map.yaml", "tests.yaml", "/hikari/private/reports/<suite>.xml", "record_test_run", "hikari-test-survey", "mark_map_current", "Send no messages"} {
		if !strings.Contains(build, want) {
			t.Errorf("D8: build prompt lacks %q", want)
		}
	}
	update := indexingPrompt(partitionAgent, api.TurnContext{MapState: "stale",
		StaleFeatures:       []featuremap.StaleFeature{{ID: "create-order", Paths: []string{"create.py", "tests/test_create.py"}}},
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
	if got := turnNotices(partitionAgent, api.TurnContext{MapState: "current"}); got != "" {
		t.Errorf("no notices expected, got %q", got)
	}
	got := turnNotices(partitionAgent, api.TurnContext{MapState: "stale", StaleFeatures: []featuremap.StaleFeature{{ID: "create-order", Paths: []string{"create.py"}}}, UnmappedPaths: []string{"README.md"}, VocabularyChanged: true})
	for _, want := range []string{"Stale features: create-order (create.py).", "belong to no feature: README.md.", "Keep true procedure", "The vocabulary changed.", "Rename entries in your map"} {
		if !strings.Contains(got, want) {
			t.Errorf("D9: notices lack %q:\n%s", want, got)
		}
	}
	lead := turnNotices(api.Self{Name: "lead", Orchestration: true}, api.TurnContext{MapState: "stale", VocabularyChanged: true})
	if strings.Contains(lead, "partition documents") || strings.Contains(lead, "Rename") || !strings.Contains(lead, "The vocabulary changed.") {
		t.Errorf("D9: the orchestration agent got %q", lead)
	}
}

func TestBuiltinSkills(t *testing.T) {
	for name, wants := range map[string][]string{
		"skills/partition/hikari-map-features/SKILL.md":                             {"name: hikari-map-features", "## Build", "## Keep true", "tests.yaml", "hikari-test-survey", "change_wiki", "set_partition_info", "hikari-feature-survey", "hikari-feature-review", "mark_map_current", "references/wiki.md", "references/code-map.md", "references/tests.md"},
		"skills/partition/hikari-map-features/references/wiki.md":                   {"## Purpose", "W1.", "W4.", "K1.", "K2.", "## The kinds", "principle", "## What a feature is", "## How to link", "## The public wiki", "create", "modify", "rename", "remove"},
		"skills/partition/hikari-map-features/references/code-map.md":               {"P2", "P3", "code:"},
		"skills/partition/hikari-map-features/references/tests.md":                  {"X1.", "X3.", "gaps", "/hikari/private/reports/<suite>.xml", "junit_family=xunit1", "--reporter=junit", "untested", "failing", "pending", "passing", "record_test_run"},
		"skills/partition/hikari-map-features/references/examples/tests.yaml":       {"version: 1", "tests/test_stock.py", "--junitxml=/hikari/private/reports/all.xml"},
		"skills/partition/hikari-map-features/references/examples/reserve-stock.md": {"---\nid: reserve-stock\n", "## Purpose", "[[release-stock]]"},
		"skills/partition/hikari-plan-work/SKILL.md":                                {"name: hikari-plan-work", "more than one feature", "post the plan in the room", "interfaces", "wait for an answer"},
		"skills/partition/hikari-do-work/SKILL.md":                                  {"name: hikari-do-work", "code-map.yaml", "`apply`", "## Publish a release", "check_release", "publish_release", "/hikari/staging/<name>/"},
		"skills/partition/hikari-validate-work/SKILL.md":                            {"name: hikari-validate-work", "## Test", "## Problems", "record_test_run", "get_my_partition", "../hikari-map-features/references/tests.md"},
		"skills/orchestration/hikari-map-features/SKILL.md":                         {"name: hikari-map-features", "change_wiki", "change_public_wiki", "/hikari/self/features/", "feature", "rule", "principle", "## The vocabulary", "define_term"},
		"skills/orchestration/hikari-plan-work/SKILL.md":                            {"name: hikari-plan-work", "which partitions", "## Plans and notes", "## Carry out a plan"},
		"skills/orchestration/hikari-health-check/SKILL.md":                         {"name: hikari-health-check", "list_partitions", "get_partition", "vocabulary", "@name"},
		"agents/hikari-test-survey.md":                                              {"name: hikari-test-survey", "tools: Read, Grep, Glob", "keys", "gaps", "hikari-map-features"},
		"agents/hikari-feature-survey.md":                                           {"name: hikari-feature-survey", "tools: Read, Grep, Glob", "candidates", "evidence", "hikari-map-features"},
		"agents/hikari-feature-review.md":                                           {"name: hikari-feature-review", "tools: Read, Grep, Glob", "findings", "change_wiki", "hikari-map-features"},
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
	examples, _ := fs.ReadDir(builtin, "skills/partition/hikari-map-features/references/examples")
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
	data, _ := builtin.ReadFile("skills/partition/hikari-map-features/references/examples/tests.yaml")
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
		data, _ := builtin.ReadFile("skills/partition/hikari-map-features/references/examples/" + e.Name())
		d, err := wiki.Parse(string(data))
		if err != nil || d.ID+".md" != e.Name() || d.KindOf() != wiki.Feature {
			t.Errorf("example %s: %v", e.Name(), err)
		}
	}
}

func newTestShared(t *testing.T, role string) (*sharedArea, string, string) {
	t.Helper()
	root := t.TempDir()
	public, skills := filepath.Join(root, "public"), filepath.Join(root, "skills")
	for _, d := range []string{"policies", "vocabulary", "deliverables"} {
		os.MkdirAll(filepath.Join(public, d), 0o755)
	}
	os.MkdirAll(skills, 0o755)
	return &sharedArea{home: filepath.Join(root, "home"), public: public, skills: skills, role: role, builtins: api.Packs[role], warned: map[string]bool{}}, public, skills
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSharedAreaSync(t *testing.T) {
	s, public, skills := newTestShared(t, api.RolePartition)
	writeFile(t, filepath.Join(skills, "style", "SKILL.md"), "---\nname: style\n---\n")
	writeFile(t, filepath.Join(skills, "other", "SKILL.md"), "---\nname: other\n---\n")
	writeFile(t, filepath.Join(skills, "empty-dir", "notes.txt"), "no skill here")
	writeFile(t, filepath.Join(public, "policies", "b-security.md"), "Never log secrets.")
	writeFile(t, filepath.Join(public, "policies", "a-style.md"), "Write short sentences.")
	s.granted = []string{"style", "empty-dir", "hikari-map-features"}

	policies := s.sync()
	if target, _ := os.Readlink(filepath.Join(s.skillsDir(), "style")); target != filepath.Join(skills, "style") {
		t.Errorf("V7: style skill link = %q", target)
	}
	if _, err := os.Lstat(filepath.Join(s.skillsDir(), "other")); err == nil {
		t.Error("V7: a skill that is not granted must not be linked")
	}
	if _, err := os.Lstat(filepath.Join(s.skillsDir(), "empty-dir")); err == nil {
		t.Error("V7: a directory without SKILL.md must not be linked")
	}
	if entries, _ := os.ReadDir(s.agentsDir()); len(entries) != 3 {
		t.Errorf("V8: only hikari's own agents are installed: %v", entries)
	}
	if !strings.Contains(policies, "# Policies") || strings.Index(policies, "## a-style") > strings.Index(policies, "## b-security") || !strings.Contains(policies, "Never log secrets.") {
		t.Errorf("H3: policies:\n%s", policies)
	}
	if _, err := os.Stat(filepath.Join(s.skillsDir(), "hikari-map-features", "references", "wiki.md")); err != nil {
		t.Errorf("H11: the skill package is installed with its references: %v", err)
	}
	for _, a := range []string{"hikari-feature-survey.md", "hikari-feature-review.md", "hikari-test-survey.md"} {
		if _, err := os.Stat(filepath.Join(s.agentsDir(), a)); err != nil {
			t.Errorf("H11: the agent %s is installed: %v", a, err)
		}
	}
	data, _ := os.ReadFile(filepath.Join(s.skillsDir(), "hikari-map-features", "SKILL.md"))
	if !strings.HasPrefix(string(data), "---\nname: hikari-map-features") {
		t.Errorf("H4: the built-in hikari-map-features skill was replaced: %.40q", data)
	}

	// The skill of a held deliverable is linked, and it wins over a granted
	// skill of the same name.
	writeFile(t, filepath.Join(public, "deliverables", "python-library", "SKILL.md"), "---\nname: python-library\n---\n")
	writeFile(t, filepath.Join(skills, "python-library", "SKILL.md"), "granted")
	s.deliverables = []string{"python-library"}
	s.granted = append(s.granted, "python-library")
	s.sync()
	if target, _ := os.Readlink(filepath.Join(s.skillsDir(), "python-library")); target != filepath.Join(public, "deliverables", "python-library") {
		t.Errorf("H8, H9: deliverable skill link = %q", target)
	}
	os.RemoveAll(filepath.Join(public, "deliverables", "python-library"))
	s.sync()
	if _, err := os.Lstat(filepath.Join(s.skillsDir(), "python-library")); err == nil {
		t.Error("H10: the link of a removed deliverable must go")
	}
	os.RemoveAll(filepath.Join(skills, "style"))
	s.sync()
	if _, err := os.Lstat(filepath.Join(s.skillsDir(), "style")); err == nil {
		t.Error("H6: the link of a removed skill must go")
	}
}

// TestPacks checks that each kind of agent gets its own pack and no skill
// of the other pack (rules K1 and K16 of the skill packs spec).
func TestPacks(t *testing.T) {
	for self, want := range map[*api.Self][]string{
		&partitionAgent:                     {"hikari-do-work", "hikari-map-features", "hikari-plan-work", "hikari-validate-work"},
		{Name: "lead", Orchestration: true}: {"hikari-health-check", "hikari-map-features", "hikari-plan-work"},
	} {
		role := packRole(*self)
		s, _, _ := newTestShared(t, role)
		// A skill of the other pack, or of an earlier version, is removed.
		writeFile(t, filepath.Join(s.skillsDir(), "hikari-partition", "SKILL.md"), "old")
		other := "hikari-health-check"
		if role == api.RoleOrchestration {
			other = "hikari-do-work"
		}
		writeFile(t, filepath.Join(s.skillsDir(), other, "SKILL.md"), "other pack")
		s.sync()
		entries, _ := os.ReadDir(s.skillsDir())
		var got []string
		for _, e := range entries {
			got = append(got, e.Name())
		}
		if !slices.Equal(got, want) {
			t.Errorf("K1: %s gets %v, want %v", role, got, want)
		}
		for _, name := range want {
			data, _ := os.ReadFile(filepath.Join(s.skillsDir(), name, "SKILL.md"))
			embedded, _ := builtin.ReadFile("skills/" + role + "/" + name + "/SKILL.md")
			if len(data) == 0 || string(data) != string(embedded) {
				t.Errorf("K1: %s's %s is not its own pack's", role, name)
			}
		}
		_, err := os.Stat(filepath.Join(s.agentsDir(), "hikari-feature-survey.md"))
		if (err == nil) != (role == api.RolePartition) {
			t.Errorf("K3: %s and the agent definitions: %v", role, err)
		}
	}
	for role, pack := range api.Packs {
		entries, _ := fs.ReadDir(builtin, "skills/"+role)
		if len(entries) != len(pack) {
			t.Errorf("K1: the binary embeds %d skills for %s; its pack has %d", len(entries), role, len(pack))
		}
	}
}

func TestSharedAreaForNonPartitionAgents(t *testing.T) {
	s, _, _ := newTestShared(t, "")
	if got := s.sync(); got != "" {
		t.Errorf("no policies, no section: %q", got)
	}
	if entries, _ := os.ReadDir(s.skillsDir()); len(entries) != 0 {
		t.Errorf("an agent of no role gets no built-in skill: %v", entries)
	}
	if _, err := os.Stat(filepath.Join(s.agentsDir(), "hikari-feature-survey.md")); err == nil {
		t.Error("only partition agents get the built-in agents")
	}
}

func TestHealthNotice(t *testing.T) {
	var failing []api.FailingFeature
	for i := range 22 {
		failing = append(failing, api.FailingFeature{ID: fmt.Sprintf("f%02d", i), Tests: []string{"a", "b", "c", "d", "e", "f", "g"}})
	}
	got := turnNotices(partitionAgent, api.TurnContext{MapState: "current", FailingFeatures: failing, PendingFeatures: []string{"p1", "p2"}})
	for _, want := range []string{"Failing features: f00 (a, b, c, d, e, and 2 more); ", "f19 (", "; and 2 more.", "Pending features: p1, p2.", "Run the suites that touch them, call record_test_run for each run"} {
		if !strings.Contains(got, want) {
			t.Errorf("D10: notice lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "f20 (") {
		t.Errorf("D10: at most 20 features:\n%s", got)
	}
	if got := turnNotices(partitionAgent, api.TurnContext{MapState: "current"}); got != "" {
		t.Errorf("D10: no notice when every feature is passing or untested: %q", got)
	}
}

func TestTurnNoticesReportPrivateErrors(t *testing.T) {
	got := turnNotices(partitionAgent, api.TurnContext{MapState: "current", PrivateErrors: []string{`P3: code-map.yaml: path "x.py" is in no folder of the workspace, or does not exist`}})
	if !strings.Contains(got, "Your private documents have problems: P3:") {
		t.Errorf("notices: %q", got)
	}
}

func TestGrantsPrompt(t *testing.T) {
	consumer := partitionAgent
	consumer.Releases = []string{"inventory"}
	p := systemPrompt(consumer)
	for _, want := range []string{"## Releases you can use", "- inventory: /hikari/grants/inventory/releases", "name its partition and version in the description of the suite in tests.yaml"} {
		if !strings.Contains(p, want) {
			t.Errorf("P10: prompt lacks %q", want)
		}
	}
	lead := api.Self{Name: "lead", Orchestration: true, VisiblePartitions: []string{"orders"}, Releases: []string{"orders"}}
	if strings.Contains(systemPrompt(lead), "## Releases you can use") {
		t.Error("an orchestration agent's own partitions are not grants")
	}
}

func TestDeliverablesPrompt(t *testing.T) {
	holder := partitionAgent
	holder.Deliverables = []api.DeliverableInfo{{Name: "python-library", Description: "A Python library."}}
	p := systemPrompt(holder)
	for _, want := range []string{
		"## Your deliverables",
		"- python-library: A Python library. Its definition is in /hikari/public/deliverables/python-library/, and you build its product in /hikari/staging/python-library/.",
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
	lead := api.Self{Name: "lead", Orchestration: true, VisiblePartitions: []string{"orders"}, PartitionAgentDeliverables: map[string][]api.DeliverableInfo{"orders": {{Name: "python-library", Description: "A Python library."}}}}
	if p := systemPrompt(lead); !strings.Contains(p, "orders publishes python-library (A Python library.)") {
		t.Errorf("P14: the orchestration agent must know the partition agents' deliverables:\n%s", p)
	}
}

func TestPartitionAgentGrantsPrompt(t *testing.T) {
	lead := api.Self{Name: "lead", Orchestration: true, VisiblePartitions: []string{"inventory", "orders"}, PartitionAgentGrants: map[string][]string{"orders": {"inventory"}}}
	if p := systemPrompt(lead); !strings.Contains(p, "orders reads the releases of inventory") {
		t.Errorf("the orchestration agent must know the partition agents' grants:\n%s", p)
	}
}

func TestOrchestrationSkillAndWikiPrompts(t *testing.T) {
	s, _, skills := newTestShared(t, api.RoleOrchestration)
	s.granted = []string{"hikari-map-features"}
	writeFile(t, filepath.Join(skills, "hikari-map-features", "SKILL.md"), "fake")
	s.sync()
	if data, err := os.ReadFile(filepath.Join(s.skillsDir(), "hikari-map-features", "SKILL.md")); err != nil || !strings.Contains(string(data), "change_public_wiki") {
		t.Errorf("S2, S4: the orchestration agent's map-features skill comes from hikari: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.agentsDir(), "hikari-feature-survey.md")); err == nil {
		t.Error("the partition agent's agents are not installed for an orchestration agent")
	}

	member := api.Self{Name: "orders", Participants: room, Partition: partitionAgent.Partition}
	p := systemPrompt(member)
	for _, want := range []string{"## The public wiki", "The public wiki, /hikari/public/wiki,", "act on a plan only when the user asks", "The rules and principles of the public wiki apply to your partition"} {
		if !strings.Contains(p, want) {
			t.Errorf("A1, E2: prompt lacks %q", want)
		}
	}
	n := turnNotices(member, api.TurnContext{WikiChanged: []api.WikiChange{
		{Wiki: "public", Docs: []api.DocChange{{ID: "stock-never-negative", Kind: "rule"}, {ID: "old", Removed: true}}},
		{Wiki: "orders", Docs: []api.DocChange{{ID: "Roadmap", Kind: "plan"}}},
	}})
	for _, want := range []string{"The public wiki changed: stock-never-negative (rule), old (removed).", "Your wiki changed: Roadmap (plan)."} {
		if !strings.Contains(n, want) {
			t.Errorf("E1: notices lack %q:\n%s", want, n)
		}
	}
}

func TestRestorePrompt(t *testing.T) {
	tests := restorePrompt(api.TurnContext{NoLocalResults: true})
	for _, want := range []string{"fresh clone", "another machine recorded", "Load the hikari-validate-work skill", "record_test_run", "Send no messages"} {
		if !strings.Contains(tests, want) {
			t.Errorf("SY10 restore prompt lacks %q:\n%s", want, tests)
		}
	}
	n := turnNotices(partitionAgent, api.TurnContext{MapState: "current", NoLocalResults: true})
	if !strings.Contains(n, "run each suite") {
		t.Errorf("SY10 notice lacks the restore step:\n%s", n)
	}
}

// TestModulePacks checks MO4 of the task pipeline spec: a module's skills
// join the pack of each kind of agent, and leave it when the module is off.
func TestModulePacks(t *testing.T) {
	for role, want := range map[string]string{api.RolePartition: "hikari-task-work", api.RoleOrchestration: "hikari-scope-work"} {
		s, _, _ := newTestShared(t, role)
		self := api.Self{Name: "x", Modules: []string{"tasks"}}
		s.builtins = modules.Pack(role, self.Modules)
		s.extra = map[string]fs.FS{}
		for _, m := range modules.Named(self.Modules) {
			for _, sk := range m.Skills(role) {
				s.extra[sk.Name] = sk.Files
			}
		}
		s.sync()
		data, err := os.ReadFile(filepath.Join(s.skillsDir(), want, "SKILL.md"))
		if err != nil || !strings.HasPrefix(string(data), "---\nname: "+want+"\n") {
			t.Errorf("MO4 %s gets %s: %v", role, want, err)
		}
		if p := sharedPrompt(api.Self{Partition: map[bool]*api.SelfPartition{true: {}}[role == api.RolePartition], Orchestration: role == api.RoleOrchestration, Modules: []string{"tasks"}}); !strings.Contains(p, want) {
			t.Errorf("MO4 the prompt lists %s:\n%s", want, p)
		}
		s.builtins, s.extra = api.Packs[role], nil
		s.sync()
		if _, err := os.Stat(filepath.Join(s.skillsDir(), want)); err == nil {
			t.Errorf("MO5 without the module, %s goes", want)
		}
	}
}

func TestTrigger(t *testing.T) {
	msgs := []api.Message{{From: "user"}, {From: "lead", To: "dev"}, {From: "user"}}
	if got := trigger(msgs); got != "woken by user in the room, lead directly" {
		t.Errorf("trigger = %q", got)
	}
}
