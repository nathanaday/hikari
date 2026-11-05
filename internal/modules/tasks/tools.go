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
	"github.com/nathanaday/hikari/internal/forge"
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

type issueArgs struct {
	Title string `json:"title" jsonschema:"a short title, such as Coupons at checkout"`
	Body  string `json:"body,omitempty" jsonschema:"what and why, in plain words; for a fault, what you saw and where"`
	Repo  string `json:"repo,omitempty" jsonschema:"the repository whose tracker gets the issue; leave it out for the project's first tracker, or for the vault when the project has none"`
}

type issueRefArgs struct {
	Issue string `json:"issue" jsonschema:"the issue, as <repo>#<number>, or the id of a local issue"`
}

type commentArgs struct {
	Issue string `json:"issue" jsonschema:"the issue, as <repo>#<number>"`
	Body  string `json:"body" jsonschema:"the comment, in markdown"`
}

type scopeArgs struct {
	ID         string            `json:"id" jsonschema:"the task to scope, by id, or its issue as <repo>#<number>"`
	Partitions []string          `json:"partitions" jsonschema:"the partition agents whose code the work changes"`
	Features   []string          `json:"features,omitempty" jsonschema:"the features it changes or creates, as ids of the partitions' feature maps or of your own; their health decides when it is done"`
	Needs      []string          `json:"needs,omitempty" jsonschema:"other scopes, by id, that must be done before this one starts"`
	Goal       string            `json:"goal" jsonschema:"what the work achieves, for the user, in two or three sentences"`
	Parts      map[string]string `json:"parts" jsonschema:"for each partition in partitions, what that partition changes, in terms of its features and interfaces, not code"`
	DoneWhen   string            `json:"done_when" jsonschema:"what shows that the work is done: the features whose health passes, and the flows that work"`
	Answers    string            `json:"answers,omitempty" jsonschema:"the questions the issue left open, each with the user's answer or the assumption you made"`
}

type queueArgs struct {
	Op       string `json:"op" jsonschema:"create, rename, remove, add, move, or out"`
	Queue    string `json:"queue,omitempty" jsonschema:"the queue, by id: for rename, remove, add, and move"`
	Name     string `json:"name,omitempty" jsonschema:"the queue's name: for create and rename"`
	Scope    string `json:"scope,omitempty" jsonschema:"the task, by id: for add, move, and out"`
	Issue    string `json:"issue,omitempty" jsonschema:"the issue, as <repo>#<number>, in place of scope"`
	Position int    `json:"position,omitempty" jsonschema:"the place in the queue, from 1; leave it out for the end"`
}

