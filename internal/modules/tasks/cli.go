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

// Commands are hikari task and hikari queue (rule TK25). With hikari
// running for the topology file, they go through its UI's API; with it
// stopped, the ones that only change your documents write the files, and
// hikari commits them as yours at its next start (decision 8).
func (Module) Commands() map[string]module.Command {
	return map[string]module.Command{"task": taskCommand, "queue": queueCommand}
}

const taskUsage = `Usage:
  hikari task stub [-file topology.yaml] <words>          plant a stub; its first line is its name
  hikari task list [-file topology.yaml]                  list the stubs, the drafts, and the queues
  hikari task status [-file topology.yaml] [-part <partition>] <id> <status> [note]
                                                          set a scope's status, or one part's, as you
`

const queueUsage = `Usage:
  hikari queue new <name>                   make a queue, paused
  hikari queue rename <queue> <name>        rename a queue
  hikari queue remove <queue>               remove an empty queue
  hikari queue add <queue> <scope> [n]      put a scope in a queue, at place n or at the end
  hikari queue move <queue> <scope> [n]     move a scope within its queue, or to another
  hikari queue out <scope>                  take a scope out of its queue
  hikari queue start <queue>                start a queue
  hikari queue pause <queue>                pause a queue: it finishes the scope in progress
  hikari queue list                         list the queues
Each takes -file topology.yaml before its arguments.
`

// target is the topology file of a command, and how to reach its pipeline.
type target struct {
	file, shown string
}

func flags(name, usage string, args []string) (*flag.FlagSet, *target, *string) {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	file := fs.String("file", "topology.yaml", "the topology file of the project")
	part := fs.String("part", "", "the partition whose part to set")
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	fs.Parse(args)
	shown := *file
	if rel, err := filepath.Rel(".", *file); err == nil {
		shown = rel
	}
	return fs, &target{file: *file, shown: shown}, part
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

func taskCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, taskUsage)
		os.Exit(2)
	}
	fs, t, part := flags("task "+args[0], taskUsage, args[1:])
	rest := fs.Args()
	switch args[0] {
	case "stub":
		words := strings.TrimSpace(strings.Join(rest, " "))
		if words == "" {
			fmt.Fprint(os.Stderr, taskUsage)
			os.Exit(2)
		}
		name, body := stubName(words)
		var v TaskView
		if t.live() {
			if err := t.call(ctx, http.MethodPost, "/stub", StubRequest{Name: name, Body: body}, &v); err != nil {
				return err
			}
		} else {
			st, _, err := t.offline(ctx)
			if err != nil {
				return err
			}
			task, _, err := st.PlantStub(name, body, User)
			if err != nil {
				return err
			}
			v = st.TaskView(task)
		}
		fmt.Printf("Planted the stub [[%s]] in %s.\n", v.ID, v.File)
		return nil
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
		req := StatusRequest{Task: rest[0], Part: *part, Status: rest[1], Note: strings.Join(rest[2:], " ")}
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

// stubName takes a stub's name from its words: the first line, cut at a
// word near 60 characters, and the rest is the body.
func stubName(words string) (string, string) {
	first, _, _ := strings.Cut(words, "\n")
	name := strings.TrimRight(strings.TrimSpace(first), ".")
	if len(name) > 60 {
		cut := strings.LastIndex(name[:60], " ")
		if cut < 20 {
			cut = 60
		}
		name = strings.TrimRight(name[:cut], ",;:") + "…"
	}
	if strings.TrimSpace(words) == name {
		return name, ""
	}
	return name, words
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
	return st.View(topo.Orchestration, topo.PartitionAgents()), nil
}

func queueCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, queueUsage)
		os.Exit(2)
	}
	fs, t, _ := flags("queue "+args[0], queueUsage, args[1:])
	rest := fs.Args()
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
		op = QueueOp{Op: args[0], Queue: rest[0], Scope: rest[1], Position: position(2)}
	case "out":
		need(1)
		op = QueueOp{Op: "out", Scope: rest[0]}
	default:
		fmt.Fprint(os.Stderr, queueUsage)
		os.Exit(2)
	}
	op.Scope = strings.Trim(op.Scope, "[]")
	if t.live() {
		if err := t.call(ctx, http.MethodPost, "/queue", op, nil); err != nil {
			return err
		}
	} else {
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

// formatView renders the pipeline for a terminal.
func formatView(v View) string {
	var b strings.Builder
	line := func(t TaskView) {
		fmt.Fprintf(&b, "  [[%s]] %s", t.ID, t.Name)
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
	if len(v.Stubs) > 0 {
		b.WriteString("Stubs\n")
		for _, t := range v.Stubs {
			line(t)
		}
	}
	if len(v.Drafts) > 0 {
		b.WriteString("Draft scopes\n")
		for _, t := range v.Drafts {
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
