package server

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nathanaday/hikari/internal/assistant"
)

// quickClaude answers at once, with no permission prompt.
const quickClaude = `#!/bin/sh
read -r first
echo '{"type":"system","subtype":"init","session_id":"s9","slash_commands":["hikari:room"]}'
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"Hello from the panel."}]}}'
echo '{"type":"result","subtype":"success","is_error":false,"result":"Hello from the panel.","total_cost_usd":0.01,"session_id":"s9"}'
`

// TestAssistantRoutes checks rule HA9 of the hikari agent spec: the
// snapshot carries the hikari agent's view or the reason it is off, its
// changes are events, and its routes answer.
func TestAssistantRoutes(t *testing.T) {
	s, _ := newTestServer(t)
	srv := httptest.NewServer(s.Handler("secret"))
	defer srv.Close()
	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar}
	browser.Post(srv.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"secret"}`))
	post := func(path, body string) *http.Response {
		t.Helper()
		resp, err := browser.Post(srv.URL+path, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}

	s.SetAssistant(nil, "the hikari agent is off; pass -agent on to run it")
	if snap := s.snapshot(); snap.Assistant.Enabled || !strings.Contains(snap.Assistant.Reason, "off") || snap.Assistant.Messages == nil {
		t.Errorf("without an assistant: %+v", snap.Assistant)
	}
	if resp := post("/api/ui/assistant/send", `{"text":"hi"}`); resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("send without an assistant = %s", resp.Status)
	}

	dir := t.TempDir()
	cmd := filepath.Join(dir, "claude")
	os.WriteFile(cmd, []byte(quickClaude), 0o755)
	a, err := assistant.New(filepath.Join(dir, "assistant"), dir, "ctx", s.PublishAssistant)
	if err != nil {
		t.Fatal(err)
	}
	a.Command = cmd
	defer a.Close()
	s.SetAssistant(a, "")
	events, cancel := s.Subscribe()
	defer cancel()
	if e := <-events; !e.Snapshot.Assistant.Enabled || e.Snapshot.Assistant.State != assistant.StateIdle {
		t.Errorf("snapshot with an assistant: %+v", e.Snapshot.Assistant)
	}

	if resp := post("/api/ui/assistant/options", `{"model":"sonnet","effort":"low","permission_mode":"acceptEdits"}`); resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("options by POST = %s", resp.Status)
	}
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/ui/assistant/options", strings.NewReader(`{"model":"sonnet","effort":"low","permission_mode":"acceptEdits"}`))
	if resp, err := browser.Do(req); err != nil || resp.StatusCode != http.StatusNoContent {
		t.Errorf("options = %v %v", resp, err)
	}
	if resp := post("/api/ui/assistant/send", `{"text":"hello"}`); resp.StatusCode != http.StatusNoContent {
		t.Errorf("send = %s", resp.Status)
	}
	var types []string
	deadline := time.After(5 * time.Second)
	for done := false; !done; {
		select {
		case e := <-events:
			if e.Type != "assistant" {
				continue
			}
			types = append(types, e.Assistant.Type)
			done = e.Assistant.Type == "status" && e.Assistant.Status.State == assistant.StateIdle && e.Assistant.Status.Session == "s9"
		case <-deadline:
			t.Fatalf("the turn did not end; events %v", types)
		}
	}
	joined := strings.Join(types, " ")
	if !strings.HasPrefix(joined, "status message status") || !strings.Contains(joined, "activity") {
		t.Errorf("events = %v", types)
	}
	snap := s.snapshot()
	if snap.Assistant.Model != "sonnet" || snap.Assistant.PermissionMode != "acceptEdits" || snap.Assistant.Cost != 0.01 || len(snap.Assistant.Messages) != 1 {
		t.Errorf("snapshot after the turn: %+v", snap.Assistant.Status)
	}
	if resp := post("/api/ui/assistant/answer", `{"id":"nope","allow":true}`); resp.StatusCode != http.StatusNotFound {
		t.Errorf("answer of an unknown prompt = %s", resp.Status)
	}
	if resp := post("/api/ui/assistant/stop", ``); resp.StatusCode != http.StatusNoContent {
		t.Errorf("stop = %s", resp.Status)
	}
	if resp := post("/api/ui/assistant/new", ``); resp.StatusCode != http.StatusNoContent {
		t.Errorf("new = %s", resp.Status)
	}
	if snap := s.snapshot(); snap.Assistant.Session != "" {
		t.Errorf("after New chat: %+v", snap.Assistant.Status)
	}
	var view map[string]any
	data, _ := json.Marshal(s.snapshot().Assistant)
	json.Unmarshal(data, &view)
	for _, key := range []string{"enabled", "session", "model", "effort", "permission_mode", "state", "cost", "commands", "messages", "activity", "prompts"} {
		if _, ok := view[key]; !ok {
			t.Errorf("the view lacks %s: %s", key, data)
		}
	}
}
