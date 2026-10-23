package mcpserver

import (
	"context"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nathanaday/hikari/internal/api"
)

func toolNames(t *testing.T, self api.Self) []string {
	t.Helper()
	ctx := context.Background()
	serverT, clientT := mcp.NewInMemoryTransports()
	if _, err := New(&api.Client{}, self).Connect(ctx, serverT, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	res, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	return names
}

var (
	everyone     = []string{"check_inbox", "get_document", "get_vocabulary", "list_hubs", "post_to_hub", "search_wiki", "send_direct_message"}
	ownerOnly    = []string{"change_wiki", "check_release", "commit_changes", "get_my_system", "mark_map_current", "publish_release", "record_test_run", "set_system_info"}
	orchestrator = []string{"change_hub_wiki", "define_interface", "define_term", "get_system", "list_systems", "remove_entry"}
)

func sorted(lists ...[]string) []string {
	var out []string
	for _, l := range lists {
		out = append(out, l...)
	}
	slices.Sort(out)
	return out
}

func TestToolsByRole(t *testing.T) {
	cases := map[string]struct {
		self api.Self
		want []string
	}{
		"member":       {api.Self{Name: "writer"}, sorted(everyone)},
		"owner":        {api.Self{Name: "orders", System: &api.SelfSystem{}}, sorted(everyone, ownerOnly)},
		"orchestrator": {api.Self{Name: "lead", Orchestrates: []string{"eng"}}, sorted(everyone, orchestrator)},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := toolNames(t, c.self); !slices.Equal(got, c.want) {
				t.Errorf("A4 tools:\n got %v\nwant %v", got, c.want)
			}
		})
	}
}
