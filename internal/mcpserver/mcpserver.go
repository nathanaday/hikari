// Package mcpserver exposes the hub's node API to Claude Code as MCP tools.
package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nathanaday/hikari/internal/api"
)

type sendArgs struct {
	To   string `json:"to" jsonschema:"name of the neighbor to send to"`
	Body string `json:"body" jsonschema:"the message text"`
}

type noArgs struct{}

func New(c *api.Client) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "hikari", Version: "0.1.0"}, nil)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "send_message",
		Description: "Send a message to one neighbor. This is the only way to give information to another node or to the user.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a sendArgs) (*mcp.CallToolResult, any, error) {
		m, err := c.Send(ctx, a.To, a.Body)
		if err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("Sent message %d to %s.", m.ID, m.To)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_neighbors",
		Description: "List the nodes you can send messages to.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		self, err := c.Self(ctx)
		if err != nil {
			return nil, nil, err
		}
		var b strings.Builder
		fmt.Fprintf(&b, "You are %s. Your neighbors:\n", self.Name)
		for _, p := range self.Neighbors {
			fmt.Fprintf(&b, "- %s: %s\n", p.Name, p.Description)
		}
		return text(b.String()), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "check_inbox",
		Description: "Read messages that arrived during your current turn. It does not wait. Messages that arrive while you are idle start a new turn instead.",
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

// FormatMessages renders messages the same way for a new turn and for
// check_inbox.
func FormatMessages(msgs []api.Message) string {
	var b strings.Builder
	for i, m := range msgs {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "<message id=%q from=%q time=%q>\n%s\n</message>\n",
			fmt.Sprint(m.ID), m.From, m.Time.Format("2006-01-02T15:04:05Z"), m.Body)
	}
	return b.String()
}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}
