// Package launcher runs a topology's compose project. It writes an override
// file that adds hikari to each agent: its mounts, its token, its own
// network, and the hikari binary. Each agent gets its own network, so
// containers cannot reach each other directly; the only path between agents
// is the server.
package launcher

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/nathanaday/hikari/internal/topology"
)

// Project is the compose project of a topology.
type Project struct {
	Topo *topology.Topology
	// Override is the path of the override file, once it is written.
	Override string
	// Env holds the agents' tokens (TokenVar), which every compose command
	// needs once the override file refers to them.
	Env []string
}

func (p *Project) compose(ctx context.Context, args ...string) *exec.Cmd {
	base := []string{"compose", "-p", p.Topo.ID, "-f", p.Topo.File}
	if p.Override != "" {
		base = append(base, "-f", p.Override)
	}
	cmd := docker(ctx, append(base, args...)...)
	cmd.Env = append(os.Environ(), p.Env...)
	return cmd
}

// Running reports whether containers of the project are running.
func (p *Project) Running(ctx context.Context) (bool, error) {
	out, err := p.compose(ctx, "ps", "--quiet").Output()
	if err != nil {
		return false, fmt.Errorf("docker compose ps: %v", err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}

// Build builds the images that the project builds.
func (p *Project) Build(ctx context.Context) error {
	if !slices.ContainsFunc(slices.Collect(maps.Values(p.Topo.Services)), func(s topology.Service) bool { return s.Build }) {
		return nil
	}
	if err := run(p.compose(ctx, "build"), "build"); err != nil {
		return fmt.Errorf("docker compose build: %w", err)
	}
	return nil
}

// Image is what hikari needs to know of an agent's image.
type Image struct {
	Arch string
	// User is the user that the container runs as: the service's user
	// key, or else the image's user.
	User string
}

// InspectAgent reads the image of an agent, pulling it when it is not on
// this machine, and checks the image contract (rules C11 and C12).
func (p *Project) InspectAgent(ctx context.Context, agent string) (Image, error) {
	svc := p.Topo.ServiceOf(agent)
	if svc.Image == "" {
		return Image{}, fmt.Errorf("agent %q has neither an image nor a build", agent)
	}
	inspect := func() ([]byte, error) {
		return docker(ctx, "image", "inspect", "--format", "{{.Architecture}}\t{{.Config.User}}", svc.Image).Output()
	}
	out, err := inspect()
	if err != nil {
		if err := run(docker(ctx, "pull", "--quiet", svc.Image), "pull"); err != nil {
			return Image{}, fmt.Errorf("agent %q: image %s is not on this machine and cannot be pulled", agent, svc.Image)
		}
		if out, err = inspect(); err != nil {
			return Image{}, fmt.Errorf("agent %q: inspect image %s: %v", agent, svc.Image, err)
		}
	}
	arch, user, _ := strings.Cut(strings.TrimSpace(string(out)), "\t")
	img := Image{Arch: arch, User: user}
	if svc.User != "" {
		img.User = svc.User
	}
	if isRoot(img.User) {
		return Image{}, fmt.Errorf("agent %q: image %s runs as root; set USER in its Dockerfile or user in the topology file", agent, svc.Image)
	}
	probe := docker(ctx, "run", "--rm", "--network", "none", "--user", img.User, "--entrypoint", "claude", svc.Image, "--version")
	if out, err := probe.CombinedOutput(); err != nil {
		return Image{}, fmt.Errorf("agent %q: image %s has no claude on its PATH: %s", agent, svc.Image, strings.TrimSpace(string(out)))
	}
	return img, nil
}

func isRoot(user string) bool {
	name, _, _ := strings.Cut(user, ":")
	return name == "" || name == "root" || name == "0"
}

// LinuxBinary returns the hikari binary for Linux on arch: this binary when
// it is one, or else hikari-linux-<arch> next to it, which `make build`
// writes (decision 5).
func LinuxBinary(arch string) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "linux" && runtime.GOARCH == arch {
		return self, nil
	}
	p := filepath.Join(filepath.Dir(self), "hikari-linux-"+arch)
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("no hikari binary for linux/%s at %s; build it with `make build`", arch, p)
	}
	return p, nil
}

// WriteOverride writes the override file to path.
func (p *Project) WriteOverride(path string, in OverrideInput) error {
	data, err := Override(p.Topo, in)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	p.Override = path
	return nil
}

// Up starts the project.
func (p *Project) Up(ctx context.Context) error {
	if err := run(p.compose(ctx, "up", "--detach", "--no-color", "--remove-orphans"), "compose"); err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}
	return nil
}

// Watch reports each stop of an agent's or a sidecar's container to exited,
// with its exit code. Start it before Up, so that no stop is missed.
func (p *Project) Watch(ctx context.Context, exited func(service string, code int)) error {
	cmd := p.compose(ctx, "events", "--json")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			var e struct {
				Action     string            `json:"action"`
				Service    string            `json:"service"`
				Attributes map[string]string `json:"attributes"`
			}
			if json.Unmarshal(sc.Bytes(), &e) != nil || e.Action != "die" {
				continue
			}
			if _, ok := p.Topo.Services[e.Service]; ok {
				code, _ := strconv.Atoi(e.Attributes["exitCode"])
				exited(e.Service, code)
			}
		}
		cmd.Wait()
	}()
	return nil
}

// Logs shows each line of each service's output in hikari's log, with the
// service name as prefix. Each follower ends when its container is removed,
// so the log keeps what a container prints while it stops.
func (p *Project) Logs() {
	for _, name := range slices.Sorted(maps.Keys(p.Topo.Services)) {
		cmd := p.compose(context.Background(), "logs", "--follow", "--no-color", "--no-log-prefix", name)
		out, err := cmd.StdoutPipe()
		if err != nil {
			continue
		}
		cmd.Stderr = cmd.Stdout
		if cmd.Start() != nil {
			continue
		}
		go func() {
			prefix(name, out)
			cmd.Wait()
		}()
	}
}

// Down stops and removes the project's containers and networks. Named
// volumes stay.
func (p *Project) Down() {
	run(p.compose(context.Background(), "down", "--remove-orphans"), "compose")
}

// run runs cmd and logs its output with prefix.
func run(cmd *exec.Cmd, name string) error {
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	prefix(name, out)
	return cmd.Wait()
}

func docker(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "docker", args...)
}

// ansi matches terminal control sequences, which compose prints even with
// --no-color.
var ansi = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func prefix(name string, r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		if line := ansi.ReplaceAllString(sc.Text(), ""); strings.TrimSpace(line) != "" {
			log.Printf("[%s] %s", name, line)
		}
	}
}
