package mcpserver

import (
	"context"
	"slices"
	"testing"
	"time"

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
	everyone           = []string{"check_inbox", "get_document", "get_vocabulary", "list_agents", "post", "search_wiki", "send_direct_message"}
	partitionAgentOnly = []string{"change_wiki", "check_release", "apply", "get_my_partition", "mark_map_current", "publish_release", "record_test_run", "set_partition_info"}
	orchestration      = []string{"change_public_wiki", "define_interface", "define_term", "get_partition", "list_partitions", "remove_entry"}
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
		"owner":        {api.Self{Name: "orders", Partition: &api.SelfPartition{}}, sorted(everyone, partitionAgentOnly)},
		"orchestrator": {api.Self{Name: "lead", Orchestration: true}, sorted(everyone, orchestration)},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := toolNames(t, c.self); !slices.Equal(got, c.want) {
				t.Errorf("A4 tools:\n got %v\nwant %v", got, c.want)
			}
		})
	}
}

// Every result of a tool renders as YAML.
func TestResultsRender(t *testing.T) {
	now := time.Now()
	for _, v := range []any{
		api.DocumentHit{Backlinks: []api.DocRef{{ID: "a"}}},
		api.WikiView{Documents: []api.WikiDoc{{ID: "a", Created: now}}},
	} {
		if _, _, err := yamlResult(v); err != nil {
			t.Errorf("%T: %v", v, err)
		}
	}
	if _, _, err := yamlResult(struct {
		X int `yaml:"x,bogus"`
	}{}); err == nil {
		t.Error("a value YAML cannot render is an error, not a panic")
	}
}
