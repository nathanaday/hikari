package assistant

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nathanaday/hikari/internal/api"
)

// fakeClaude is a claude that reads the user message, asks one permission,
// and ends with a result. It writes its arguments to FAKE_ARGS and each
// line it reads after the first to FAKE_IN.
const fakeClaude = `#!/bin/sh
echo "$*" > "$FAKE_ARGS"
read -r first
echo '{"type":"system","subtype":"init","session_id":"s1","slash_commands":["threads:run","hikari:hikari"]}'
echo '{"type":"assistant","session_id":"s1","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}]}}'
echo '{"type":"control_request","request_id":"r-'$$'","request":{"subtype":"can_use_tool","tool_name":"Bash","input":{"command":"ls"},"description":"List the files"}}'
read -r answer
echo "$answer" >> "$FAKE_IN"
echo '{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"a.go","is_error":false}]}}'
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"One file: a.go."}]}}'
echo '{"type":"result","subtype":"success","is_error":false,"result":"One file: a.go.","total_cost_usd":0.02,"session_id":"s1"}'
`

// slowClaude starts a turn and never ends it.
const slowClaude = `#!/bin/sh
read -r first
echo '{"type":"system","subtype":"init","session_id":"s2"}'
sleep 30
`

type recorder struct {
	mu     sync.Mutex
	events []Event
}

func (r *recorder) listen(e Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *recorder) types() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, e := range r.events {
		out = append(out, e.Type)
	}
	return out
}

