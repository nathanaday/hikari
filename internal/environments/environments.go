// Package environments runs the environment of a tester on the host: a
// compose project that hikari starts, connects the tester to, logs, and
// stops (section 3 of the environments spec).
package environments

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// The states of an environment (rule ST6).
const (
	None     = "none"
	Starting = "starting"
	Running  = "running"
	Failed   = "failed"
	Stopped  = "stopped"
)

// StartTimeout bounds how long a start waits for healthy services.
const StartTimeout = 5 * time.Minute

// Service is the state of one service of an environment.
type Service struct {
	Name     string `yaml:"name" json:"name"`
	State    string `yaml:"state" json:"state"`
	Health   string `yaml:"health,omitempty" json:"health,omitempty"`
	Image    string `yaml:"image" json:"image"`
	Platform string `yaml:"platform,omitempty" json:"platform,omitempty"`
	Replicas int    `yaml:"replicas" json:"replicas"`
	// Emulated says that the image's platform is not the host's (rule
	// ST4).
	Emulated bool `yaml:"emulated,omitempty" json:"emulated,omitempty"`
}

// Status is the state of a hub's environment.
type Status struct {
	State    string    `yaml:"state" json:"state"`
	Scenario string    `yaml:"scenario,omitempty" json:"scenario,omitempty"`
	Reason   string    `yaml:"reason,omitempty" json:"reason,omitempty"`
	Services []Service `yaml:"services,omitempty" json:"services,omitempty"`
	Started  time.Time `yaml:"started,omitzero" json:"started,omitzero"`
	Stops    time.Time `yaml:"stops,omitzero" json:"stops,omitzero"`
	// Logs is where the tester reads each service's log.
	Logs string `yaml:"logs,omitempty" json:"logs,omitempty"`
	// Connected says whether the tester's container joined the network.
	Connected bool `yaml:"connected" json:"connected"`
}

// Spec is everything a start needs.
type Spec struct {
	// Project is the environment project's name; Network its network.
	Project string
	Network string
	// Dir holds environment.yaml and the override.
	Dir      string
	File     string
	Override []byte
	Replicas map[string]int
	Scenario string
	// Tester is the tester's container, or "" when it does not run.
	Tester string
	// LogDir receives one log file per service (rule LG1); LogsShown is
	// the same directory as the tester sees it.
	LogDir    string
	LogsShown string
	Minutes   int
	// HostPlatform is the platform of the host's containers.
	HostPlatform string
}

// Env is one running environment.
type Env struct {
	spec   Spec
	mu     sync.Mutex
	status Status
	logs   []*exec.Cmd
	timer  *time.Timer
	// onChange is told of each change of the status.
	onChange func()
}

// Start starts an environment and waits until its services are healthy,
// or StartTimeout ends (rules ST2 to ST4). onChange is told of each later
// change of the status, such as the stop at the end of its time.
func Start(ctx context.Context, spec Spec, onChange func()) *Env {
	e := &Env{spec: spec, onChange: onChange}
	e.status = Status{State: Starting, Scenario: spec.Scenario, Started: time.Now().UTC(), Logs: spec.LogsShown}
	override := filepath.Join(spec.Dir, ".environment.override.yaml")
	if err := os.WriteFile(override, spec.Override, 0o644); err != nil {
		e.fail(err.Error())
		return e
	}
	args := []string{"up", "--detach", "--wait", "--wait-timeout", fmt.Sprint(int(StartTimeout.Seconds())), "--remove-orphans"}
	for _, name := range slices.Sorted(func(yield func(string) bool) {
		for k := range spec.Replicas {
			if !yield(k) {
				return
			}
		}
	}) {
		args = append(args, "--scale", fmt.Sprintf("%s=%d", name, spec.Replicas[name]))
	}
	ctx, cancel := context.WithTimeout(ctx, StartTimeout+30*time.Second)
	defer cancel()
	out, err := e.compose(ctx, args...).CombinedOutput()
	e.refresh(context.Background())
	if err != nil {
		e.fail("docker compose up: " + lastLines(string(out), 12))
		return e
	}
	if spec.Tester != "" {
		if out, err := exec.CommandContext(ctx, "docker", "network", "connect", spec.Network, spec.Tester).CombinedOutput(); err != nil && !strings.Contains(string(out), "already exists") {
			e.fail("connect the tester: " + strings.TrimSpace(string(out)))
			return e
		}
		e.status.Connected = true
	}
	e.startLogs()
	minutes := spec.Minutes
	if minutes <= 0 {
		minutes = 30
	}
	e.mu.Lock()
	e.status.State = Running
	e.status.Stops = e.status.Started.Add(time.Duration(minutes) * time.Minute)
	e.timer = time.AfterFunc(time.Until(e.status.Stops), func() {
		e.Stop(context.Background(), "its time ended")
	})
	e.mu.Unlock()
	return e
}

