package node

import (
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
