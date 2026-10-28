package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/server"
)

// TestPathEntry checks the PATH entry of CP10 of the plugin spec.
func TestPathEntry(t *testing.T) {
	files := map[string]string{"/etc/profile.d/hikari.sh": `export PATH="/home/a/.hikari/bin:$PATH"` + "\n"}
	read := func(p string) ([]byte, error) {
		if s, ok := files[p]; ok {
			return []byte(s), nil
		}
		return nil, os.ErrNotExist
	}
	if ok, _ := pathEntry("/home/a/.hikari/bin", "linux", "/usr/bin", read); !ok {
		t.Error("the entry of install.sh puts the folder on the PATH of new shells")
	}
	ok, fix := pathEntry("/home/b/.hikari/bin", "linux", "/usr/bin", read)
	if ok || fix != `echo 'export PATH="/home/b/.hikari/bin:$PATH"' | sudo tee /etc/profile.d/hikari.sh` {
		t.Errorf("a missing entry gives the line that writes it: %v %q", ok, fix)
	}
	if ok, _ := pathEntry("/home/b/.hikari/bin", "linux", "/usr/bin:/home/b/.hikari/bin", read); !ok {
		t.Error("a folder already on PATH is on it")
	}
	if _, fix := pathEntry("/Users/c/.hikari/bin", "darwin", "", read); fix != "echo /Users/c/.hikari/bin | sudo tee /etc/paths.d/hikari" {
		t.Errorf("on macOS, the entry is in /etc/paths.d: %q", fix)
	}
}

// TestSend checks CP11 and CP12 of the plugin spec: hikari send posts as
// you through the server that host.json names, with the UI's token.
func TestSend(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "topology.yaml")
	if _, err := sendMessage(t.Context(), file, "", "hi"); err == nil || !strings.Contains(err.Error(), "hikari is not running for") || !strings.Contains(err.Error(), "hikari up") {
		t.Errorf("CP12 with no host.json, hikari is not running: %v", err)
	}
	var got api.SendRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(server.UICookie); err != nil || c.Value != "secret" || r.URL.Path != "/api/ui/send" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "open the URL that hikari printed at startup"})
			return
		}
		json.NewDecoder(r.Body).Decode(&got)
		json.NewEncoder(w).Encode(api.Message{From: "you", To: got.To, Body: got.Body, Time: time.Now()})
	}))
	defer srv.Close()

	if err := writeHost(file, hostFile{URL: srv.URL, Token: "secret", PID: os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(hostPath(file)); st.Mode().Perm() != 0o600 {
		t.Errorf("CP11 host.json is readable only by you: %v", st.Mode())
	}
	m, err := sendMessage(t.Context(), file, "orchestrator", "Please carry out the plan.")
	if err != nil || got.To != "orchestrator" || got.Body != "Please carry out the plan." || m.To != "orchestrator" {
		t.Errorf("CP12 a direct message reaches the server: %+v, %v", got, err)
	}

	writeHost(file, hostFile{URL: srv.URL, Token: "wrong", PID: os.Getpid()})
	if _, err := sendMessage(t.Context(), file, "", "hi"); err == nil || !strings.Contains(err.Error(), "open the URL") {
		t.Errorf("the server's error is shown: %v", err)
	}

	writeHost(file, hostFile{URL: srv.URL, Token: "secret", PID: 1 << 22})
	if _, err := sendMessage(t.Context(), file, "", "hi"); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Errorf("CP12 a dead process is not running: %v", err)
	}

	writeHost(file, hostFile{URL: srv.URL, Token: "secret", PID: os.Getpid()})
	removeHost(file)
	if _, err := os.Stat(hostPath(file)); !errors.Is(err, os.ErrNotExist) {
		t.Error("CP11 hikari up removes its host.json at stop")
	}
}

// TestSessionStart checks CP14 of the plugin spec.
func TestSessionStart(t *testing.T) {
	dir := t.TempDir()
	if got := sessionStart(dir); got != "" {
		t.Errorf("nothing outside a project: %q", got)
	}
	os.WriteFile(filepath.Join(dir, "topology.yaml"), []byte("name: plain\nservices: {}\n"), 0o644)
	if got := sessionStart(dir); got != "" {
		t.Errorf("nothing for a Compose file without x-hikari: %q", got)
	}
	os.WriteFile(filepath.Join(dir, "topology.yaml"), []byte("name: shop\nx-hikari: {orchestration: orchestrator}\nservices: {}\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "onboarding"), 0o755)
	os.WriteFile(filepath.Join(dir, "onboarding", "state.md"), []byte("# Onboarding\n\nStage: hikify: review\n"), 0o644)
	for id, stage := range map[string]string{"2026-11-02-split-api": "change: plan", "2026-10-30-units": "change: done"} {
		os.MkdirAll(filepath.Join(dir, "changes", id), 0o755)
		os.WriteFile(filepath.Join(dir, "changes", id, "state.md"), []byte("Stage: "+stage+"\n"), 0o644)
	}
	got := sessionStart(dir)
	for _, want := range []string{"the hikari project shop", "Onboarding stage: hikify: review.", "2026-11-02-split-api (change: plan)", "hikari is not running for it."} {
		if !strings.Contains(got, want) {
			t.Errorf("the session start says %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "units") {
		t.Errorf("a done change is not open:\n%s", got)
	}
	writeHost(filepath.Join(dir, "topology.yaml"), hostFile{URL: "http://127.0.0.1:7420", Token: "secret", PID: os.Getpid()})
	got = sessionStart(dir)
	if !strings.Contains(got, "hikari runs for it, with its UI at http://127.0.0.1:7420") || strings.Contains(got, "secret") {
		t.Errorf("a run is named, without its token:\n%s", got)
	}
	if n := strings.Count(got, "\n"); n > 5 {
		t.Errorf("a few lines, not %d", n)
	}
}
