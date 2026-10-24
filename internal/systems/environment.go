package systems

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/e2e"
	"github.com/nathanaday/hikari/internal/environments"
	"github.com/nathanaday/hikari/internal/images"
	"github.com/nathanaday/hikari/internal/sysmap"
)

// envs holds the environment of each hub (rule ST1: one per hub).
type envs struct {
	mu  sync.Mutex
	run string
	by  map[string]*environments.Env
}

// SetRun names the run, for the labels of each environment, so that
// `make clean` finds what a crashed run left.
func (m *Manager) SetRun(id string) {
	m.envs.mu.Lock()
	defer m.envs.mu.Unlock()
	m.envs.run = id
}

// environment reads a tester's environment.yaml, or reports that it has
// none.
func (m *Manager) environment(o *owner) (e2e.Environment, bool, error) {
	data, err := os.ReadFile(filepath.Join(o.store.Root(), e2e.EnvironmentFile))
	if os.IsNotExist(err) {
		return e2e.Environment{}, false, nil
	}
	if err != nil {
		return e2e.Environment{}, true, err
	}
	env, err := e2e.ParseEnvironment(data)
	return env, true, err
}

// environmentFindings checks the environment of a tester against its plan
// and its lock (rules EN1 to EN4).
func (m *Manager) environmentFindings(o *owner, plan e2e.Plan, lock e2e.Lock) []string {
	env, ok, err := m.environment(o)
	if !ok {
		return nil
	}
	if err != nil {
		return strings.Split(err.Error(), "\n")
	}
	return env.PlanFindings(plan, m.imageReleases(lock))
}

// imageReleases returns the pinned releases that are image releases.
func (m *Manager) imageReleases(lock e2e.Lock) map[string]bool {
	out := map[string]bool{}
	for _, r := range lock.Releases {
		if man, ok := m.release(r.System, r.Deliverable, r.Version); ok && man.Image != nil {
			out[r.System+"/"+r.Deliverable] = true
		}
	}
	return out
}

// tester returns the owner of agent when it is a tester, and its hub.
func (m *Manager) tester(agent string) (*owner, string, error) {
	o, err := m.owner(agent)
	if err != nil {
		return nil, "", err
	}
	hub := m.topo.Agents[agent].Tests
	if hub == "" {
		return nil, "", fmt.Errorf("%w: only a tester runs an environment", ErrForbidden)
	}
	return o, hub, nil
}

// environmentProject is the compose project of a hub's environment.
func (m *Manager) environmentProject(hub string) string { return m.topo.Name + "-e2e-" + hub }

