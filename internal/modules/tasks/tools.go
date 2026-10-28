package tasks

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/module"
)

//go:embed skills
var skills embed.FS

// The skills of the pipeline (rules TK22 and TK23).
const (
	TaskWorkSkill  = "hikari-task-work"
	ScopeWorkSkill = "hikari-scope-work"
)

// Skills are hikari-task-work for a partition agent, and hikari-scope-work
// for the orchestration agent.
func (Module) Skills(role string) []module.Skill {
	name := map[string]string{api.RolePartition: TaskWorkSkill, api.RoleOrchestration: ScopeWorkSkill}[role]
	if name == "" {
		return nil
	}
	files, err := fs.Sub(skills, "skills/"+role+"/"+name)
	if err != nil {
		return nil
	}
	return []module.Skill{{Name: name, Files: files}}
}

type stubArgs struct {
	Name string `json:"name" jsonschema:"a short name for the idea, such as Coupons at checkout"`
	Body string `json:"body,omitempty" jsonschema:"the idea in a few sentences: what and why, in plain words"`
}

type scopeArgs struct {
	ID         string            `json:"id" jsonschema:"the stub or draft scope to scope, by id"`
	Partitions []string          `json:"partitions" jsonschema:"the partition agents whose code the work changes"`
	Features   []string          `json:"features,omitempty" jsonschema:"the features it changes or creates, as ids of the partitions' feature maps or of your own; their health decides when it is done"`
	Needs      []string          `json:"needs,omitempty" jsonschema:"other scopes, by id, that must be done before this one starts"`
	Goal       string            `json:"goal" jsonschema:"what the work achieves, for the user, in two or three sentences"`
	Parts      map[string]string `json:"parts" jsonschema:"for each partition in partitions, what that partition changes, in terms of its features and interfaces, not code"`
	DoneWhen   string            `json:"done_when" jsonschema:"what shows that the work is done: the features whose health passes, and the flows that work"`
	Answers    string            `json:"answers,omitempty" jsonschema:"your questions and the user's answers, as a list"`
}

type queueArgs struct {
	Op       string `json:"op" jsonschema:"create, rename, remove, add, move, or out"`
	Queue    string `json:"queue,omitempty" jsonschema:"the queue, by id: for rename, remove, add, and move"`
	Name     string `json:"name,omitempty" jsonschema:"the queue's name: for create and rename"`
	Scope    string `json:"scope,omitempty" jsonschema:"the scope, by id: for add, move, and out"`
	Position int    `json:"position,omitempty" jsonschema:"the scope's place in the queue, from 1; leave it out for the end"`
}

type statusArgs struct {
	Task   string `json:"task" jsonschema:"the scope, by id"`
	Status string `json:"status" jsonschema:"for your part as a partition agent: started, testing, blocked, or done; for a scope as the orchestration agent: blocked, started (to clear the scope's block), or done"`
	Note   string `json:"note,omitempty" jsonschema:"why; required for blocked"`
}

type taskArgs struct {
	ID string `json:"id" jsonschema:"the task, by id"`
}

type noArgs struct{}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

const prefix = "/api/agent/" + Name

