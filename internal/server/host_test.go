package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nathanaday/hikari/internal/editor"
	"github.com/nathanaday/hikari/internal/topoedit"
	"github.com/nathanaday/hikari/internal/topology"
)

// projectCompose is a project of one orchestrator, in a project
// repository.
const projectCompose = `# The demo project.
name: demo

x-hikari:
  project: {branch: main}
  orchestration: lead

services:
  lead:
    image: hikari-agent
    environment: [CLAUDE_CODE_OAUTH_TOKEN]
    x-hikari: {description: Leads the demo.}
`

func gitEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR", "GIT_COMMITTER"} {
		t.Setenv(k+"_NAME", "Test User")
		t.Setenv(k+"_EMAIL", "user@example.com")
	}
}

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func needCompose(t *testing.T) {
	t.Helper()
	if exec.Command("docker", "compose", "version").Run() != nil {
		t.Skip("docker compose is not available")
	}
}

// hostClient serves a host on a test server, and returns a signed-in
// client and the base URL.
func hostClient(t *testing.T, file string) (*Host, *http.Client, string) {
	t.Helper()
	ed, err := editor.New(file)
	if err != nil {
		t.Fatal(err)
	}
	h := NewHost("secret", ed)
	ts := httptest.NewServer(h.Handler())
	t.Cleanup(ts.Close)
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	res, err := c.Post(ts.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"secret"}`))
	if err != nil || res.StatusCode != http.StatusNoContent {
		t.Fatalf("login: %v %v", res, err)
	}
	return h, c, ts.URL
}

func call(t *testing.T, c *http.Client, method, url string, body any, out any) int {
	t.Helper()
	var r io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		r = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, url, r)
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("%s %s: %v: %s", method, url, err, data)
		}
	}
	return res.StatusCode
}

func newProjectDir(t *testing.T) (string, string) {
	t.Helper()
	gitEnv(t)
	// The fixtures must not depend on the machine's credentials.
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")
	dir := t.TempDir()
	file := filepath.Join(dir, "compose.yaml")
	os.WriteFile(file, []byte(projectCompose), 0o644)
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".env\n/.hikari/\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "store", "shared", "wiki"), 0o755)
	os.WriteFile(filepath.Join(dir, "store", "shared", "wiki", "plan.md"), []byte("---\nkind: plan\nstatus: draft\n---\nShip it soon, with @lead.\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "app"), 0o755)
	os.WriteFile(filepath.Join(dir, "app", "main.py"), []byte("x\n"), 0o644)
	run(t, dir, "init", "--quiet", "--initial-branch=main")
	run(t, dir, "add", "--all")
	run(t, dir, "commit", "--quiet", "-m", "start")
	return dir, file
}

func TestTopologyAPI(t *testing.T) {
	needCompose(t)
	dir, file := newProjectDir(t)
	h, c, base := hostClient(t, file)

	var state editor.State
	if code := call(t, c, "GET", base+"/api/ui/topology", nil, &state); code != 200 {
		t.Fatalf("GET topology: %d", code)
	}
	if !state.Exists || state.Revision != editor.Revision([]byte(projectCompose), true) || state.Mode != ModeEditing || !state.Commits ||
		state.Model == nil || state.Model.Project.Orchestration != "lead" || state.Text != projectCompose {
		t.Errorf("TA1, TM1: %+v", state)
	}
	if editor.HasError(state.Findings) {
		t.Errorf("TL1: a valid file has no error: %+v", state.Findings)
	}
	writeFixture(t, "state", state)

	// The cookie guards every endpoint; the run's endpoints wait for a run.
	if res, _ := http.Get(base + "/api/ui/topology"); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("the cookie: %d", res.StatusCode)
	}
	if code := call(t, c, "POST", base+"/api/ui/send", map[string]string{"body": "hi"}, nil); code != http.StatusServiceUnavailable {
		t.Errorf("UP3: %d", code)
	}

	ops := []topoedit.Op{
		{Op: "rename", Kind: "agent", ID: "lead", Name: ptrTo("chief")},
		{Op: "add_agent", ID: "dev", Template: "lead", Description: ptrTo("Builds it.")},
		{Op: "set_system", ID: "dev", Repo: ptrTo("project"), Scope: ptrTo("app")},
	}
	var plan editor.Plan
	if code := call(t, c, "POST", base+"/api/ui/topology/plan", map[string]any{"revision": state.Revision, "operations": ops}, &plan); code != 200 {
		t.Fatalf("plan: %d %+v", code, plan)
	}
	if plan.AppliesAtStart || len(plan.Summary) != 3 || editor.HasError(plan.Findings) || !strings.Contains(plan.Text, "name: chief") ||
		plan.Model == nil || plan.Model.Agents[1].Name != "chief" || len(plan.Occurrences) != 1 || plan.Occurrences[0].Key != "shared/wiki/plan.md:5:20" {
		t.Errorf("plan: %+v", plan)
	}
	added := 0
	for _, l := range plan.Diff {
		if l.Op == "+" {
			added++
		}
	}
	if added == 0 {
		t.Errorf("TA3: %+v", plan.Diff)
	}
	if b, _ := os.ReadFile(file); string(b) != projectCompose {
		t.Error("a plan writes nothing")
	}
	writeFixture(t, "plan", plan)

	var conflict map[string]string
	if code := call(t, c, "POST", base+"/api/ui/topology/apply", map[string]any{"revision": "stale", "operations": ops}, &conflict); code != http.StatusConflict || conflict["text"] != projectCompose {
		t.Errorf("TA2: %d %v", code, conflict)
	}
	var refused map[string]string
	if code := call(t, c, "POST", base+"/api/ui/topology/plan", map[string]any{"revision": state.Revision, "operations": []topoedit.Op{{Op: "rename", Kind: "agent", ID: "lead", Name: ptrTo("Chief")}}}, &refused); code != http.StatusBadRequest || !strings.Contains(refused["error"], "not a valid name") {
		t.Errorf("TA9: %d %v", code, refused)
	}
	if code := call(t, c, "POST", base+"/api/ui/topology/plan", map[string]any{"revision": state.Revision, "operations": []topoedit.Op{{Op: "set_agent", ID: "ghost"}}}, &refused); code != http.StatusNotFound {
		t.Errorf("TA9: %d %v", code, refused)
	}

	var saved editor.Saved
	if code := call(t, c, "POST", base+"/api/ui/topology/apply", map[string]any{"revision": state.Revision, "operations": ops}, &saved); code != 200 {
		t.Fatalf("apply: %d %+v", code, saved)
	}
	data, _ := os.ReadFile(file)
	if saved.Revision != editor.Revision(data, true) || saved.Commit == "" || string(data) != plan.Text {
		t.Errorf("apply: %+v\n%s", saved, data)
	}
	if got := run(t, dir, "log", "-1", "--format=%an|%s"); got != "Test User|user: rename agent lead to chief; add agent dev; give dev a system" {
		t.Errorf("TC1, TC2: %q", got)
	}
	if run(t, dir, "status", "--porcelain") != "" {
		t.Error("TC1: the save leaves the repository clean")
	}

	// A broken text is your file: it saves, and lint says why.
	broken := "name: demo\nservices:\n  lead: [oops\n"
	if code := call(t, c, "PUT", base+"/api/ui/topology/source", map[string]any{"revision": saved.Revision, "text": broken}, &saved); code != 200 {
		t.Fatalf("TA5: %d", code)
	}
	call(t, c, "GET", base+"/api/ui/topology", nil, &state)
	if state.Model != nil || len(state.Findings) != 1 || state.Findings[0].Stage != topology.StageYAML || state.Findings[0].Line == 0 {
		t.Errorf("TM4, TF4: %+v", state)
	}
	if got := run(t, dir, "log", "-1", "--format=%s"); got != "user: edit compose.yaml" {
		t.Errorf("TC2: %q", got)
	}
	var startRefused struct {
		Findings []topology.Finding `json:"findings"`
	}
	h.Start = func() { t.Error("a broken file must not start") }
	if code := call(t, c, "POST", base+"/api/ui/run/start", nil, &startRefused); code != http.StatusBadRequest || len(startRefused.Findings) == 0 {
		t.Errorf("UP4: %d %+v", code, startRefused)
	}
	var lint struct {
		Findings []topology.Finding `json:"findings"`
	}
	call(t, c, "POST", base+"/api/ui/topology/lint", map[string]string{"text": projectCompose}, &lint)
	if editor.HasError(lint.Findings) {
		t.Errorf("lint: %+v", lint.Findings)
	}
	call(t, c, "POST", base+"/api/ui/topology/lint", map[string]string{"text": strings.Replace(projectCompose, "orchestration: lead", "orchestration: ghost", 1)}, &lint)
	found := false
	for _, f := range lint.Findings {
		found = found || (f.Path == "x-hikari.orchestration" && f.Line == 6)
	}
	if !found {
		t.Errorf("TF1 to TF3: %+v", lint.Findings)
	}
}

func TestStartAndModes(t *testing.T) {
	needCompose(t)
	_, file := newProjectDir(t)
	h, c, base := hostClient(t, file)
	started := make(chan struct{})
	h.Start = func() { close(started) }
	if code := call(t, c, "POST", base+"/api/ui/run/start", nil, nil); code != http.StatusAccepted {
		t.Fatalf("UP4: %d", code)
	}
	<-started
	if h.Mode() != ModeStarting {
		t.Errorf("mode %s", h.Mode())
	}
	if code := call(t, c, "POST", base+"/api/ui/run/start", nil, nil); code != http.StatusConflict {
		t.Errorf("a second start: %d", code)
	}
	h.Editing([]topology.Finding{{Severity: "error", Stage: "start", Message: "a repo has uncommitted changes"}})

	// The editing stream starts with a snapshot, and follows the file.
	req, _ := http.NewRequest("GET", base+"/api/ui/events", nil)
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	events := bufio.NewScanner(res.Body)
	next := func() Event {
		for events.Scan() {
			if line, ok := strings.CutPrefix(events.Text(), "data: "); ok {
				var e Event
				json.Unmarshal([]byte(line), &e)
				return e
			}
		}
		return Event{}
	}
	e := next()
	if e.Type != "snapshot" || e.Snapshot.Mode != ModeEditing || e.Snapshot.Revision == "" || len(e.Snapshot.StartFindings) != 1 || e.Snapshot.Agents == nil {
		t.Errorf("UP9: %+v", e.Snapshot)
	}
	h.TopologyChanged("abc")
	if e := next(); e.Type != "topology" || e.Revision != "abc" {
		t.Errorf("TA8: %+v", e)
	}
	done := make(chan bool)
	go func() {
		for events.Scan() {
		}
		done <- true
	}()
	h.Editing(nil)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Error("decision 7: a change of mode closes the stream")
	}
}

func TestCreate(t *testing.T) {
	needCompose(t)
	gitEnv(t)
	dir := filepath.Join(t.TempDir(), "My Project")
	// hikari up writes its log here before the project exists.
	os.MkdirAll(filepath.Join(dir, ".hikari"), 0o755)
	_, c, base := hostClient(t, filepath.Join(dir, "compose.yaml"))
	var state editor.State
	call(t, c, "GET", base+"/api/ui/topology", nil, &state)
	if state.Exists || state.Revision != "" || state.Model != nil {
		t.Errorf("a missing file: %+v", state)
	}
	var saved editor.Saved
	if code := call(t, c, "POST", base+"/api/ui/topology/create", nil, &saved); code != 200 || saved.Revision == "" {
		t.Fatalf("TA6: %d %+v", code, saved)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Errorf("TA6: an empty directory becomes a project repository: %v", err)
	}
	call(t, c, "GET", base+"/api/ui/topology", nil, &state)
	if state.Model == nil || state.Model.Project.ID != "my-project" || !state.Model.Project.Repo || editor.HasError(state.Findings) {
		t.Errorf("TA6: %+v", state)
	}
	if code := call(t, c, "POST", base+"/api/ui/topology/create", nil, nil); code != http.StatusBadRequest {
		t.Errorf("TA6: the file exists: %d", code)
	}

	other := t.TempDir()
	os.WriteFile(filepath.Join(other, "README.md"), []byte("# mine\n"), 0o644)
	_, c, base = hostClient(t, filepath.Join(other, "compose.yaml"))
	call(t, c, "POST", base+"/api/ui/topology/create", nil, &saved)
	call(t, c, "GET", base+"/api/ui/topology", nil, &state)
	if state.Model == nil || state.Model.Project.Repo || editor.HasError(state.Findings) {
		t.Errorf("TA6: a directory with files gets a compose file alone: %+v", state)
	}
	var options editor.Options
	call(t, c, "GET", base+"/api/ui/topology/options", nil, &options)
	if len(options.Models) != 3 || len(options.Templates) != 1 {
		t.Errorf("TA7: %+v", options)
	}
}

func ptrTo(s string) *string { return &s }

var tempPath = regexp.MustCompile(`(/private)?/var/folders/[^"]*?/(compose\.yaml)|/tmp/[^"]*?/(compose\.yaml)`)

// writeFixture keeps a response of the topology API for the UI's tests
// (decision 5 of the topology API spec). With HIKARI_UPDATE_FIXTURES set,
// it writes the file; otherwise it fails when the file is out of date.
func writeFixture(t *testing.T, name string, v any) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(tempPath.ReplaceAll(data, []byte("compose.yaml")), '\n')
	path := filepath.Join("..", "..", "web", "src", "lib", "fixtures", "topology", name+".json")
	if os.Getenv("HIKARI_UPDATE_FIXTURES") != "" {
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	old, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(old, data) {
		t.Errorf("the fixture %s is out of date; run HIKARI_UPDATE_FIXTURES=1 go test ./internal/server", path)
	}
}