func fake(t *testing.T, script string) (*Assistant, *recorder, string) {
	t.Helper()
	dir := t.TempDir()
	cmd := filepath.Join(dir, "claude")
	if err := os.WriteFile(cmd, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_ARGS", filepath.Join(dir, "args"))
	t.Setenv("FAKE_IN", filepath.Join(dir, "in"))
	rec := &recorder{}
	a, err := New(filepath.Join(dir, "assistant"), dir, "the context", rec.listen)
	if err != nil {
		t.Fatal(err)
	}
	a.Command = cmd
	t.Cleanup(a.Close)
	return a, rec, dir
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for start := time.Now(); time.Since(start) < 5*time.Second; time.Sleep(10 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func (a *Assistant) pendingPrompt() *Prompt {
	v := a.View()
	for _, p := range v.Prompts {
		if p.Answer == "" {
			return &p
		}
	}
	return nil
}

// TestTurn checks rules HA3, HA5, HA7, and HA8 of the hikari agent spec: a
// turn against a fake claude, with its prompt answered from the panel, its
// session and commands kept, and its transcript and session.json written.
func TestTurn(t *testing.T) {
	a, rec, dir := fake(t, fakeClaude)
	if err := a.Send("  list the files  "); err != nil {
		t.Fatal(err)
	}
	if err := a.Send("again"); err == nil || !strings.Contains(err.Error(), "working") {
		t.Errorf("a send during a turn must be refused: %v", err)
	}
	waitFor(t, "the prompt", func() bool { return a.pendingPrompt() != nil })
	p := a.pendingPrompt()
	if p.Tool != "Bash" || string(p.Input) != `{"command":"ls"}` || p.Description != "List the files" || p.Turn != 1 {
		t.Errorf("prompt = %+v", p)
	}
	if err := a.Answer("nope", true, ""); err == nil {
		t.Error("an unknown prompt must be refused")
	}
	if err := a.Answer(p.ID, true, ""); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the turn to end", func() bool { return a.View().State == StateIdle })

	v := a.View()
	if v.Session != "s1" || v.Cost != 0.02 || strings.Join(v.Commands, " ") != "hikari:hikari threads:run" || v.PermissionMode != ModeAsk {
		t.Errorf("status = %+v", v.Status)
	}
	if len(v.Messages) != 1 || v.Messages[0].Text != "list the files" || v.Messages[0].ID != 1 {
		t.Errorf("messages = %+v", v.Messages)
	}
	kinds := make([]string, len(v.Activity))
	for i, e := range v.Activity {
		kinds[i] = e.Kind
	}
	if want := "turn tool text done"; strings.Join(kinds, " ") != want {
		t.Errorf("activity kinds = %v, want %s", kinds, want)
	}
	last := v.Activity[len(v.Activity)-1]
	if last.Text != "One file: a.go." || last.Cost != 0.02 || last.Error {
		t.Errorf("done entry = %+v", last)
	}
	if v.Prompts[0].Answer != "allow" {
		t.Errorf("the prompt keeps its answer: %+v", v.Prompts[0])
	}

	in, _ := os.ReadFile(filepath.Join(dir, "in"))
	var resp struct {
		Type     string `json:"type"`
		Response struct {
			Subtype   string `json:"subtype"`
			RequestID string `json:"request_id"`
			Response  struct {
				Behavior     string          `json:"behavior"`
				UpdatedInput json.RawMessage `json:"updatedInput"`
			} `json:"response"`
		} `json:"response"`
	}
	if err := json.Unmarshal(in, &resp); err != nil || resp.Type != "control_response" || resp.Response.RequestID != p.ID || resp.Response.Response.Behavior != "allow" || string(resp.Response.Response.UpdatedInput) != `{"command":"ls"}` {
		t.Errorf("control response = %s (%v)", in, err)
	}
	args, _ := os.ReadFile(filepath.Join(dir, "args"))
	for _, want := range []string{"-p --input-format stream-json --output-format stream-json --verbose --include-partial-messages --permission-prompt-tool stdio --permission-mode default", "--append-system-prompt the context"} {
		if !strings.Contains(string(args), want) {
			t.Errorf("args = %s, want %s", args, want)
		}
	}
	if strings.Contains(string(args), "--resume") || strings.Contains(string(args), "--model") {
		t.Errorf("a first turn with no model names no session and no model: %s", args)
	}

	types := rec.types()
	if types[0] != "message" || types[1] != "status" || !strings.Contains(strings.Join(types, " "), "prompt") || types[len(types)-1] != "status" {
		t.Errorf("events = %v", types)
	}

	var sess session
	data, err := os.ReadFile(filepath.Join(dir, "assistant", "session.json"))
	if err != nil || json.Unmarshal(data, &sess) != nil || sess.Session != "s1" || sess.Cost != 0.02 {
		t.Errorf("session.json = %s (%v)", data, err)
	}
	transcript, _ := os.ReadFile(filepath.Join(dir, "assistant", "s1.jsonl"))
	lines := strings.Split(strings.TrimSpace(string(transcript)), "\n")
	if lines[0] == "" || !strings.Contains(lines[0], `"kind":"message"`) || !strings.Contains(string(transcript), `"kind":"prompt"`) || !strings.Contains(string(transcript), `"answer":"allow"`) {
		t.Errorf("transcript:\n%s", transcript)
	}

	// The second turn resumes the session, with the options set.
	if err := a.SetOptions(Options{Model: "opus", Effort: "high", PermissionMode: ModeBypass}); err != nil {
		t.Fatal(err)
	}
	if err := a.SetOptions(Options{PermissionMode: "dontAsk"}); err == nil {
		t.Error("an unknown permission mode must be refused")
	}
	if err := a.Send("and again"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the second prompt", func() bool { return a.pendingPrompt() != nil })
	a.Answer(a.pendingPrompt().ID, false, "")
	waitFor(t, "the second turn to end", func() bool { return a.View().State == StateIdle })
	args, _ = os.ReadFile(filepath.Join(dir, "args"))
	for _, want := range []string{"--resume s1", "--model opus", "--effort high", "--permission-mode bypassPermissions --dangerously-skip-permissions"} {
		if !strings.Contains(string(args), want) {
			t.Errorf("args = %s, want %s", args, want)
		}
	}
	v = a.View()
	if v.Cost != 0.04 || len(v.Messages) != 2 || v.Prompts[1].Answer != "deny" || v.Prompts[1].Message == "" {
		t.Errorf("after the second turn: %+v %+v", v.Status, v.Prompts)
	}

	// A new assistant on the same folder loads the session (rule HA2).
	b, err := New(filepath.Join(dir, "assistant"), dir, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	bv := b.View()
	if bv.Session != "s1" || bv.Model != "opus" || len(bv.Messages) != 2 || len(bv.Activity) != len(v.Activity) || len(bv.Prompts) != 2 || bv.Prompts[0].Answer != "allow" {
		t.Errorf("loaded view: %+v, %d messages, %d entries, %+v", bv.Status, len(bv.Messages), len(bv.Activity), bv.Prompts)
	}
	if err := b.NewChat(); err != nil {
		t.Fatal(err)
	}
	bv = b.View()
	if bv.Session != "" || len(bv.Messages) != 0 || len(bv.Activity) != 0 || bv.Cost != 0 || bv.Model != "opus" {
		t.Errorf("after New chat: %+v", bv)
	}
}

// TestStop checks rule HA6: Stop ends a running turn with the error
// "stopped", and a pending prompt is denied.
func TestStop(t *testing.T) {
	a, _, _ := fake(t, slowClaude)
	if err := a.Send("wait"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the session", func() bool { return a.View().Session == "s2" })
	a.Stop()
	v := a.View()
	last := v.Activity[len(v.Activity)-1]
	if v.State != StateIdle || last.Kind != api.ActivityDone || !last.Error || last.Text != "stopped" {
		t.Errorf("after Stop: %+v, last %+v", v.Status, last)
	}
	if err := a.Send("again"); err != nil {
		t.Errorf("a send after Stop runs: %v", err)
	}
	a.Stop()
}

func TestContextAndEnabled(t *testing.T) {
	c := Context("shop", "/p/shop", "/p/shop/.hikari/runs/1", "http://127.0.0.1:7420/?token=x")
	for _, want := range []string{"hikari agent", "shop", "/p/shop", "feed.jsonl", "messages.jsonl", "http://127.0.0.1:7420/?token=x", "closing text", "hikari send"} {
		if !strings.Contains(c, want) {
			t.Errorf("context lacks %q", want)
		}
	}
	t.Setenv("PATH", t.TempDir())
	if ok, reason := Enabled("auto", "127.0.0.1"); ok || !strings.Contains(reason, "claude is not on the PATH") {
		t.Errorf("without claude: %v %q", ok, reason)
	}
	if ok, reason := Enabled("auto", "0.0.0.0"); ok || !strings.Contains(reason, "outside this machine") {
		t.Errorf("outside: %v %q", ok, reason)
	}
	if ok, reason := Enabled("off", "127.0.0.1"); ok || !strings.Contains(reason, "off") {
		t.Errorf("off: %v %q", ok, reason)
	}
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\n"), 0o755)
	t.Setenv("PATH", bin)
	if ok, _ := Enabled("on", "0.0.0.0"); !ok {
		t.Error("on overrides the listener")
	}
	if ok, _ := Enabled("auto", "localhost"); !ok {
		t.Error("auto on a loopback listener with claude")
	}
}
