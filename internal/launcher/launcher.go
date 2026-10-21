// Package launcher starts one container per agent with the docker CLI. Each
// container gets its own bridge network, so containers cannot reach each
// other directly; the only path between them is the server. An owner's
// container also gets its scope and nothing else of its repo.
package launcher

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
)

// PassEnv lists host variables that are passed to every container when set.
var PassEnv = []string{"ANTHROPIC_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN"}

type Launcher struct {
	Image     string // the base agent image
	ServerURL string // the server address as seen from inside a container
	RunID     string
	// EmptyDir is an empty host directory that hides .git in a whole-repo
	// scope.
	EmptyDir string
}

// Spec describes the container of one agent.
type Spec struct {
	Name    string
	Token   string
	Image   string
	WorkDir string
	Mounts  []Mount
}

func (l *Launcher) label() string { return "hikari.run=" + l.RunID }

func (l *Launcher) name(node string) string { return "hikari-" + l.RunID + "-" + node }

func (l *Launcher) CheckImage(ctx context.Context) error {
	if err := docker(ctx, "image", "inspect", l.Image).Run(); err != nil {
		return fmt.Errorf("docker image %q not found; build it with `make image`", l.Image)
	}
	return nil
}

// Start creates the network and container of spec and returns at once. The
// container output is logged with the agent name as prefix. done receives
// the exit error when the container stops.
func (l *Launcher) Start(ctx context.Context, spec Spec, done func(error)) error {
	for _, m := range spec.Mounts {
		if m.Volume {
			if err := ensureVolume(ctx, m.Source, spec.Image); err != nil {
				return err
			}
		}
	}
	name := l.name(spec.Name)
	if out, err := docker(ctx, "network", "create", "--label", l.label(), name).CombinedOutput(); err != nil {
		return fmt.Errorf("create network %s: %v: %s", name, err, out)
	}
	cmd := docker(ctx, l.runArgs(spec)...)
	// Pass secrets by name so their values never appear in a process list.
	cmd.Env = append(os.Environ(), "HIKARI_SERVER_URL="+l.ServerURL, "HIKARI_TOKEN="+spec.Token)

	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	go prefix(spec.Name, out)
	go func() { done(cmd.Wait()) }()
	return nil
}

func (l *Launcher) runArgs(spec Spec) []string {
	name := l.name(spec.Name)
	args := []string{"run", "--rm", "--init",
		"--name", name,
		"--label", l.label(),
		"--network", name,
		"--add-host", "host.docker.internal:host-gateway",
		"-e", "HIKARI_SERVER_URL",
		"-e", "HIKARI_TOKEN",
		"-e", "DISABLE_AUTOUPDATER=1",
	}
	for _, v := range PassEnv {
		if os.Getenv(v) != "" {
			args = append(args, "-e", v)
		}
	}
	if spec.WorkDir != "" {
		args = append(args, "--workdir", spec.WorkDir)
	}
	for _, m := range spec.Mounts {
		args = append(args, "--mount", m.flag())
	}
	return append(args, spec.Image)
}

// Cleanup removes every container and network of this run.
func (l *Launcher) Cleanup() {
	ctx := context.Background()
	filter := "label=" + l.label()
	if ids := lines(docker(ctx, "ps", "-aq", "--filter", filter)); len(ids) > 0 {
		docker(ctx, append([]string{"rm", "-f"}, ids...)...).Run()
	}
	if ids := lines(docker(ctx, "network", "ls", "-q", "--filter", filter)); len(ids) > 0 {
		docker(ctx, append([]string{"network", "rm"}, ids...)...).Run()
	}
}

// ensureVolume creates a named volume that the agent user can write, if it
// does not exist.
func ensureVolume(ctx context.Context, name, image string) error {
	if docker(ctx, "volume", "inspect", name).Run() == nil {
		return nil
	}
	if out, err := docker(ctx, "volume", "create", "--label", "hikari.cache=1", name).CombinedOutput(); err != nil {
		return fmt.Errorf("create volume %s: %v: %s", name, err, out)
	}
	out, err := docker(ctx, "run", "--rm", "--user", "root", "--entrypoint", "chown",
		"--mount", "type=volume,source="+name+",target=/volume", image, "agent:agent", "/volume").CombinedOutput()
	if err != nil {
		return fmt.Errorf("prepare volume %s: %v: %s", name, err, out)
	}
	return nil
}

func docker(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "docker", args...)
}

func lines(cmd *exec.Cmd) []string {
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return strings.Fields(string(out))
}

func prefix(name string, r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		log.Printf("[%s] %s", name, sc.Text())
	}
}
