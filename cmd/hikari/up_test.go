package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/console"
	"github.com/nathanaday/hikari/internal/editor"
	"github.com/nathanaday/hikari/internal/onboard"
	"github.com/nathanaday/hikari/internal/scaffold"
	"github.com/nathanaday/hikari/internal/server"
)

// newRunner makes a runner of file with -no-launch, which starts no
// container, and a host.
func newRunner(t *testing.T, file string) (*runner, *server.Host, string) {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "console")
	if err != nil {
		t.Fatal(err)
	}
	ed, err := editor.New(file)
	if err != nil {
		t.Fatal(err)
	}
	h := server.NewHost("secret", ed)
	r := &runner{ctx: t.Context(), con: console.New(out), file: ed.File, listen: "127.0.0.1:0", port: "0", noLaunch: true, url: "http://x", host: h}
	h.Start, h.Stop = r.start, func() { r.stop(false) }
	return r, h, out.Name()
}

func TestRunnerEditingStates(t *testing.T) {
	if exec.Command("docker", "compose", "version").Run() != nil {
		t.Skip("docker compose is not available")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	file := filepath.Join(dir, "topology.yaml")

	r, h, _ := newRunner(t, file)
	r.start()
	if h.Mode() != server.ModeEditing || r.cur != nil {
		t.Errorf("UP2: a missing file waits in the editing state: %s", h.Mode())
	}

	os.WriteFile(file, []byte("name: demo\nx-hikari: {orchestration: ghost}\nservices: {}\n"), 0o644)
	r.start()
	if h.Mode() != server.ModeEditing || r.cur != nil {
		t.Errorf("UP2: a broken file waits in the editing state: %s", h.Mode())
	}
}

func TestRunnerStartAndStop(t *testing.T) {
	if exec.Command("docker", "compose", "version").Run() != nil {
		t.Skip("docker compose is not available")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR", "GIT_COMMITTER"} {
		t.Setenv(k+"_NAME", "Test User")
		t.Setenv(k+"_EMAIL", "user@example.com")
	}
	dir := filepath.Join(t.TempDir(), "demo")
	if _, err := scaffold.WriteProject(dir, "demo", onboard.Write); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "--all"}, {"commit", "--quiet", "-m", "start"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	t.Chdir(dir)
	r, h, log := newRunner(t, filepath.Join(dir, "topology.yaml"))
	r.start()
	if h.Mode() != server.ModeRunning || r.cur == nil {
		data, _ := os.ReadFile(log)
		t.Fatalf("UP2: a valid file starts at once: %s\n%s", h.Mode(), data)
	}
	first := r.cur.dir
	r.stop(false)
	if h.Mode() != server.ModeEditing || r.cur != nil {
		t.Errorf("UP6: stop returns to the editing state: %s", h.Mode())
	}
	r.start()
	if r.cur == nil || r.cur.dir == first || !strings.HasPrefix(r.cur.dir, filepath.Join(".hikari", "runs")) {
		t.Errorf("UP7: each start is a new run: %v", r.cur)
	}
	r.stop(true)
	if h.Mode() == server.ModeEditing {
		t.Error("UP10: a stop on exit does not return to editing")
	}
}
