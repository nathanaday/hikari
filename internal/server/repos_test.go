package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/topology"
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

// TestDecisions checks DC1 to DC3 of the attention spec: an agent asks the
// user a decision, hikari may ask one too, and the user's answer is a
// direct message to the decision's recipient that names what it answers.
func TestDecisions(t *testing.T) {
	s, _ := newTestServer(t)
	d := api.Decision{Question: "Wrap style?", Options: []string{"One style", "Let the customer pick"}}
	if _, err := s.Send("writer", api.SendRequest{To: "critic", Decision: &d}); err == nil {
		t.Error("DC1 a decision goes to the user")
	}
	if _, err := s.Send("writer", api.SendRequest{To: "user", Decision: &api.Decision{Question: "x", Options: []string{"only one"}}}); err == nil {
		t.Error("DC1 a decision has at least two options")
	}
	events, cancel := s.Subscribe()
	defer cancel()
	<-events
	m, err := s.Send("writer", api.SendRequest{To: "user", Decision: &d})
	if err != nil {
		t.Fatal(err)
	}
	if e := <-events; e.Type != "message" || e.Message.Decision == nil {
		t.Errorf("DC1 the event carries the decision: %+v", e.Message)
	}
	if m.Decision == nil || m.Decision.ReplyTo != "writer" || m.Body != "Wrap style?\n1. One style\n2. Let the customer pick" {
		t.Errorf("the decision message: %+v", m)
	}
	ask := api.Decision{Question: "Proceed?", Options: []string{"Yes", "No"}, ReplyTo: "lead", Task: "cancel"}
	h := &moduleHost{s: s, name: "tasks"}
	h.Ask(ask)
	snap := s.snapshot()
	last := snap.Messages[len(snap.Messages)-1]
	if last.From != topology.Hikari || last.To != "user" || last.Decision == nil || last.Decision.ReplyTo != "lead" {
		t.Errorf("DC2 hikari asks the user: %+v", last)
	}
	if _, err := s.Decide(DecideRequest{Message: m.ID, Choice: 3}); err == nil {
		t.Error("DC3 the choice is within the options")
	}
	if _, err := s.Decide(DecideRequest{Message: 999, Choice: 1}); err == nil {
		t.Error("DC3 only a decision is answered")
	}
	a, err := s.Decide(DecideRequest{Message: m.ID, Choice: 2, Note: "and remember the choice"})
	if err != nil {
		t.Fatal(err)
	}
	if a.From != "user" || a.To != "writer" || a.Answers != m.ID || a.Body != "2. Let the customer pick\nand remember the choice" {
		t.Errorf("DC3 the answer: %+v", a)
	}
	if msgs := s.Take(context.Background(), "writer", 0); len(msgs) != 1 || msgs[0].ID != a.ID {
		t.Errorf("the answer wakes the asker: %+v", msgs)
	}
	b, err := s.Decide(DecideRequest{Message: last.ID, Choice: 1})
	if err != nil || b.To != "lead" {
		t.Errorf("hikari's decision is answered to the orchestration agent: %+v %v", b, err)
	}
}

// TestProposedRule checks a proposed rule: an agent proposes one as a
// decision, Accept writes it into the public wiki as the user's rule and
// says so in the answer, Decline writes nothing, and a bad proposal is
// refused.
func TestProposedRule(t *testing.T) {
	s, _ := newTestServer(t)
	good := api.Proposal{ID: "no-silent-retries", Name: "No silent retries", Rule: "A retry of a failed call is logged with its reason.", Reason: "orders retried a stock call three times without a line, and the block took an hour to find."}
	for name, bad := range map[string]api.Proposal{
		"id":     {ID: "No Silent", Name: good.Name, Rule: good.Rule, Reason: good.Reason},
		"name":   {ID: good.ID, Name: "Retries", Rule: good.Rule, Reason: good.Reason},
		"rule":   {ID: good.ID, Name: good.Name, Rule: "", Reason: good.Reason},
		"long":   {ID: good.ID, Name: good.Name, Rule: strings.Repeat("x", 201), Reason: good.Reason},
		"reason": {ID: good.ID, Name: good.Name, Rule: good.Rule},
	} {
		d := api.ProposeRule("writer", bad)
		if _, err := s.Send("writer", api.SendRequest{To: "user", Decision: &d}); err == nil {
			t.Errorf("a proposal with a bad %s is accepted", name)
		}
	}
	d := api.ProposeRule("writer", good)
	m, err := s.Send("writer", api.SendRequest{To: "user", Decision: &d})
	if err != nil {
		t.Fatal(err)
	}
	if m.Decision == nil || m.Decision.Proposal == nil || m.Decision.ReplyTo != "writer" || !strings.Contains(m.Body, "writer proposes a rule for every agent: A retry of a failed call is logged with its reason. Accept it") {
		t.Errorf("the proposal message: %+v %q", m.Decision, m.Body)
	}
	// Decline writes nothing.
	a, err := s.Decide(DecideRequest{Message: m.ID, Choice: 2, Note: "we log already"})
	if err != nil || a.To != "writer" || strings.Contains(a.Body, "wrote the rule") {
		t.Errorf("decline: %+v %v", a, err)
	}
	if w, _ := s.sys.WikiView(api.PublicWikiRef); slices.ContainsFunc(w.Documents, func(x api.WikiDoc) bool { return x.ID == good.ID }) {
		t.Error("a declined rule is not written")
	}
	// Accept writes the rule as the user's.
	m2, _ := s.Send("critic", api.SendRequest{To: "user", Decision: ptr(api.ProposeRule("critic", good))})
	a, err = s.Decide(DecideRequest{Message: m2.ID, Choice: 1})
	if err != nil || a.To != "critic" || !strings.Contains(a.Body, "hikari wrote the rule [[no-silent-retries]] into the public wiki") {
		t.Fatalf("accept: %+v %v", a, err)
	}
	w, _ := s.sys.WikiView(api.PublicWikiRef)
	i := slices.IndexFunc(w.Documents, func(x api.WikiDoc) bool { return x.ID == good.ID })
	if i < 0 || w.Documents[i].Kind != "rule" || w.Documents[i].Name != "No silent retries" || w.Documents[i].Description != good.Rule {
		t.Errorf("the rule in the public wiki: %+v", w.Documents)
	}
	hit, err := s.sys.GetDocument("writer", good.ID)
	if err != nil || !strings.Contains(hit.Document, "## Purpose") || !strings.Contains(hit.Document, "took an hour to find") {
		t.Errorf("every agent reads the rule: %+v %v", hit, err)
	}
	// A second accept of the same id is refused by the wiki, and the
	// decision stays open.
	m3, _ := s.Send("writer", api.SendRequest{To: "user", Decision: ptr(api.ProposeRule("writer", api.Proposal{ID: good.ID, Name: "Another name here", Rule: "Something else.", Reason: "r"}))})
	if _, err := s.Decide(DecideRequest{Message: m3.ID, Choice: 1}); err == nil {
		t.Error("writing over an existing rule is refused")
	}
}

func ptr[T any](v T) *T { return &v }