type statusArgs struct {
	Task   string `json:"task" jsonschema:"the scope, by id"`
	Status string `json:"status" jsonschema:"for your part as a partition agent: started, testing, blocked, or done; for a scope as the orchestration agent: blocked, started (to clear the scope's block), or done"`
	Note   string `json:"note,omitempty" jsonschema:"why; required for blocked"`
	Review string `json:"review,omitempty" jsonschema:"with done, as a partition agent: the verdict of the hikari-review-work review in one line, who reviewed, the counts by severity, what you fixed, and what you left; required when your prompt says your work is reviewed"`
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
// TK16, TK21, and IS7).
func (Module) Tools(s *mcp.Server, c *api.Client, self api.Self) {
	if self.Partition == nil && !self.Orchestration {
		return
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_issue",
		Description: "Open an issue: a piece of work, in a few words, that the user triages later. It goes to the issue tracker of the project's repository, or to the vault when the project has none. Open one for work you find outside what you were asked to do, such as a fault in code you cannot see. It starts no work.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a issueArgs) (*mcp.CallToolResult, any, error) {
		var v IssueView
		if err := c.Call(ctx, http.MethodPost, prefix+"/issues", IssueRequest{Repo: a.Repo, Title: a.Title, Body: a.Body}, &v); err != nil {
			return nil, nil, err
		}
		if v.Local {
			return text(fmt.Sprintf("Opened the local issue %s in the vault. It waits for the user.", v.Ref)), nil, nil
		}
		return text(fmt.Sprintf("Opened the issue %s: %s. It waits for the user.", v.Ref, v.URL)), nil, nil
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
		Description: "List the task pipeline. A partition agent sees its scopes and their statuses; the orchestration agent sees every issue, with its task when it has one, and each queue with its scopes.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		var l List
		if err := c.Call(ctx, http.MethodGet, prefix+"/list", nil, &l); err != nil {
			return nil, nil, err
		}
		return text(formatList(l)), nil, nil
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "set_task_status",
		Description: "Report a status in the task pipeline. As a partition agent, set your part of a scope: started when you begin, testing when your suites run, blocked with a note the moment you cannot go on, and done when your suites pass; hikari grants done only when the health of the scope's features in your partition passes, and, when your work is reviewed, only with the review's verdict. As the orchestration agent, set a scope blocked with a note, started to clear its block, or done after you validate it; hikari grants done only when every part is done and the scope's orchestration features pass.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a statusArgs) (*mcp.CallToolResult, any, error) {
		var g Grant
		if err := c.Call(ctx, http.MethodPost, prefix+"/status", StatusRequest{Task: strings.Trim(a.Task, "[] "), Status: a.Status, Note: a.Note, Review: a.Review}, &g); err != nil {
			return nil, nil, err
		}
		if len(g.Refused) > 0 {
			return text(fmt.Sprintf("Done is not granted yet; the status stays %s, because: %s. Settle each, and ask for done again.", g.Status, strings.Join(g.Refused, "; "))), nil, nil
		}
		return text("The status is " + g.Status + "."), nil, nil
	})
	if !self.Orchestration {
		return
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_issue",
		Description: "Read an issue with its comments: the user's request as the tracker holds it. Read it before you scope it, and read its comments for the user's answers. Pass <repo>#<number>, or the id of a local issue.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a issueRefArgs) (*mcp.CallToolResult, any, error) {
		var d IssueDetail
		if err := c.Call(ctx, http.MethodGet, prefix+"/issues/"+url.PathEscape(strings.TrimSpace(a.Issue)), nil, &d); err != nil {
			return nil, nil, err
		}
		return text(formatIssue(d)), nil, nil
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "comment_issue",
		Description: "Comment on a tracker issue, as yourself: a plan, a question for the user, or what you found. The comment is public on the tracker. Keep it short and in markdown.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a commentArgs) (*mcp.CallToolResult, any, error) {
		var out forge.Comment
		if err := c.Call(ctx, http.MethodPost, prefix+"/issues/"+url.PathEscape(strings.TrimSpace(a.Issue))+"/comments", CommentRequest{Body: a.Body}, &out); err != nil {
			return nil, nil, err
		}
		return text("Commented on " + a.Issue + "."), nil, nil
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "scope_task",
		Description: "Write the scope of an issue's task: the partitions it touches, the features it changes, what each partition changes, and what shows that it is done. Call it when a queue starts an issue that is not scoped yet, or when the user asks you to scope one. The issue's words stay as the task's Request. A scope that is already in a queue is the user's, and this tool refuses it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a scopeArgs) (*mcp.CallToolResult, any, error) {
		var v TaskView
		in := ScopeInput{ID: strings.Trim(a.ID, "[] "), Partitions: a.Partitions, Features: a.Features, Needs: a.Needs, Goal: a.Goal, Parts: a.Parts, DoneWhen: a.DoneWhen, Answers: a.Answers}
		if err := c.Call(ctx, http.MethodPost, prefix+"/scope", in, &v); err != nil {
			return nil, nil, err
		}
		if active(v.Status) {
			return text(fmt.Sprintf("Scoped [[%s]] for %s. It is started: carry it out now.", v.ID, strings.Join(v.Partitions, ", "))), nil, nil
		}
		return text(fmt.Sprintf("Scoped [[%s]] for %s. It waits in no queue until the user, or you at the user's request, puts it in one.", v.ID, strings.Join(v.Partitions, ", "))), nil, nil
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "change_queue",
		Description: "Change the queues of the task pipeline when the user asks you to: create a queue, rename one, remove an empty one, add an issue or a task at a position, move one within its queue or to another, or take one out. A task that has started stays where it is. Only the user starts or pauses a queue.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a queueArgs) (*mcp.CallToolResult, any, error) {
		op := QueueOp{Op: a.Op, Queue: a.Queue, Name: a.Name, Scope: strings.Trim(a.Scope, "[] "), Issue: strings.TrimSpace(a.Issue), Position: a.Position}
		if err := c.Call(ctx, http.MethodPost, prefix+"/queue", op, nil); err != nil {
			return nil, nil, err
		}
		return text("Done: " + a.Op + "."), nil, nil
	})
}

