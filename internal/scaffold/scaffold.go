// Package scaffold writes the files of a new project: the project
// repository of hikari init, or a topology file alone.
package scaffold

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/internal/vault"
)

// ProjectIgnore is the .gitignore of a new project repository (rule PR23,
// IN1 of the linked repositories spec, and HS3 of the containerize spec).
// The runs directory is anchored, so that the .hikari directory of each
// partition's folder, which holds the private documents, stays versioned.
func ProjectIgnore() string {
	return strings.Join([]string{".env", "/.hikari/", "/repos/", ".obsidian/", ".DS_Store", "/.claude/settings.local.json"}, "\n") + "\n"
}

// TopologyFile is the name of the topology file that hikari init writes and
// that hikari's commands open when no file is named.
const TopologyFile = "topology.yaml"

const envExample = `# Copy this file to .env and set one credential. Git ignores .env.
# Make a long-lived token with: claude setup-token
CLAUDE_CODE_OAUTH_TOKEN=
# ANTHROPIC_API_KEY=

# When more than one person runs agents for this project, set your own
# branch, so that your agents' commits do not mix with a teammate's.
# HIKARI_BRANCH=hikari-<your name>
`

const starterTopology = `# The %[1]s project. Start it with: hikari up
#
# This is the topology file: a Docker Compose file whose x-hikari keys
# describe the orchestration agent and the partitions; see the hikari README.

name: %[1]s

x-hikari:
  # Agents commit to this branch of this repository and of each linked repo,
  # and you merge it. HIKARI_BRANCH in .env gives each person their own.
  project: {branch: "${HIKARI_BRANCH:-hikari}"}
  # repos:                   # a repository that holds a partition's code
  #   app: {url: https://github.com/you/app.git}   # cloned into repos/app by: hikari sync
  orchestration: orchestrator   # every other agent has a partition

x-agent: &agent
  environment: [CLAUDE_CODE_OAUTH_TOKEN, ANTHROPIC_API_KEY]

services:
  orchestrator:
    <<: *agent
    image: hikari-agent      # a partition agent builds its own image from hikari-agent instead
    x-hikari:
      description: Orchestrates the project. Plans from the feature maps and reports to you.

  # A partition agent. A partition is a list of folders, each a path in a
  # repo under x-hikari.repos, or in this repository with repo: project.
  # app:
  #   <<: *agent
  #   build: {context: ., dockerfile: app.Dockerfile}   # FROM hikari-agent, with the app's toolchain
  #   x-hikari:
  #     description: Owns the app.
  #     partition:
  #       folders: [{repo: app}]         # no path: the whole repo, at /workspace/app
  #       test: make -C app test         # runs in /workspace
`

// StarterTopology returns the starter topology file of a project named
// name. Without project, it leaves out x-hikari.project, for a directory
// that is not a project repository (rule TA6 of the topology API spec).
func StarterTopology(name string, project bool) string {
	text := fmt.Sprintf(starterTopology, name)
	if !project {
		text = strings.Replace(text, projectLines, "", 1)
		text = strings.Replace(text, linkedRepoLines, "", 1)
	}
	return text
}

const projectLines = `  # Agents commit to this branch of this repository and of each linked repo,
  # and you merge it. HIKARI_BRANCH in .env gives each person their own.
  project: {branch: "${HIKARI_BRANCH:-hikari}"}
`

const linkedRepoLines = `  # repos:                   # a repository that holds a partition's code
  #   app: {url: https://github.com/you/app.git}   # cloned into repos/app by: hikari sync
`

// Empty reports whether a directory holds nothing but hikari's own
// .hikari folder, which hikari up writes before a project exists.
func Empty(entries []os.DirEntry) bool {
	for _, e := range entries {
		if e.Name() != ".hikari" {
			return false
		}
	}
	return true
}

// WriteProject writes a new project repository in dir and commits it with
// the user's git identity. It reports whether it made the commit.
func WriteProject(dir, name string, writeSkills func(dir string) error) (bool, error) {
	entries, err := os.ReadDir(dir)
	switch {
	case err == nil && !Empty(entries):
		return false, fmt.Errorf("%s exists and is not empty", dir)
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return false, err
	}
	files := map[string]string{
		".gitignore":   ProjectIgnore(),
		".env.example": envExample,
		TopologyFile:   StarterTopology(name, true),
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
	st := vault.Vault{Root: filepath.Join(dir, topology.DefaultVault)}
	if err := st.Prepare(nil, true); err != nil {
		return false, err
	}
	if _, err := st.WriteIgnore(); err != nil {
		return false, err
	}
	if err := gitIn(dir, "init", "--quiet", "--initial-branch=main"); err != nil {
		return false, err
	}
	if err := writeSkills(dir); err != nil {
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
