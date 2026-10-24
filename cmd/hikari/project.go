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
	topo, err := topology.Load(ctx, file)
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
// CK3).
func check(ctx context.Context, args []string) error {
	file := composeArg(flag.NewFlagSet("check", flag.ExitOnError), args, "hikari check <compose.yaml>")
	con := console.New(os.Stdout)
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
		if r.Failed {
			mark = con.Red("✗")
		}
		con.Println(fmt.Sprintf("  %s %-*s  %s", mark, width, r.Repo, r.Text))
	}
	if linked.Failed(results) {
		return errShown
	}
	return nil
}
