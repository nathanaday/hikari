// Package mcpserver exposes the server's node API to Claude Code as MCP tools.
package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nathanaday/hikari/internal/api"
)

type postArgs struct {
	Hub  string `json:"hub" jsonschema:"name of the hub to post to, without the #"`
	Body string `json:"body" jsonschema:"the message text; write @name to wake a specific participant"`
}

type directArgs struct {
	To   string `json:"to" jsonschema:"name of a node that shares a hub with you"`
	Body string `json:"body" jsonschema:"the message text"`
}

type noArgs struct{}

// New returns the MCP server of the agent described by self. Every agent
// gets the messaging tools and get_vocabulary. Owners also get the map
// tools, and orchestrators the system and vocabulary tools.
func New(c *api.Client, self api.Self) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "hikari", Version: "0.3.0"}, nil)
	addVocabularyReader(s, c)
	addWikiReaders(s, c)
	if self.System != nil {
		addOwnerTools(s, c)
	}
	if self.Tests != "" {
		addTesterTools(s, c)
		addEnvironmentTools(s, c)
	}
	if self.Tests != "" || len(self.Testers) > 0 {
		addTestPlanReader(s, c)
	}
	if len(self.Orchestrates) > 0 {
		addOrchestratorTools(s, c)
	}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "post_to_hub",
		Description: "Post a message to a hub. Every participant of the hub receives it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a postArgs) (*mcp.CallToolResult, any, error) {
		m, err := c.Send(ctx, api.SendRequest{Hub: strings.TrimPrefix(a.Hub, "#"), Body: a.Body})
		if err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("Posted message %d to #%s.%s", m.ID, m.Hub, wokeNote(m))), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "send_direct_message",
		Description: "Send a private message to one node that shares a hub with you. Only that node receives it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a directArgs) (*mcp.CallToolResult, any, error) {
		m, err := c.Send(ctx, api.SendRequest{To: a.To, Body: a.Body})
		if err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("Sent direct message %d to %s.", m.ID, m.To)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_hubs",
		Description: "List your hubs, your role in each, and their participants.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		self, err := c.Self(ctx)
		if err != nil {
			return nil, nil, err
		}
		return text(DescribeHubs(self)), nil, nil
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

// DescribeHubs lists the hubs of self with the role of each participant.
func DescribeHubs(self api.Self) string {
	var b strings.Builder
	for i, h := range self.Hubs {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "#%s", h.Name)
		if h.Description != "" {
			fmt.Fprintf(&b, ": %s", h.Description)
		}
		b.WriteString("\n")
		for _, p := range h.Participants {
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
		channel := "direct"
		if m.Hub != "" {
			channel = "#" + m.Hub
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
