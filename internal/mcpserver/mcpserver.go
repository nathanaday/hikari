// Package mcpserver exposes the server's agent API to Claude Code as MCP tools.
package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nathanaday/hikari/internal/api"
)

type postArgs struct {
	Body string `json:"body" jsonschema:"the message text; write @name to wake a specific participant"`
}

type directArgs struct {
	To   string `json:"to" jsonschema:"name of an agent, or user"`
	Body string `json:"body" jsonschema:"the message text"`
}

type noArgs struct{}

// New returns the MCP server of the agent described by self. Every agent
// gets the messaging tools and get_vocabulary. Partition agents also get the map
// tools, and the orchestration agent the partition and vocabulary tools.
func New(c *api.Client, self api.Self) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "hikari", Version: "0.3.0"}, nil)
	addVocabularyReader(s, c)
	addWikiReaders(s, c)
	if self.Partition != nil {
		addPartitionAgentTools(s, c)
	}
	if self.Orchestration {
		addOrchestrationTools(s, c)
	}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "post",
		Description: "Post a message to the room. Every agent and the user receive it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a postArgs) (*mcp.CallToolResult, any, error) {
		m, err := c.Send(ctx, api.SendRequest{Body: a.Body})
		if err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("Posted message %d to the room.%s", m.ID, wokeNote(m))), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "send_direct_message",
		Description: "Send a private message to one agent or the user. Only that one receives it, and it starts a turn for an agent.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a directArgs) (*mcp.CallToolResult, any, error) {
		m, err := c.Send(ctx, api.SendRequest{To: a.To, Body: a.Body})
		if err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("Sent direct message %d to %s.", m.ID, m.To)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_agents",
		Description: "List everyone in the room: each agent and the user, with their role and description.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		self, err := c.Self(ctx)
		if err != nil {
			return nil, nil, err
		}
		return text(DescribeRoom(self)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "check_inbox",
		Description: "Read messages that arrived during your current turn. It does not wait.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		msgs, err := c.Inbox(ctx, 0)
		if err != nil {
			return nil, nil, err
		}
		if len(msgs) == 0 {
			return text("No new messages."), nil, nil
		}
		return text(FormatMessages(msgs)), nil, nil
	})

	return s
}

// DescribeRoom lists everyone in the room with their role.
func DescribeRoom(self api.Self) string {
	var b strings.Builder
	for _, p := range self.Participants {
		name := p.Name
		if name == self.Name {
			name += " (you)"
		}
		fmt.Fprintf(&b, "- %s, %s", name, p.Role)
		if p.Description != "" {
			fmt.Fprintf(&b, ": %s", p.Description)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// FormatMessages renders messages the same way for a new turn and for
// check_inbox.
func FormatMessages(msgs []api.Message) string {
	var b strings.Builder
	for i, m := range msgs {
		if i > 0 {
			b.WriteString("\n")
		}
		channel := "room"
		if m.To != "" {
			channel = "direct"
		}
		fmt.Fprintf(&b, "<message id=\"%d\" from=%q channel=%q time=%q>\n%s\n</message>\n",
			m.ID, m.From, channel, m.Time.Format("2006-01-02T15:04:05Z"), m.Body)
	}
	return b.String()
}

func wokeNote(m api.Message) string {
	if len(m.Woke) == 0 {
		return " It started a turn for nobody; the others read it in their next turn."
	}
	return " It started a turn for " + strings.Join(m.Woke, ", ") + "."
}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}