// compose runs a compose command of the environment project, with no
// variable of the host's environment beyond what Docker itself needs
// (rule CR1).
func (e *Env) compose(ctx context.Context, args ...string) *exec.Cmd {
	base := []string{"compose", "-p", e.spec.Project, "--project-directory", e.spec.Dir,
		"--env-file", os.DevNull, "-f", e.spec.File, "-f", filepath.Join(e.spec.Dir, ".environment.override.yaml")}
	cmd := exec.CommandContext(ctx, "docker", append(base, args...)...)
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if k == "PATH" || k == "HOME" || strings.HasPrefix(k, "DOCKER_") || k == "TMPDIR" {
			env = append(env, kv)
		}
	}
	cmd.Env = env
	return cmd
}

func (e *Env) fail(reason string) {
	e.mu.Lock()
	e.status.State, e.status.Reason = Failed, reason
	e.mu.Unlock()
	e.down(context.Background())
}

// Status returns the environment's status, refreshed from Docker while it
// runs.
func (e *Env) Status(ctx context.Context) Status {
	e.mu.Lock()
	running := e.status.State == Running
	e.mu.Unlock()
	if running {
		e.refresh(ctx)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.status
	s.Services = slices.Clone(s.Services)
	return s
}

// psEntry is one container of `docker compose ps --format json`.
type psEntry struct {
	Service string `json:"Service"`
	State   string `json:"State"`
	Health  string `json:"Health"`
	Image   string `json:"Image"`
}

// refresh reads the services' state from Docker.
func (e *Env) refresh(ctx context.Context) {
	out, err := e.compose(ctx, "ps", "--all", "--format", "json").Output()
	if err != nil {
		return
	}
	byName := map[string]*Service{}
	var order []string
	add := func(p psEntry) {
		s := byName[p.Service]
		if s == nil {
			s = &Service{Name: p.Service, State: p.State, Health: p.Health, Image: p.Image}
			byName[p.Service] = s
			order = append(order, p.Service)
		}
		s.Replicas++
		if p.State != "running" {
			s.State = p.State
		}
		if p.Health != "" && p.Health != "healthy" {
			s.Health = p.Health
		}
	}
	trimmed := bytes.TrimSpace(out)
	if bytes.HasPrefix(trimmed, []byte("[")) {
		var list []psEntry
		json.Unmarshal(trimmed, &list)
		for _, p := range list {
			add(p)
		}
	} else {
		sc := bufio.NewScanner(bytes.NewReader(trimmed))
		for sc.Scan() {
			var p psEntry
			if json.Unmarshal(sc.Bytes(), &p) == nil {
				add(p)
			}
		}
	}
	slices.Sort(order)
	services := make([]Service, 0, len(order))
	for _, name := range order {
		s := *byName[name]
		if out, err := exec.CommandContext(ctx, "docker", "image", "inspect", "--format", "{{.Os}}/{{.Architecture}}", s.Image).Output(); err == nil {
			s.Platform = strings.TrimSpace(string(out))
			s.Emulated = e.spec.HostPlatform != "" && s.Platform != e.spec.HostPlatform
		}
		services = append(services, s)
	}
	e.mu.Lock()
	e.status.Services = services
	e.mu.Unlock()
}

// startLogs follows each service's log into its file (rule LG1).
func (e *Env) startLogs() {
	os.RemoveAll(e.spec.LogDir)
	if err := os.MkdirAll(e.spec.LogDir, 0o755); err != nil {
		return
	}
	for name := range e.spec.Replicas {
		f, err := os.Create(filepath.Join(e.spec.LogDir, name+".log"))
		if err != nil {
			continue
		}
		cmd := e.compose(context.Background(), "logs", "--follow", "--no-color", "--timestamps", name)
		cmd.Stdout, cmd.Stderr = f, f
		if err := cmd.Start(); err != nil {
			f.Close()
			continue
		}
		go func() { cmd.Wait(); f.Close() }()
		e.logs = append(e.logs, cmd)
	}
}

// Stop disconnects the tester and removes the environment's containers,
// networks, and volumes (rule ST5). why says why it stopped.
func (e *Env) Stop(ctx context.Context, why string) {
	e.mu.Lock()
	if e.status.State == Stopped {
		e.mu.Unlock()
		return
	}
	if e.timer != nil {
		e.timer.Stop()
	}
	e.mu.Unlock()
	e.down(ctx)
	e.mu.Lock()
	if e.status.State != Failed {
		e.status.State, e.status.Reason = Stopped, why
	}
	e.status.Connected = false
	onChange := e.onChange
	e.mu.Unlock()
	if onChange != nil {
		onChange()
	}
}

func (e *Env) down(ctx context.Context) {
	if e.spec.Tester != "" {
		exec.CommandContext(ctx, "docker", "network", "disconnect", "--force", e.spec.Network, e.spec.Tester).Run()
	}
	for _, cmd := range e.logs {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}
	e.logs = nil
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	e.compose(ctx, "down", "--volumes", "--remove-orphans", "--timeout", "5").Run()
}

// Active reports whether the environment runs or is starting.
func (e *Env) Active() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.status.State == Running || e.status.State == Starting
}

// Container returns the container of a service of a compose project, or ""
// when it does not run.
func Container(ctx context.Context, project, service string) string {
	out, err := exec.CommandContext(ctx, "docker", "compose", "-p", project, "ps", "--quiet", service).Output()
	if err != nil {
		return ""
	}
	id, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return id
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
