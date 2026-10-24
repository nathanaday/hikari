package systems

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/e2e"
	"github.com/nathanaday/hikari/internal/images"
	"github.com/nathanaday/hikari/internal/testmap"
)

// needImage skips a test when Docker or the image it builds from is
// missing.
func needImage(t *testing.T, image string) {
	t.Helper()
	if err := exec.Command("docker", "image", "inspect", image).Run(); err != nil {
		t.Skipf("needs Docker with the image %s", image)
	}
}

func TestImageRelease(t *testing.T) {
	needImage(t, "hikari-agent")
	f := newFixture(t)
	defineDeliverable(t, f.dir, "python-library", pythonLibrary+"image: {dockerfile: Dockerfile}\n")
	write(t, f.repo, "services/orders/Dockerfile", "FROM hikari-agent\nCOPY create.py /app/create.py\n")
	f.describeOrders(t)
	f.runOrders(t, testmap.Passed)
	staging := filepath.Join(f.dir, "store", "systems", "orders", "staging", "python-library")
	write(t, staging, "README.md", "# orders\n")
	write(t, staging, "orders/create.py", "def create(): pass\n")
	_, err := f.m.MarkCurrent("orders")
	must(t, err)
	if _, err := f.m.CheckRelease("orders", "python-library"); err != nil {
		t.Fatalf("IM4: %v", err)
	}
	rel, _, err := f.m.PublishRelease("orders", api.PublishRequest{Deliverable: "python-library", Version: "1.0.0", Notes: "First."})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { exec.Command("docker", "image", "rm", "--force", rel.Image.Tag).Run() })
	if rel.Image == nil || rel.Image.Tag != images.Tag("test", "orders", "python-library", "1.0.0") || !strings.HasPrefix(rel.Image.ID, "sha256:") || rel.Image.Platform != images.HostPlatform() {
		t.Fatalf("IM3: image = %+v", rel.Image)
	}
	id, _, err := images.Inspect(context.Background(), rel.Image.Tag)
	if err != nil || id != rel.Image.ID {
		t.Errorf("IM2: the image is in Docker: %s, %v", id, err)
	}
	out, _ := exec.Command("docker", "run", "--rm", "--entrypoint", "cat", rel.Image.Tag, "/app/create.py").Output()
	if string(out) != "def create(): pass\n" {
		t.Errorf("IM2: the image holds the described commit's file: %q", out)
	}

	// A broken Dockerfile fails the publish and changes nothing.
	write(t, f.repo, "services/orders/Dockerfile", "FROM hikari-agent\nCOPY nowhere /x\n")
	write(t, staging, "README.md", "# orders\n")
	write(t, staging, "orders/create.py", "def create(): pass\n")
	_, err = f.m.MarkCurrent("orders")
	must(t, err)
	if _, _, err := f.m.PublishRelease("orders", api.PublishRequest{Deliverable: "python-library", Version: "1.0.1", Notes: "Broken."}); err == nil || !strings.Contains(err.Error(), "IM2") {
		t.Errorf("IM2: a failed build = %v", err)
	}
	if versions, _ := f.m.Store().Versions("orders", "python-library"); len(versions) != 1 {
		t.Errorf("IM2: a failed build published %v", versions)
	}
}

