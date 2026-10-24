package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/nathanaday/hikari/internal/console"
	"github.com/nathanaday/hikari/internal/linked"
	"github.com/nathanaday/hikari/internal/topology"
)

// composeArg parses the flags of a command that takes one compose file.
func composeArg(fs *flag.FlagSet, args []string, usage string) string {
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: "+usage)
		fs.PrintDefaults()
	}
	fs.Parse(args)
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}
	return fs.Arg(0)
}

// loadShown loads a topology, and shows its errors one to a line.
func loadShown(ctx context.Context, con *console.Console, file string) (*topology.Topology, error) {
	return showLoaded(con, topology.Load, ctx, file)
}

func showLoaded(con *console.Console, load func(context.Context, string) (*topology.Topology, error), ctx context.Context, file string) (*topology.Topology, error) {
	topo, err := load(ctx, file)
	if err != nil {
		for _, line := range strings.Split(err.Error(), "\n") {
			con.Error(line)
		}
		return nil, errShown
	}
	for _, w := range topo.Warnings {
		con.Warn(w)
	}
	return topo, nil
}

// check runs every check of hikari up and starts nothing (rules CK1 to
// CK3). With -draft, it checks a target topology by the topology's rules
// only (rules DR1 to DR3 of the hikify design spec).
func check(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	draft := fs.Bool("draft", false, "check a target topology: repos and scopes that do not exist yet are allowed, and the running project is not checked")
	file := composeArg(fs, args, "hikari check [flags] <compose.yaml>")
	con := console.New(os.Stdout)
	if *draft {
		topo, err := showLoaded(con, topology.LoadDraft, ctx, file)
		if err != nil {
			return err
		}
		con.Println("  " + con.Green("✓") + " the draft is valid: " + describe(topo))
		return nil
	}
	topo, err := loadShown(ctx, con, file)
	if err != nil {
		return err
	}
	if showFindings(con, linked.Check(topo, true)) {
		return errShown
	}
	con.Println("  " + con.Green("✓") + " " + topo.Name + " can start: " + describe(topo))
	return nil
}

// showFindings shows the findings of a check, and reports whether any is
// an error.
func showFindings(con *console.Console, findings []linked.Finding) bool {
	failed := false
	for _, f := range findings {
		if f.Warning {
			con.Warn(f.Text)
		} else {
			con.Error(f.Text)
			failed = true
		}
	}
	return failed
}

// syncProject makes a clone of the project complete (rules SY1 to SY9).
func syncProject(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("sync", flag.ExitOnError)
	from := fs.String("from", "", "start your branch from this branch of the remote, where yours does not exist yet")
	file := composeArg(fs, args, "hikari sync [flags] <compose.yaml>")
	con := console.New(os.Stdout)
	topo, err := loadShown(ctx, con, file)
	if err != nil {
		return err
	}
	return showResults(con, linked.Sync(topo, *from))
}

// push pushes your branches (rules PS1 to PS5).
func push(ctx context.Context, args []string) error {
	file := composeArg(flag.NewFlagSet("push", flag.ExitOnError), args, "hikari push <compose.yaml>")
	con := console.New(os.Stdout)
	topo, err := loadShown(ctx, con, file)
	if err != nil {
		return err
	}
	return showResults(con, linked.Push(topo))
}

func showResults(con *console.Console, results []linked.Result) error {
	width := 0
	for _, r := range results {
		width = max(width, len(r.Repo))
	}
	for _, r := range results {
		mark := con.Green("✓")
		switch {
		case r.Failed:
			mark = con.Red("✗")
		case r.Warning:
			mark = con.Yellow("!")
		}
		con.Println(fmt.Sprintf("  %s %-*s  %s", mark, width, r.Repo, r.Text))
	}
	if linked.Failed(results) {
		return errShown
	}
	return nil
}

// status prints what the files say of each system (rules SU1 to SU4 of
// the containerize spec).
func status(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	tests := fs.Bool("tests", false, "print each test of each suite, with its outcome")
	file := composeArg(fs, args, "hikari status [flags] <compose.yaml>")
	con := console.New(os.Stdout)
	topo, err := loadShown(ctx, con, file)
	if err != nil {
		return err
	}
	for _, line := range statusLines(linked.Status(topo), *tests) {
		con.Println(line)
	}
	return nil
}

// statusLines renders the status of each system as plain lines.
func statusLines(systems []linked.SystemStatus, tests bool) []string {
	var out []string
	for _, s := range systems {
		head := s.Name + "  "
		switch {
		case !s.Cloned:
			head += "not cloned"
		case s.Commit == "":
			head += "not described"
		default:
			head += fmt.Sprintf("described at %.12s · %s", s.Commit, plural(s.Features, "feature"))
			if parts := counts(s.Health, "passing", "failing", "pending", "untested"); parts != "" {
				head += ": " + parts
			}
		}
		out = append(out, head)
		if s.Problem != "" {
			out = append(out, "  problem: "+s.Problem)
		}
		if s.Cloned && !s.Results {
			out = append(out, "  no test results on this machine")
		}
		for _, su := range s.Suites {
			out = append(out, fmt.Sprintf("  %s  ran %s · %s", su.Name, su.Ran.Local().Format("2006-01-02 15:04"), counts(su.Counts, "passed", "failed", "skipped")))
			if tests {
				for _, t := range su.Tests {
					out = append(out, fmt.Sprintf("    %-7s %s", t.Outcome, t.ID))
				}
			}
		}
	}
	return out
}

// counts joins the nonzero counts, in the order of keys.
func counts(m map[string]int, keys ...string) string {
	var parts []string
	for _, k := range keys {
		if n := m[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, k))
		}
	}
	return strings.Join(parts, ", ")
}
