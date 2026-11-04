package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/api"
)

// TestRepoRoutes checks RP6 and RP7 of the repositories, forges, and
// issues spec: the UI reads the views and acts on them, the orchestration
// agent reads them, and a partition agent may not.
func TestRepoRoutes(t *testing.T) {
	s, _ := newTestServer(t)
	srv := httptest.NewServer(s.Handler("ui-secret"))
	defer srv.Close()
	ui := func(method, path string, body string) (*http.Response, []byte) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, bytes.NewReader([]byte(body)))
		req.AddCookie(&http.Cookie{Name: UICookie, Value: "ui-secret"})
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)
		return resp, buf.Bytes()
	}

	resp, data := ui(http.MethodGet, "/api/ui/repos", "")
	var views []api.RepoView
	if resp.StatusCode != http.StatusOK || json.Unmarshal(data, &views) != nil || len(views) != 1 || views[0].Name != "app" || views[0].Branch != "hikari/work" {
		t.Fatalf("GET /api/ui/repos: %d %s", resp.StatusCode, data)
	}
	if len(views[0].Branches) == 0 || !views[0].Branches[0].Yours {
		t.Errorf("RP2 your branch comes first: %+v", views[0].Branches)
	}
	if resp, data := ui(http.MethodGet, "/api/ui/repos/app", ""); resp.StatusCode != http.StatusOK || !strings.Contains(string(data), `"name":"app"`) {
		t.Errorf("GET one view: %d %s", resp.StatusCode, data)
	}
	if resp, _ := ui(http.MethodGet, "/api/ui/repos/nope", ""); resp.StatusCode != http.StatusNotFound {
		t.Errorf("an unknown repository is 404, not %d", resp.StatusCode)
	}
	// The test repo has no origin: a push and a pull request refuse with
	// the reason, and a refresh still answers the view.
	if resp, data := ui(http.MethodPost, "/api/ui/repos/app/push", ""); resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(data), "no origin") {
		t.Errorf("push without origin: %d %s", resp.StatusCode, data)
	}
	if resp, data := ui(http.MethodPost, "/api/ui/repos/app/pulls", `{"title":"x"}`); resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(data), "no origin") {
		t.Errorf("a pull request without origin: %d %s", resp.StatusCode, data)
	}
	if resp, data := ui(http.MethodPost, "/api/ui/repos/app/pulls/x/merge", ""); resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(data), "not a number") {
		t.Errorf("a bad number: %d %s", resp.StatusCode, data)
	}
	if resp, data := ui(http.MethodPost, "/api/ui/repos/app/refresh", ""); resp.StatusCode != http.StatusOK || !strings.Contains(string(data), `"name":"app"`) {
		t.Errorf("refresh: %d %s", resp.StatusCode, data)
	}

	agent := func(name string) (*http.Response, []byte) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/agent/repos", nil)
		req.Header.Set("Authorization", "Bearer "+s.IssueToken(name))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)
		return resp, buf.Bytes()
	}
	if resp, data := agent("lead"); resp.StatusCode != http.StatusOK || !strings.Contains(string(data), `"name":"app"`) {
		t.Errorf("RP7 the orchestration agent reads the views: %d %s", resp.StatusCode, data)
	}
	if resp, _ := agent("writer"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("RP7 a partition agent may not: %d", resp.StatusCode)
	}

	// A snapshot carries the views, and the module host reaches no tracker
	// for a repository without a forge.
	if snap := s.snapshot(); len(snap.Repos) != 1 || snap.Repos[0].Name != "app" {
		t.Errorf("the snapshot's repos: %+v", snap.Repos)
	}
	h := &moduleHost{s: s, name: "tasks"}
	if _, ok := h.Tracker("app"); ok || len(h.TrackerRepos()) != 0 {
		t.Error("a repository without a forge has no tracker")
	}
}
