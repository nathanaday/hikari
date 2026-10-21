// Package launcher starts one container per agent node with the docker CLI.
// Each container gets its own bridge network, so containers cannot reach each
// other directly; the only path between them is the hub.
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
	Image  string
	HubURL string // the hub address as seen from inside a container
	RunID  string
}

func (l *Launcher) label() string { return "hikari.run=" + l.RunID }

func (l *Launcher) name(node string) string { return "hikari-" + l.RunID + "-" + node }

func (l *Launcher) CheckImage(ctx context.Context) error {
	if err := docker(ctx, "image", "inspect", l.Image).Run(); err != nil {
		return fmt.Errorf("docker image %q not found; build it with `make image`", l.Image)
	}
	return nil
}

// Start creates the network and container of node and returns at once. The
// container output is logged with the node name as prefix. done receives the
// exit error when the container stops.
func (l *Launcher) Start(ctx context.Context, node, token string, done func(error)) error {
	name := l.name(node)
	if out, err := docker(ctx, "network", "create", "--label", l.label(), name).CombinedOutput(); err != nil {
		return fmt.Errorf("create network %s: %v: %s", name, err, out)
	}
	cmd := docker(ctx, "run", "--rm", "--init",
		"--name", name,
		"--label", l.label(),
		"--network", name,
		"--add-host", "host.docker.internal:host-gateway",
		"-e", "HIKARI_HUB_URL",
		"-e", "HIKARI_TOKEN",
		"-e", "DISABLE_AUTOUPDATER=1",
	)
	// Pass secrets by name so their values never appear in a process list.
	cmd.Env = append(os.Environ(), "HIKARI_HUB_URL="+l.HubURL, "HIKARI_TOKEN="+token)
	for _, v := range PassEnv {
		if os.Getenv(v) != "" {
			cmd.Args = append(cmd.Args, "-e", v)
		}
	}
	cmd.Args = append(cmd.Args, l.Image)

	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	go prefix(node, out)
	go func() { done(cmd.Wait()) }()
	return nil
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

func prefix(node string, r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		log.Printf("[%s] %s", node, sc.Text())
	}
}