// Tools are the pipeline's tools for self (rules TK8, TK10, TK11, TK14,
// TK16, and TK21).
func (Module) Tools(s *mcp.Server, c *api.Client, self api.Self) {
	if self.Partition == nil && !self.Orchestration {
		return
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "plant_stub",
		Description: "Plant a stub in the task pipeline: an idea of work, in a few words, that the user and the orchestration agent scope later. Plant one for work you find outside what you were asked to do, such as a fault in code you cannot see. It starts no work.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a stubArgs) (*mcp.CallToolResult, any, error) {
		var v TaskView
		if err := c.Call(ctx, http.MethodPost, prefix+"/stub", StubRequest{Name: a.Name, Body: a.Body}, &v); err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("Planted the stub [[%s]]. It waits for the user.", v.ID)), nil, nil
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_task",
		Description: "Read a task of the pipeline: its properties, its sections, its parts with their statuses, and its log. A partition agent reads the scopes that name it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a taskArgs) (*mcp.CallToolResult, any, error) {
		var t TaskText
		if err := c.Call(ctx, http.MethodGet, prefix+"/tasks/"+url.PathEscape(strings.Trim(a.ID, "[] ")), nil, &t); err != nil {
			return nil, nil, err
		}
		return text(t.Text), nil, nil
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_tasks",
		Description: "List the task pipeline. A partition agent sees its scopes and their statuses; the orchestration agent sees the stubs, the draft scopes, and each queue with its scopes.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		var l List
		if err := c.Call(ctx, http.MethodGet, prefix+"/list", nil, &l); err != nil {
			return nil, nil, err
		}
		return text(formatList(l)), nil, nil
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "set_task_status",
		Description: "Report a status in the task pipeline. As a partition agent, set your part of a scope: started when you begin, testing when your suites run, blocked with a note the moment you cannot go on, and done when your suites pass; hikari grants done only when the health of the scope's features in your partition passes. As the orchestration agent, set a scope blocked with a note, started to clear its block, or done after you validate it; hikari grants done only when every part is done and the scope's orchestration features pass.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a statusArgs) (*mcp.CallToolResult, any, error) {
		var g Grant
		if err := c.Call(ctx, http.MethodPost, prefix+"/status", StatusRequest{Task: strings.Trim(a.Task, "[] "), Status: a.Status, Note: a.Note}, &g); err != nil {
			return nil, nil, err
		}
		if len(g.Refused) > 0 {
			return text(fmt.Sprintf("Done is not granted yet; the status stays %s, because %s. Fix them, run your suites again, and ask for done once they pass.", g.Status, strings.Join(g.Refused, ", "))), nil, nil
		}
		return text("The status is " + g.Status + "."), nil, nil
	})
	if !self.Orchestration {
		return
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "scope_task",
		Description: "Turn a stub, or a draft scope, into a scope of the task pipeline, once the user has answered your questions: the partitions it touches, the features it changes, what each partition changes, and what shows that it is done. The stub's words stay as its Request. A scope in a queue is the user's, and this tool refuses it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a scopeArgs) (*mcp.CallToolResult, any, error) {
		var v TaskView
		in := ScopeInput{ID: strings.Trim(a.ID, "[] "), Partitions: a.Partitions, Features: a.Features, Needs: a.Needs, Goal: a.Goal, Parts: a.Parts, DoneWhen: a.DoneWhen, Answers: a.Answers}
		if err := c.Call(ctx, http.MethodPost, prefix+"/scope", in, &v); err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("Scoped [[%s]] for %s. It is a draft until the user, or you at the user's request, puts it in a queue.", v.ID, strings.Join(v.Partitions, ", "))), nil, nil
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "change_queue",
		Description: "Change the queues of the task pipeline when the user asks you to: create a queue, rename one, remove an empty one, add a scope at a position, move a scope within its queue or to another, or take a scope out. A scope that has started stays where it is. Only the user starts or pauses a queue.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a queueArgs) (*mcp.CallToolResult, any, error) {
		op := QueueOp{Op: a.Op, Queue: a.Queue, Name: a.Name, Scope: strings.Trim(a.Scope, "[] "), Position: a.Position}
		if err := c.Call(ctx, http.MethodPost, prefix+"/queue", op, nil); err != nil {
			return nil, nil, err
		}
		return text("Done: " + a.Op + "."), nil, nil
	})
}

// formatList renders a list for an agent.
func formatList(l List) string {
	var b strings.Builder
	line := func(t TaskView) {
		fmt.Fprintf(&b, "- [[%s]] %s", t.ID, t.Name)
		if t.Status != "" {
			fmt.Fprintf(&b, ": %s", t.Status)
		}
		if len(t.Parts) > 0 {
			var ps []string
			for _, p := range t.Parts {
				ps = append(ps, p.Partition+" "+p.Status)
			}
			fmt.Fprintf(&b, " (%s)", strings.Join(ps, ", "))
		} else if len(t.Partitions) > 0 {
			fmt.Fprintf(&b, " (%s)", strings.Join(t.Partitions, ", "))
		}
		if t.Wait != "" {
			fmt.Fprintf(&b, "; waits: %s", t.Wait)
		}
		if t.Block != "" {
			fmt.Fprintf(&b, "; blocked: %s", t.Block)
		}
		b.WriteString("\n")
	}
	section := func(title string, ts []TaskView) {
		if len(ts) == 0 {
			return
		}
		fmt.Fprintf(&b, "%s:\n", title)
		for _, t := range ts {
			line(t)
		}
	}
	section("Your scopes", l.Scopes)
	section("Stubs", l.Stubs)
	section("Draft scopes", l.Drafts)
	for _, q := range l.Queues {
		fmt.Fprintf(&b, "Queue %s (%s), %s:\n", q.ID, q.Name, q.State)
		for _, t := range q.Scopes {
			line(t)
		}
	}
	if b.Len() == 0 {
		return "The pipeline holds nothing for you."
	}
	return b.String()
}
