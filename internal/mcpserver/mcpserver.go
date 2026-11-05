// Package mcpserver exposes the server's agent API to Claude Code as MCP tools.
package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/modules"
	"github.com/nathanaday/hikari/internal/version"
)

type postArgs struct {
	Body string `json:"body" jsonschema:"the message text; write @name to wake a specific participant"`
}

type directArgs struct {
	To   string `json:"to" jsonschema:"name of an agent, or user"`
	Body string `json:"body" jsonschema:"the message text"`
}

type askArgs struct {
	Question string   `json:"question" jsonschema:"the question, in one or two sentences, with what each choice means"`
	Options  []string `json:"options" jsonschema:"two to six choices, each a short phrase; put the one you recommend first"`
	Task     string   `json:"task,omitempty" jsonschema:"the task the question concerns, by id, when it concerns one"`
}

type proposeArgs struct {
	ID     string `json:"id" jsonschema:"the rule's id: lowercase letters, digits, and dashes, such as no-silent-retries"`
	Name   string `json:"name" jsonschema:"two to five words that name the rule"`
	Rule   string `json:"rule" jsonschema:"the rule as one sentence that is true or false at any moment, under 200 characters"`
	Reason string `json:"reason" jsonschema:"what happened that taught it, and what the rule prevents: the block, the refusal, the review finding, or the question"`
}

type noArgs struct{}

// New returns the MCP server of the agent described by self. Every agent
// gets the messaging tools and get_vocabulary. Partition agents also get the map
// tools, and the orchestration agent the partition and vocabulary tools.
func New(c *api.Client, self api.Self) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "hikari", Version: version.Version}, nil)
	addVocabularyReader(s, c)
	addWikiReaders(s, c)
	addReleaseReaders(s, c)
	if self.Partition != nil || self.Orchestration {
		addWikiWriter(s, c)
	}
	if self.Partition != nil {
		addPartitionAgentTools(s, c)
	}
	if self.Orchestration {
		addOrchestrationTools(s, c)
	}
	for _, m := range modules.Named(self.Modules) {
		m.Tools(s, c, self)
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
		Name:        "ask_user",
		Description: "Ask the user one question with numbered choices. The UI shows it as a card that the user answers with one click, and the answer reaches you as a direct message, \"<n>. <choice>\" with any words the user adds. Use it when a choice between two or three readings would waste work either way; state your own reading as the first choice. It does not wait: end your turn after asking.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a askArgs) (*mcp.CallToolResult, any, error) {
		m, err := c.Send(ctx, api.SendRequest{To: "user", Decision: &api.Decision{Question: a.Question, Options: a.Options, Task: strings.Trim(a.Task, "[] ")}})
		if err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("Asked the user (message %d). The answer arrives as a direct message; end your turn.", m.ID)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "propose_rule",
		Description: "Propose a rule for every agent, from a lesson of your work: a block that a rule would have prevented, a done that was refused, a review finding that will recur, a question the user answered that every agent should know. The user sees it as a card; Accept writes it into the public wiki as a rule, which every agent reads from its next turn, and the answer reaches you as a direct message. Propose a rule that is one sentence and true or false at any moment, with its reason; do not propose what the public wiki already says.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a proposeArgs) (*mcp.CallToolResult, any, error) {
		d := api.ProposeRule(self.Name, api.Proposal{ID: a.ID, Name: a.Name, Rule: a.Rule, Reason: a.Reason})
		m, err := c.Send(ctx, api.SendRequest{To: "user", Decision: &d})
		if err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("Proposed the rule %s to the user (message %d). The answer arrives as a direct message; go on with your work.", a.ID, m.ID)), nil, nil
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
