package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	iofs "io/fs"
	"os"
	"strings"

	"github.com/nathanaday/hikari/internal/console"
	"github.com/nathanaday/hikari/internal/linked"
	"github.com/nathanaday/hikari/internal/onboard"
	"github.com/nathanaday/hikari/internal/scaffold"
	"github.com/nathanaday/hikari/internal/topology"
)

// topologyArg parses the flags of a command that takes one topology file.
func topologyArg(fs *flag.FlagSet, args []string, usage string) (string, error) {
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: "+usage)
		fs.PrintDefaults()
	}
	fs.Parse(args)
	return fileArg(fs)
}

// fileArg returns the topology file that the command names, or
// topology.yaml when it names none. A directory that has only the old
// compose.yaml is told to rename it.
func fileArg(fs *flag.FlagSet) (string, error) {
	switch fs.NArg() {
	case 0:
	case 1:
		return fs.Arg(0), nil
	default:
		fs.Usage()
		os.Exit(2)
	}
	if _, err := os.Stat(scaffold.TopologyFile); errors.Is(err, iofs.ErrNotExist) {
		if _, err := os.Stat("compose.yaml"); err == nil {
			return "", fmt.Errorf("%s does not exist; hikari now opens %s by default, so rename compose.yaml to %s", scaffold.TopologyFile, scaffold.TopologyFile, scaffold.TopologyFile)
		}
	}
	return scaffold.TopologyFile, nil
}

// loadShown loads a topology, and shows its errors one to a line.
func loadShown(ctx context.Context, con *console.Console, file string) (*topology.Topology, error) {
	return showLoaded(con, topology.Load, ctx, file)
}

func showLoaded(con *console.Console, load func(context.Context, string) (*topology.Topology, error), ctx context.Context, file string) (*topology.Topology, error) {
	topo, err := load(ctx, file)
	if err != nil {
		src, _ := os.ReadFile(file)
		for _, f := range topology.Findings(err, src) {
			for _, line := range strings.Split(f.Message, "\n") {
				if f.Line > 0 {
					line = fmt.Sprintf("%s:%d: %s", linked.Shown(file), f.Line, line)
				}
				con.Error(line)
			}
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
	draft := fs.Bool("draft", false, "check a target topology: repos and folders that do not exist yet are allowed, and the running project is not checked")
	file, err := topologyArg(fs, args, "hikari check [flags] [topology.yaml]")
	if err != nil {
		return err
	}
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
	file, err := topologyArg(fs, args, "hikari sync [flags] [topology.yaml]")
	if err != nil {
		return err
	}
	con := console.New(os.Stdout)
	topo, err := loadShown(ctx, con, file)
	if err != nil {
		return err
	}
	return showResults(con, linked.Sync(topo, *from, writeSkills))
}

// push pushes your branches (rules PS1 to PS5).
func push(ctx context.Context, args []string) error {
	file, err := topologyArg(flag.NewFlagSet("push", flag.ExitOnError), args, "hikari push [topology.yaml]")
	if err != nil {
		return err
	}
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

// status prints what the files say of each partition (rules SU1 to SU4 of
// the containerize spec).
func status(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	tests := fs.Bool("tests", false, "print each test of each suite, with its outcome")
	file, err := topologyArg(fs, args, "hikari status [flags] [topology.yaml]")
	if err != nil {
		return err
	}
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

// shortCommits writes described commits as <repo>@<12 characters>.
func shortCommits(commits []string) string {
	out := make([]string, len(commits))
	for i, c := range commits {
		repo, commit, _ := strings.Cut(c, "@")
		out[i] = fmt.Sprintf("%s@%.12s", repo, commit)
	}
	return strings.Join(out, ", ")
}

// statusLines renders the status of each partition as plain lines.
func statusLines(partitions []linked.PartitionStatus, tests bool) []string {
	var out []string
	for _, s := range partitions {
		head := s.Name + "  "
		switch {
		case !s.Cloned:
			head += "not cloned"
		case len(s.Commits) == 0:
			head += "not described"
		default:
			head += fmt.Sprintf("described at %s · %s", shortCommits(s.Commits), plural(s.Features, "feature"))
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

// writeSkills writes the host skills into the project repository (rule HS2
// of the containerize spec).
func writeSkills(dir string) linked.Result {
	if err := onboard.Write(dir); err != nil {
		return linked.Result{Repo: topology.ProjectRepo, Text: "write the host skills: " + err.Error(), Failed: true}
	}
	return linked.Result{Repo: topology.ProjectRepo, Text: "wrote the host skills " + strings.Join(onboard.Names(), ", ") + " into " + onboard.SkillsDir}
}
