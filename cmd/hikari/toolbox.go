package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/nathanaday/hikari/internal/console"
	"github.com/nathanaday/hikari/internal/toolbox"
)

const toolboxUsage = `Usage:
  hikari toolbox list                      the built-in policies, styles, skills, and profile recipes, and which the vault has
  hikari toolbox show <kind>/<name>        print one item
  hikari toolbox add [-force] <kind>/<name>...   install items into the vault; a profile is printed to paste
Each takes -file topology.yaml before its arguments. A policy reaches every
agent on its next turn; a style or a skill reaches the agents you grant it
to, with grants.styles, grants.skills, or a profile. An installed file is
yours: change it as you like, and commit it with the project.
`

// toolboxCmd is hikari toolbox: the library of ready-made ways for agents
// to work, installed into the vault.
func toolboxCmd(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, toolboxUsage)
		os.Exit(2)
	}
	fs := flag.NewFlagSet("toolbox "+args[0], flag.ExitOnError)
	file := fs.String("file", "topology.yaml", "the topology file of the project")
	force := fs.Bool("force", false, "replace an item the vault already has")
	fs.Usage = func() { fmt.Fprint(os.Stderr, toolboxUsage) }
	fs.Parse(args[1:])
	rest := fs.Args()
	con := console.New(os.Stdout)

	switch args[0] {
	case "list":
		topo, err := loadShown(ctx, con, *file)
		if err != nil {
			return err
		}
		st := topo.VaultOf()
		for _, kind := range toolbox.Kinds {
			con.Println("")
			con.Println("  " + con.Bold(kindTitle(kind)))
			for _, s := range toolbox.List(st) {
				if s.Kind != kind {
					continue
				}
				mark := con.Dim("·")
				note := ""
				if s.Installed {
					mark, note = con.Green("✓"), con.Dim("  in the vault at "+s.Path)
				}
				con.Println(fmt.Sprintf("  %s %-24s %s%s", mark, s.Kind+"/"+s.Name, s.Description, note))
			}
		}
		con.Println("")
		con.Println("  hikari toolbox show <kind>/<name> prints one; hikari toolbox add <kind>/<name> installs it.")
		return nil
	case "show":
		if len(rest) != 1 {
			fmt.Fprint(os.Stderr, toolboxUsage)
			os.Exit(2)
		}
		it, err := toolbox.Find(rest[0])
		if err != nil {
			return err
		}
		text, err := toolbox.Text(it)
		if err != nil {
			return err
		}
		fmt.Print(strings.TrimRight(text, "\n") + "\n")
		return nil
	case "add":
		if len(rest) == 0 {
			fmt.Fprint(os.Stderr, toolboxUsage)
			os.Exit(2)
		}
		topo, err := loadShown(ctx, con, *file)
		if err != nil {
			return err
		}
		st := topo.VaultOf()
		failed := false
		for _, ref := range rest {
			it, err := toolbox.Find(ref)
			if err != nil {
				con.Error(err.Error())
				failed = true
				continue
			}
			if it.Kind == toolbox.Profile {
				text, _ := toolbox.Text(it)
				con.Println("  " + con.Yellow("!") + " a profile goes into x-hikari.profiles of " + *file + "; paste this:")
				for _, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
					con.Println("    " + l)
				}
				continue
			}
			written, err := toolbox.Install(st, it, *force)
			if err != nil {
				con.Error(err.Error())
				failed = true
				continue
			}
			con.Println(fmt.Sprintf("  %s %s: wrote %s; %s", con.Green("✓"), it.Ref(), strings.Join(written, ", "), toolbox.Grant(it)))
		}
		if failed {
			return errShown
		}
		return nil
	}
	fmt.Fprint(os.Stderr, toolboxUsage)
	os.Exit(2)
	return nil
}

func kindTitle(kind string) string {
	switch kind {
	case toolbox.Policy:
		return "Policies: rules for every agent, in vault/public/policies/"
	case toolbox.Style:
		return "Styles: how an agent's work should look, in vault/styles/, granted"
	case toolbox.Skill:
		return "Skills: how a kind of task is done, in vault/skills/, granted"
	}
	return "Profiles: bundles of model, effort, review, skills, and styles, for x-hikari.profiles"
}
