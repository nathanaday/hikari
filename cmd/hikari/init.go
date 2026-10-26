package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nathanaday/hikari/internal/onboard"
	"github.com/nathanaday/hikari/internal/scaffold"
)

var invalidNameChars = regexp.MustCompile(`[^a-z0-9_-]+`)

// projectName makes a compose project name from a directory name (rule
// PR25), or returns "" when nothing valid is left.
func projectName(dir string) string {
	name := invalidNameChars.ReplaceAllString(strings.ToLower(filepath.Base(dir)), "-")
	return strings.TrimLeft(strings.TrimRight(name, "-_"), "-_")
}

func initProject(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	name := fs.String("name", "", "the compose project name; the default comes from the directory name")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: hikari init [flags] <dir>")
		fs.PrintDefaults()
	}
	fs.Parse(args)
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}
	dir, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		return err
	}
	if *name == "" {
		*name = projectName(dir)
	}
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`).MatchString(*name) {
		return fmt.Errorf("cannot make a project name from %q; give one with -name (lowercase letters, digits, dashes, and underscores)", filepath.Base(dir))
	}
	committed, err := scaffold.WriteProject(dir, *name, onboard.Write)
	if err != nil {
		return err
	}
	fmt.Printf("Made the %s project in %s.\n\n", *name, dir)
	if !committed {
		fmt.Printf("git has no identity, so the files are not committed. Set user.name and user.email, then run:\n  git -C %s add --all && git -C %s commit -m \"Start the %s project\"\n\n", dir, dir, *name)
	}
	fmt.Printf("Next:\n  cp %s %s    and set a credential\n  hikari up %s\n",
		filepath.Join(dir, ".env.example"), filepath.Join(dir, ".env"), filepath.Join(dir, "compose.yaml"))
	fmt.Printf("\nTo bring existing repositories into the project, open Claude Code in %s and run /containerize-repo.\n", dir)
	return nil
}