func TestEnvironment(t *testing.T) {
	needImage(t, "hikari-agent")
	f := newTesterFixture(t)
	defineDeliverable(t, f.dir, "python-library", pythonLibrary+"image: {dockerfile: Dockerfile}\n")
	write(t, f.repo, "services/orders/Dockerfile", "FROM hikari-agent\nCOPY create.py /app/create.py\n")
	f.describeOrders(t)
	f.publishOrders(t, "1.0.0")
	tag := images.Tag("test", "orders", "python-library", "1.0.0")
	t.Cleanup(func() { exec.Command("docker", "image", "rm", "--force", tag).Run() })
	_, err := f.m.PinReleases("e2e", []e2e.LockedRelease{{System: "orders", Deliverable: "python-library", Version: "1.0.0"}})
	must(t, err)
	write(t, f.repo, "e2e/.hikari/private/layout.yaml", "version: 1\nparts: [{path: tests, description: Tests.}]\nfeatures: {}\n")
	write(t, f.repo, "e2e/fixtures/readings/a.txt", "21.5\n")
	write(t, f.repo, "e2e/plan.md", `---
components:
  orders: {release: orders/python-library}
  reader: {simulates: a sensor}
actors:
  sensors: {runs: reader, count: 2}
scenarios:
  smoke: {stories: [order-items], actors: {sensors: 1}, minutes: 1}
---
`)
	if _, err := f.m.StartEnvironment(context.Background(), "e2e", "smoke"); err == nil || !strings.Contains(err.Error(), "no environment.yaml") {
		t.Errorf("ST1: no environment: %v", err)
	}
	write(t, f.repo, "e2e/environment.yaml", `services:
  orders:
    x-hikari: {release: orders/python-library}
    command: [sleep, "120"]
  reader:
    image: hikari-agent
    x-hikari:
      mount: {orders/python-library: /opt/orders, "scope:fixtures/readings": /data}
    command: [sh, -c, "cat /data/a.txt /opt/orders/README.md; sleep 120"]
`)
	f.story(t)
	ctx := context.Background()
	st, err := f.m.StartEnvironment(ctx, "e2e", "smoke")
	t.Cleanup(func() { f.m.StopEnvironments(context.Background()) })
	if err != nil || st.State != "running" {
		t.Fatalf("ST2: start = %+v, %v", st, err)
	}
	byName := map[string]int{}
	for _, s := range st.Services {
		byName[s.Name] = s.Replicas
	}
	if byName["orders"] != 1 || byName["reader"] != 2 || st.Logs != "/hikari/private/environment/logs" {
		t.Errorf("ST3, EN5: services = %+v", st.Services)
	}
	if _, err := f.m.StartEnvironment(ctx, "e2e", "smoke"); err == nil || !strings.Contains(err.Error(), "runs") {
		t.Errorf("ST1: a second start = %v", err)
	}
	// The override holds this machine's paths, so git never sees it.
	if out := git(t, f.repo, "status", "--porcelain", "--untracked-files=all", "--", "e2e"); strings.Contains(out, "override") {
		t.Errorf("the override is in the scope's versioned files:\n%s", out)
	}
	p, _ := f.m.GetTestPlan("eng-lead", "eng")
	if p.Environment == nil || p.Environment.State != "running" {
		t.Errorf("UE2: the plan's environment = %+v", p.Environment)
	}
	logFile := filepath.Join(f.repo, "e2e", ".hikari", "private", "environment", "logs", "reader.log")
	var log []byte
	for range 50 {
		if log, _ = os.ReadFile(logFile); strings.Contains(string(log), "# orders 1.0.0") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !strings.Contains(string(log), "21.5") || !strings.Contains(string(log), "# orders 1.0.0") {
		t.Errorf("LG1, EN2: the reader's log =\n%s", log)
	}
	st, err = f.m.StopEnvironment(ctx, "e2e")
	if err != nil || st.State != "stopped" {
		t.Errorf("ST5: stop = %+v, %v", st, err)
	}
	if out, _ := exec.Command("docker", "ps", "-aq", "--filter", "label=hikari.environment=test-e2e-eng").Output(); len(strings.TrimSpace(string(out))) > 0 {
		t.Errorf("ST5: containers are left: %s", out)
	}
	if _, err := f.m.StopEnvironment(ctx, "e2e"); !errors.Is(err, ErrNotFound) {
		t.Errorf("ST5: a second stop = %v", err)
	}
}

func TestEnvironmentThatFailsSaysWhy(t *testing.T) {
	needImage(t, "hikari-agent")
	f := newTesterFixture(t)
	f.describeOrders(t)
	f.story(t)
	write(t, f.repo, "e2e/.hikari/private/layout.yaml", "version: 1\nparts: [{path: tests, description: Tests.}]\nfeatures: {}\n")
	write(t, f.repo, "e2e/plan.md", "---\ncomponents:\n  sick: {image: hikari-agent}\nscenarios:\n  smoke: {stories: [order-items]}\n---\n")
	write(t, f.repo, "e2e/environment.yaml", `services:
  sick:
    image: hikari-agent
    command: [sh, -c, "echo starting up; sleep 120"]
    healthcheck: {test: [CMD, sh, -c, "echo the port is closed; exit 3"], interval: 1s, retries: 2}
`)
	st, err := f.m.StartEnvironment(context.Background(), "e2e", "smoke")
	t.Cleanup(func() { f.m.StopEnvironments(context.Background()) })
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "failed" || !strings.Contains(st.Reason, "the health check of test-e2e-eng-sick-1 exited 3: the port is closed") {
		t.Errorf("a failed start says why: %+v", st)
	}
	if log, _ := os.ReadFile(filepath.Join(f.repo, "e2e", ".hikari", "private", "environment", "logs", "sick.log")); !strings.Contains(string(log), "starting up") {
		t.Errorf("LG1: the log of a failed start is kept: %q", log)
	}
	if out, _ := exec.Command("docker", "ps", "-aq", "--filter", "label=hikari.environment=test-e2e-eng").Output(); len(strings.TrimSpace(string(out))) > 0 {
		t.Errorf("a failed start leaves containers: %s", out)
	}
}