// formatIssue renders an issue with its comments for an agent.
func formatIssue(d IssueDetail) string {
	var b strings.Builder
	is := d.Issue
	fmt.Fprintf(&b, "# %s: %s\n\n", is.Ref, is.Title)
	fmt.Fprintf(&b, "State: %s", is.State)
	if is.Author != "" {
		fmt.Fprintf(&b, " · by %s", is.Author)
	}
	if len(is.Labels) > 0 {
		fmt.Fprintf(&b, " · labels: %s", strings.Join(is.Labels, ", "))
	}
	if is.URL != "" {
		fmt.Fprintf(&b, " · %s", is.URL)
	}
	if is.Task != nil {
		fmt.Fprintf(&b, "\nTask: [[%s]]", is.Task.ID)
		if is.Task.Status != "" {
			fmt.Fprintf(&b, ", %s", is.Task.Status)
		}
		if is.Task.Stage == StageStub {
			b.WriteString(", not scoped yet")
		}
	}
	b.WriteString("\n\n")
	if body := strings.TrimSpace(is.Body); body != "" {
		b.WriteString(body + "\n\n")
	} else {
		b.WriteString("_No body._\n\n")
	}
	if len(d.Comments) > 0 {
		b.WriteString("## Comments\n\n")
		for _, c := range d.Comments {
			fmt.Fprintf(&b, "**%s** (%s):\n\n%s\n\n", orDefault(c.Author, "someone"), c.Created.UTC().Format("2006-01-02 15:04"), strings.TrimSpace(c.Body))
		}
	}
	if d.Task != nil && d.Task.Task.Stage == StageScope {
		b.WriteString("## The scope\n\n" + strings.TrimSpace(d.Task.Text) + "\n")
	}
	return strings.TrimSpace(b.String())
}

// formatList renders a list for an agent.
func formatList(l List) string {
	var b strings.Builder
	line := func(t TaskView) {
		fmt.Fprintf(&b, "- [[%s]] %s", t.ID, t.Name)
		if t.Issue != "" {
			fmt.Fprintf(&b, " (issue %s)", t.Issue)
		}
		if t.Status != "" {
			fmt.Fprintf(&b, ": %s", t.Status)
		}
		if t.Stage == StageStub && t.Status != "" {
			b.WriteString(", not scoped yet")
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
	if len(l.Scopes) > 0 {
		b.WriteString("Your scopes:\n")
		for _, t := range l.Scopes {
			line(t)
		}
	}
	if len(l.Issues) > 0 {
		b.WriteString("Issues:\n")
		for _, is := range l.Issues {
			fmt.Fprintf(&b, "- %s %s: %s", is.Ref, is.Title, is.State)
			if is.Task != nil {
				switch {
				case is.Task.Queue != "":
					fmt.Fprintf(&b, "; in the queue %s, %s", is.Task.Queue, orDefault(is.Task.Status, Queued))
				case is.Task.Stage == StageScope:
					b.WriteString("; scoped, in no queue")
				}
			}
			if len(is.Labels) > 0 {
				fmt.Fprintf(&b, " [%s]", strings.Join(is.Labels, ", "))
			}
			b.WriteString("\n")
		}
	}
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