// StartEnvironment starts the environment of agent's hub for a scenario of
// its plan, and waits until its services are healthy (rules ST1 to ST4).
func (m *Manager) StartEnvironment(ctx context.Context, agent, scenario string) (environments.Status, error) {
	o, hub, err := m.tester(agent)
	if err != nil {
		return environments.Status{}, err
	}
	if errs := m.privateErrors(o, o.store.Map()); len(errs) > 0 {
		return environments.Status{}, fmt.Errorf("ST1: fix your private documents first:\n%s", strings.Join(errs, "\n"))
	}
	plan, _, lock, _ := m.testerDocs(o)
	env, ok, err := m.environment(o)
	switch {
	case !ok:
		return environments.Status{}, fmt.Errorf("ST1: your scope has no %s", e2e.EnvironmentFile)
	case err != nil:
		return environments.Status{}, err
	}
	if findings := env.PlanFindings(plan, m.imageReleases(lock)); len(findings) > 0 {
		return environments.Status{}, fmt.Errorf("ST1: the environment does not match the plan:\n%s", strings.Join(findings, "\n"))
	}
	replicas, err := env.Replicas(plan, scenario)
	if err != nil {
		return environments.Status{}, err
	}
	in := e2e.OverrideInput{
		Network: m.environmentProject(hub), Images: map[string]string{}, Products: map[string]string{},
		Scope: o.store.Root(), Labels: map[string]string{},
	}
	if limits := m.topo.Agents[agent].E2E; limits != nil {
		in.CPUs, in.Memory = limits.CPUs, limits.Memory
	}
	var problems []string
	for _, r := range lock.Releases {
		key := r.System + "/" + r.Deliverable
		man, ok := m.release(r.System, r.Deliverable, r.Version)
		if !ok {
			continue
		}
		in.Products[key] = filepath.Join(m.store.ReleaseDir(r.System, r.Deliverable, r.Version), "product")
		if man.Image != nil {
			if id, _, err := images.Inspect(ctx, man.Image.Tag); err != nil || id != man.Image.ID {
				problems = append(problems, fmt.Sprintf("the image of %s %s is not in this machine's Docker as it was built; ask %s to publish a new version", key, r.Version, r.System))
			}
			in.Images[key] = man.Image.Tag
		}
	}
	for _, name := range env.Names() {
		for src := range env.Services[name].Mounts {
			if rel, ok := strings.CutPrefix(src, e2e.ScopePrefix); ok {
				if info, err := os.Stat(filepath.Join(o.store.Root(), filepath.FromSlash(rel))); err != nil || !info.IsDir() {
					problems = append(problems, fmt.Sprintf("the service %q mounts %s, which is not a directory of your scope", name, src))
				}
			}
		}
	}
	if len(problems) > 0 {
		return environments.Status{}, fmt.Errorf("ST1: %s", strings.Join(problems, "\n"))
	}
	m.envs.mu.Lock()
	if e := m.envs.by[hub]; e != nil && e.Active() {
		m.envs.mu.Unlock()
		return environments.Status{}, fmt.Errorf("ST1: the environment of #%s runs; call stop_environment first", hub)
	}
	if m.envs.run != "" {
		in.Labels["hikari.run"] = m.envs.run
	}
	in.Labels["hikari.environment"] = m.environmentProject(hub)
	m.envs.mu.Unlock()
	override, err := env.Override(in)
	if err != nil {
		return environments.Status{}, err
	}
	spec := environments.Spec{
		Project: m.environmentProject(hub), Network: m.environmentProject(hub),
		Dir: o.store.Root(), File: filepath.Join(o.store.Root(), e2e.EnvironmentFile), Override: override,
		OverrideFile: o.store.PrivatePath(filepath.Join(sysmap.EnvironmentDir, "override.yaml")),
		Replicas:     replicas, Scenario: scenario,
		Tester:       environments.Container(ctx, m.topo.Name, agent),
		LogDir:       o.store.PrivatePath(filepath.Join(sysmap.EnvironmentDir, "logs")),
		LogsShown:    path.Join(api.PrivateDir, sysmap.EnvironmentDir, "logs"),
		Minutes:      plan.Scenarios[scenario].Minutes,
		HostPlatform: images.HostPlatform(),
	}
	e := environments.Start(ctx, spec, func() { m.changed(Change{Wiki: api.HubWikiRef(hub)}) })
	m.envs.mu.Lock()
	m.envs.by[hub] = e
	m.envs.mu.Unlock()
	m.changed(Change{Wiki: api.HubWikiRef(hub)})
	return e.Status(ctx), nil
}

// StopEnvironment stops the environment of agent's hub (rule ST5).
func (m *Manager) StopEnvironment(ctx context.Context, agent string) (environments.Status, error) {
	_, hub, err := m.tester(agent)
	if err != nil {
		return environments.Status{}, err
	}
	return m.stopHub(ctx, hub, "the tester stopped it")
}

// StopHubEnvironment stops the environment of a hub, for the user (rule
// UE1).
func (m *Manager) StopHubEnvironment(ctx context.Context, hub string) (environments.Status, error) {
	if _, ok := m.topo.Hubs[hub]; !ok {
		return environments.Status{}, fmt.Errorf("%w: no hub %q", ErrNotFound, hub)
	}
	return m.stopHub(ctx, hub, "the user stopped it")
}

func (m *Manager) stopHub(ctx context.Context, hub, why string) (environments.Status, error) {
	m.envs.mu.Lock()
	e := m.envs.by[hub]
	m.envs.mu.Unlock()
	if e == nil || !e.Active() {
		return environments.Status{}, fmt.Errorf("%w: no environment of #%s runs", ErrNotFound, hub)
	}
	e.Stop(ctx, why)
	return e.Status(ctx), nil
}

// EnvironmentStatus returns the state of the environment of agent's hub
// (rule ST6).
func (m *Manager) EnvironmentStatus(ctx context.Context, agent string) (environments.Status, error) {
	_, hub, err := m.tester(agent)
	if err != nil {
		return environments.Status{}, err
	}
	return m.hubEnvironment(ctx, hub), nil
}

func (m *Manager) hubEnvironment(ctx context.Context, hub string) environments.Status {
	m.envs.mu.Lock()
	e := m.envs.by[hub]
	m.envs.mu.Unlock()
	if e == nil {
		return environments.Status{State: environments.None}
	}
	return e.Status(ctx)
}

// StopEnvironments stops every environment, when hikari stops (rule ST5).
func (m *Manager) StopEnvironments(ctx context.Context) {
	m.envs.mu.Lock()
	all := make([]*environments.Env, 0, len(m.envs.by))
	for _, e := range m.envs.by {
		all = append(all, e)
	}
	m.envs.mu.Unlock()
	for _, e := range all {
		if e.Active() {
			e.Stop(ctx, "hikari stopped")
		}
	}
}
