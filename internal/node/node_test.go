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
		".hikari/map.yaml, which you change only through the map tools",
		".hikari/private/layout.yaml and .hikari/private/test.yaml, which you edit directly",
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
		"no test in this network runs across two systems",
		"You keep the vocabulary of #eng",
		"get_system flags interface names",
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

func TestSkill(t *testing.T) {
	text := string(skill)
	if !strings.HasPrefix(text, "---\nname: hikari-system\ndescription: ") {
		t.Errorf("the skill must start with its front matter:\n%.120s", text)
	}
	for _, want := range []string{"## layout.yaml", "## test.yaml", "P1:", "P4:", "mark_map_current"} {
		if !strings.Contains(text, want) {
			t.Errorf("the skill lacks %q", want)
		}
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := writeSkill(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude", "skills", "hikari-system", "SKILL.md"))
	if err != nil || string(data) != text {
		t.Errorf("writeSkill: %v", err)
	}
}

func TestTurnNoticesReportPrivateErrors(t *testing.T) {
	got := turnNotices(owner, api.TurnContext{MapState: "current", PrivateErrors: []string{`P3: layout.yaml: path "x.py" does not exist in the scope`}})
	if !strings.Contains(got, "Your private documents have problems: P3:") {
		t.Errorf("notices: %q", got)
	}
}
