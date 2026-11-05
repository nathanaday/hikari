package tasks

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nathanaday/hikari/internal/hostfile"
	"github.com/nathanaday/hikari/internal/module"
	"github.com/nathanaday/hikari/internal/topology"
)

// Commands are hikari issue, hikari task, and hikari queue (rules TK25 and
// IS9). With hikari running for the topology file, they go through its
// UI's API; with it stopped, the ones that only change your documents
// write the files, and hikari commits them as yours at its next start
// (decision 8).
func (Module) Commands() map[string]module.Command {
	return map[string]module.Command{"issue": issueCommand, "task": taskCommand, "queue": queueCommand}
}

const issueUsage = `Usage:
  hikari issue list [-file topology.yaml]                 list the issues of every tracker, and the local ones
  hikari issue new [-file topology.yaml] [-repo <repo>] [-local] <title> [body]
                                                          open an issue on a tracker, or in the vault
  hikari issue work [-file topology.yaml] <issue>         hand an issue to the agents: the work queue starts it
  hikari issue sync [-file topology.yaml]                 read the trackers again
An issue is <repo>#<number>, or the id of a local issue.
`

const taskUsage = `Usage:
  hikari task list [-file topology.yaml]                  list the tasks and the queues
  hikari task status [-file topology.yaml] [-part <partition>] <id> <status> [note]
                                                          set a scope's status, or one part's, as you
`

const queueUsage = `Usage:
  hikari queue new <name>                   make a queue, paused
  hikari queue rename <queue> <name>        rename a queue
  hikari queue remove <queue>               remove an empty queue
  hikari queue add <queue> <issue> [n]      put an issue in a queue, at place n or at the end
  hikari queue move <queue> <issue> [n]     move an issue within its queue, or to another
  hikari queue out <issue>                  take an issue out of its queue
  hikari queue start <queue>                start a queue
  hikari queue pause <queue>                pause a queue: it finishes the scope in progress
  hikari queue list                         list the queues
Each takes -file topology.yaml before its arguments. An issue is <repo>#<number>, or a task id.
`

// target is the topology file of a command, and how to reach its pipeline.
type target struct {
	file, shown string
}

type cliFlags struct {
	fs    *flag.FlagSet
	t     *target
	part  *string
	repo  *string
	local *bool
}

func flags(name, usage string, args []string) cliFlags {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	file := fs.String("file", "topology.yaml", "the topology file of the project")
	part := fs.String("part", "", "the partition whose part to set")
	repo := fs.String("repo", "", "the repository whose tracker gets the issue")
	local := fs.Bool("local", false, "open the issue in the vault, not on a tracker")
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	fs.Parse(args)
	shown := *file
	if rel, err := filepath.Rel(".", *file); err == nil {
		shown = rel
	}
	return cliFlags{fs: fs, t: &target{file: *file, shown: shown}, part: part, repo: repo, local: local}
}

func (t *target) live() bool {
	_, ok := hostfile.Live(t.file)
	return ok
}

func (t *target) call(ctx context.Context, method, path string, in, out any) error {
	return hostfile.Call(ctx, t.file, t.shown, method, "/api/ui/"+Name+path, in, out)
}

// offline opens the store of a stopped project.
func (t *target) offline(ctx context.Context) (*Store, *topology.Topology, error) {
	topo, err := topology.Load(ctx, t.file)
	if err != nil {
		return nil, nil, err
	}
	if !topo.HasModule(Name) {
		return nil, nil, fmt.Errorf("%s does not turn on the tasks module; add tasks to x-hikari.modules", t.shown)
	}
	st, err := Open(filepath.Join(topo.Vault, Dir), Check{Partitions: topo.PartitionAgents(), Orchestration: topo.Orchestration}, time.Now)
	return st, topo, err
}

func issueCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, issueUsage)
		os.Exit(2)
	}
	f := flags("issue "+args[0], issueUsage, args[1:])
	t, rest := f.t, f.fs.Args()
	switch args[0] {
	case "list":
		v, err := t.view(ctx)
		if err != nil {
			return err
		}
		fmt.Print(formatIssues(v))
		return nil
	case "new":
		if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
			fmt.Fprint(os.Stderr, issueUsage)
			os.Exit(2)
		}
		req := IssueRequest{Repo: *f.repo, Title: rest[0], Body: strings.Join(rest[1:], " "), Local: *f.local}
		var v IssueView
		if t.live() {
			if err := t.call(ctx, http.MethodPost, "/issues", req, &v); err != nil {
				return err
			}
		} else {
			if req.Repo != "" || !req.Local {
				fmt.Fprintf(os.Stderr, "hikari is not running for %s, so the issue goes to the vault; start hikari up to open it on a tracker\n", t.shown)
			}
			st, _, err := t.offline(ctx)
			if err != nil {
				return err
			}
			task, _, err := st.PlantStub(req.Title, req.Body, User)
			if err != nil {
				return err
			}
			v = IssueView{Ref: task.ID, Local: true}
		}
		if v.Local {
			fmt.Printf("Opened the local issue %s in %s.\n", v.Ref, filepath.Join("vault", Dir))
		} else {
			fmt.Printf("Opened the issue %s: %s\n", v.Ref, v.URL)
		}
		return nil
	case "work":
		if len(rest) != 1 {
			fmt.Fprint(os.Stderr, issueUsage)
			os.Exit(2)
		}
		if !t.live() {
			return fmt.Errorf("%w: the agents work only while hikari runs; start it in your own terminal with: hikari up %s", hostfile.ErrNotRunning, t.shown)
		}
		var v TaskView
		if err := t.call(ctx, http.MethodPost, "/work", WorkRequest{Issue: rest[0]}, &v); err != nil {
			return err
		}
		fmt.Printf("[[%s]] is at the front of the queue %s, which is running.\n", v.ID, v.Queue)
		return nil
	case "sync":
		if !t.live() {
			return hostfile.NotRunning(t.file, t.shown)
		}
		var v View
		if err := t.call(ctx, http.MethodPost, "/sync", nil, &v); err != nil {
			return err
		}
		for _, tr := range v.Trackers {
			if tr.Error != "" {
				fmt.Printf("%s: %s\n", tr.Repo, tr.Error)
			} else {
				fmt.Printf("%s: synced\n", tr.Repo)
			}
		}
		if len(v.Trackers) == 0 {
			fmt.Println("The project has no issue tracker: no repository's origin is on a forge that hikari knows.")
		}
		return nil
	}
	fmt.Fprint(os.Stderr, issueUsage)
	os.Exit(2)
	return nil
}

func taskCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, taskUsage)
		os.Exit(2)
	}
	f := flags("task "+args[0], taskUsage, args[1:])
	t, rest := f.t, f.fs.Args()
	switch args[0] {
	case "list":
		v, err := t.view(ctx)
		if err != nil {
			return err
		}
		fmt.Print(formatView(v))
		return nil
	case "status":
		if len(rest) < 2 {
			fmt.Fprint(os.Stderr, taskUsage)
			os.Exit(2)
		}
		var g Grant
		req := StatusRequest{Task: rest[0], Part: *f.part, Status: rest[1], Note: strings.Join(rest[2:], " ")}
		if err := t.call(ctx, http.MethodPost, "/status", req, &g); err != nil {
			return err
		}
		fmt.Printf("[[%s]] is %s.\n", strings.Trim(rest[0], "[]"), g.Status)
		return nil
	}
	fmt.Fprint(os.Stderr, taskUsage)
	os.Exit(2)
	return nil
}

func (t *target) view(ctx context.Context) (View, error) {
	var v View
	if t.live() {
		err := t.call(ctx, http.MethodGet, "/view", nil, &v)
		return v, err
	}
	st, topo, err := t.offline(ctx)
	if err != nil {
		return v, err
	}
	v = st.View(topo.Orchestration, topo.PartitionAgents())
	for _, task := range v.Tasks {
		if task.Issue == "" {
			tv := task
			v.Issues = append(v.Issues, IssueView{Ref: task.ID, Title: task.Name, State: "open", Labels: []string{}, Author: task.Author, Created: task.Created, Updated: task.Updated, Local: true, Task: &tv})
		}
	}
	return v, nil
}

func queueCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, queueUsage)
		os.Exit(2)
	}
	f := flags("queue "+args[0], queueUsage, args[1:])
	t, rest := f.t, f.fs.Args()
	need := func(n int) {
		if len(rest) < n {
			fmt.Fprint(os.Stderr, queueUsage)
			os.Exit(2)
		}
	}
	position := func(i int) int {
		if len(rest) <= i {
			return 0
		}
		n, err := strconv.Atoi(rest[i])
		if err != nil || n < 1 {
			fmt.Fprintf(os.Stderr, "the place in the queue is a number from 1, not %q\n", rest[i])
			os.Exit(2)
		}
		return n
	}
	// ref names the issue or the task of an argument.
	ref := func(arg string) QueueOp {
		arg = strings.Trim(arg, "[]")
		if _, _, ok := ParseIssueRef(arg); ok {
			return QueueOp{Issue: arg}
		}
		return QueueOp{Scope: arg}
	}
	var op QueueOp
	switch args[0] {
	case "list":
		v, err := t.view(ctx)
		if err != nil {
			return err
		}
		fmt.Print(formatView(View{Queues: v.Queues}))
		return nil
	case "start", "pause":
		need(1)
		state := map[string]string{"start": Running, "pause": Paused}[args[0]]
		if !t.live() {
			return fmt.Errorf("%w: a queue runs only while hikari runs; start it in your own terminal with: hikari up %s", hostfile.ErrNotRunning, t.shown)
		}
		if err := t.call(ctx, http.MethodPost, "/state", StateRequest{Queue: rest[0], State: state}, nil); err != nil {
			return err
		}
		fmt.Printf("The queue %s is %s.\n", rest[0], state)
		return nil
	case "new":
		need(1)
		op = QueueOp{Op: "create", Name: strings.Join(rest, " ")}
	case "rename":
		need(2)
		op = QueueOp{Op: "rename", Queue: rest[0], Name: strings.Join(rest[1:], " ")}
	case "remove":
		need(1)
		op = QueueOp{Op: "remove", Queue: rest[0]}
	case "add", "move":
		need(2)
		op = ref(rest[1])
		op.Op, op.Queue, op.Position = args[0], rest[0], position(2)
	case "out":
		need(1)
		op = ref(rest[0])
		op.Op = "out"
	default:
		fmt.Fprint(os.Stderr, queueUsage)
		os.Exit(2)
	}
	if t.live() {
		if err := t.call(ctx, http.MethodPost, "/queue", op, nil); err != nil {
			return err
		}
	} else {
		if op.Issue != "" {
			return fmt.Errorf("%w: a tracker issue enters a queue only while hikari runs; start it with: hikari up %s", hostfile.ErrNotRunning, t.shown)
		}
		st, _, err := t.offline(ctx)
		if err != nil {
			return err
		}
		if _, err := st.ChangeQueue(op); err != nil {
			return err
		}
	}
	fmt.Printf("Done: %s.\n", args[0])
	return nil
}

// formatIssues renders the issues for a terminal.
func formatIssues(v View) string {
	var b strings.Builder
	for _, tr := range v.Trackers {
		fmt.Fprintf(&b, "Tracker %s: %s", tr.Repo, tr.Kind)
		if tr.URL != "" {
			b.WriteString(" " + tr.URL)
		}
		if tr.Error != "" {
			b.WriteString(" (" + tr.Error + ")")
		}
		b.WriteString("\n")
	}
	for _, is := range v.Issues {
		fmt.Fprintf(&b, "  %-14s %s", is.Ref, is.Title)
		if is.State != "open" {
			fmt.Fprintf(&b, "  %s", is.State)
		}
		if is.Task != nil && is.Task.Queue != "" {
			fmt.Fprintf(&b, "  in %s, %s", is.Task.Queue, orDefault(is.Task.Status, Queued))
		} else if is.Task != nil && is.Task.Stage == StageScope {
			b.WriteString("  scoped")
		}
		if len(is.Labels) > 0 {
			fmt.Fprintf(&b, "  [%s]", strings.Join(is.Labels, ", "))
		}
		b.WriteString("\n")
	}
	if len(v.Issues) == 0 {
		b.WriteString("No issues.\n")
	}
	return b.String()
}

// formatView renders the pipeline for a terminal.
func formatView(v View) string {
	var b strings.Builder
	line := func(t TaskView) {
		fmt.Fprintf(&b, "  [[%s]] %s", t.ID, t.Name)
		if t.Issue != "" {
			fmt.Fprintf(&b, "  (%s)", t.Issue)
		}
		if t.Status != "" {
			fmt.Fprintf(&b, "  %s", t.Status)
		}
		if len(t.Parts) > 0 {
			var ps []string
			for _, p := range t.Parts {
				ps = append(ps, p.Partition+" "+p.Status)
			}
			fmt.Fprintf(&b, "  (%s)", strings.Join(ps, ", "))
		} else if len(t.Partitions) > 0 {
			fmt.Fprintf(&b, "  (%s)", strings.Join(t.Partitions, ", "))
		}
		if t.Wait != "" {
			fmt.Fprintf(&b, "  waits: %s", t.Wait)
		}
		if t.Block != "" {
			fmt.Fprintf(&b, "  blocked: %s", t.Block)
		}
		b.WriteString("\n")
	}
	var loose []TaskView
	for _, t := range v.Tasks {
		if t.Queue == "" {
			loose = append(loose, t)
		}
	}
	if len(loose) > 0 {
		b.WriteString("Tasks in no queue\n")
		for _, t := range loose {
			line(t)
		}
	}
	for _, q := range v.Queues {
		fmt.Fprintf(&b, "Queue %s: %s, %s\n", q.ID, q.Name, q.State)
		for _, t := range q.Scopes {
			line(t)
		}
	}
	for _, p := range v.Problems {
		fmt.Fprintf(&b, "Problem in %s: %s\n", p.File, p.Reason)
	}
	if b.Len() == 0 {
		return "The pipeline is empty.\n"
	}
	return b.String()
}
