package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nathanaday/hikari/internal/gitrepo"
	"github.com/nathanaday/hikari/internal/onboard"
	"github.com/nathanaday/hikari/internal/store"
)

// projectIgnore is the .gitignore of a new project repository (rule PR23,
// IN1 of the linked repositories spec, and HS3 of the containerize spec).
// The runs directory is anchored, so that the .hikari directory of each
// scope, which holds the private documents, stays versioned.
func projectIgnore() string {
	lines := []string{".env", "/.hikari/", "/repos/", ".obsidian/", ".DS_Store", "/.claude/settings.local.json"}
	for _, name := range onboard.Names() {
		lines = append(lines, onboard.IgnoreLine(name))
	}
	return strings.Join(lines, "\n") + "\n"
}

const envExample = `# Copy this file to .env and set one credential. Git ignores .env.
# Make a long-lived token with: claude setup-token
CLAUDE_CODE_OAUTH_TOKEN=
# ANTHROPIC_API_KEY=

# When more than one person runs agents for this project, set your own
# branch, so that your agents' commits do not mix with a teammate's.
# HIKARI_BRANCH=hikari-<your name>
`

const starterCompose = `# The %[1]s project. Start it with: hikari up compose.yaml
#
# This file is a Docker compose file. The x-hikari keys describe the hubs and
# the systems; see the hikari README.

name: %[1]s

x-hikari:
  # Agents commit to this branch of this repository and of each linked repo,
  # and you merge it. HIKARI_BRANCH in .env gives each person their own.
  project: {branch: "${HIKARI_BRANCH:-hikari}"}
  # repos:                   # a repository that holds a system's code
  #   app: {url: https://github.com/you/app.git}   # cloned into repos/app by: hikari sync compose.yaml
  hubs:
    main:
      description: Builds the %[1]s project.
      orchestrator: lead
      members: []            # add each owner here

x-agent: &agent
  environment: [CLAUDE_CODE_OAUTH_TOKEN, ANTHROPIC_API_KEY]

services:
  lead:
    <<: *agent
    image: hikari-agent      # an owner builds its own image from hikari-agent instead
    x-hikari:
      description: Leads the project. Plans from the system maps and reports to you.

  # An owner of a system. A system's code can be in a repo under
  # x-hikari.repos, or in this repository with repo: project.
  # app:
  #   <<: *agent
  #   build: {context: ., dockerfile: app.Dockerfile}   # FROM hikari-agent, with the app's toolchain
  #   x-hikari:
  #     description: Owns the app.
  #     system: {repo: app, test: make test}           # no scope: the whole repo
`

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
	committed, err := writeProject(dir, *name)
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

// writeProject writes a new project repository in dir and commits it with
// the user's git identity. It reports whether it made the commit.
func writeProject(dir, name string) (bool, error) {
	entries, err := os.ReadDir(dir)
	switch {
	case err == nil && len(entries) > 0:
		return false, fmt.Errorf("%s exists and is not empty", dir)
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return false, err
	}
	files := map[string]string{
		".gitignore":   projectIgnore(),
		".env.example": envExample,
		"compose.yaml": fmt.Sprintf(starterCompose, name),
	}
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return false, err
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return false, err
		}
	}
	st := store.Store{Root: filepath.Join(dir, "store")}
	if err := st.Prepare(nil, nil); err != nil {
		return false, err
	}
	if _, err := st.WriteIgnore(); err != nil {
		return false, err
	}
	if err := gitIn(dir, "init", "--quiet", "--initial-branch=main"); err != nil {
		return false, err
	}
	repo := gitrepo.Open(dir)
	if _, err := onboard.Write(dir, repo.IsIgnored); err != nil {
		return false, err
	}
	if err := gitIn(dir, "config", "user.email"); err != nil {
		return false, nil
	}
	if err := gitIn(dir, "add", "--all"); err != nil {
		return false, err
	}
	return true, gitIn(dir, "commit", "--quiet", "-m", "Start the "+name+" project")
}

func gitIn(dir string, args ...string) error {
	var stderr bytes.Buffer
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %v %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
