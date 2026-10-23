package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/sysmap"
	"github.com/nathanaday/hikari/internal/systems"
	"github.com/nathanaday/hikari/internal/testmap"
	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/internal/vocab"
	"github.com/nathanaday/hikari/internal/wiki"
)

// company: ceo orchestrates lead. eng: lead orchestrates writer and critic.
const testTopology = `
name: test
x-hikari:
  hubs:
    company: {orchestrator: ceo, members: [lead]}
    eng: {orchestrator: lead, members: [writer, critic]}
services:
  ceo: {x-hikari: {description: runs the company}}
  lead: {x-hikari: {description: leads engineering}}
  writer: {x-hikari: {description: writes}}
  critic: {x-hikari: {description: reviews}}
`

func newTestServer(t *testing.T) (*Server, *bytes.Buffer) {
	t.Helper()
	topo, err := topology.Parse([]byte(testTopology), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sys, err := systems.New(topo)
	if err != nil {
		t.Fatal(err)
	}
	var journal bytes.Buffer
	return New(topo, &journal, sys), &journal
}

func post(hub, body string) api.SendRequest { return api.SendRequest{Hub: hub, Body: body} }
func dm(to, body string) api.SendRequest    { return api.SendRequest{To: to, Body: body} }

func TestWakeRules(t *testing.T) {
	cases := []struct {
		name string
		from string
		body string
		woke []string
	}{
		{"user post wakes only the orchestrator", "user", "hello team", []string{"lead"}},
		{"a mention also wakes that member", "user", "@writer draft it", []string{"lead", "writer"}},
		{"orchestrator post without mentions wakes all", "lead", "plan: write then review", []string{"critic", "writer"}},
		{"orchestrator post with a mention wakes that member", "lead", "@critic review this", []string{"critic"}},
		{"member post wakes the orchestrator", "writer", "draft done", []string{"lead"}},
		{"member can wake another member", "writer", "@critic please look", []string{"critic", "lead"}},
		{"mentions outside the hub do not count", "lead", "I report to @ceo", []string{"critic", "writer"}},
		{"an email address is not a mention", "lead", "mail a@writer.dev", []string{"critic", "writer"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, _ := newTestServer(t)
			m, err := s.Send(c.from, post("eng", c.body))
			if err != nil {
				t.Fatal(err)
			}
			woke := slices.Sorted(slices.Values(m.Woke))
			if want := slices.Sorted(slices.Values(c.woke)); !slices.Equal(woke, want) {
				t.Errorf("woke %v, want %v", woke, want)
			}
		})
	}
}

func TestEveryParticipantReceivesPosts(t *testing.T) {
	s, _ := newTestServer(t)
	s.Send("user", post("eng", "hello"))
	for _, n := range []string{"lead", "writer", "critic"} {
		if msgs := s.Take(context.Background(), n, 0); len(msgs) != 1 {
			t.Errorf("%s received %d messages, want 1", n, len(msgs))
		}
	}
	if msgs := s.Take(context.Background(), "ceo", 0); len(msgs) != 0 {
		t.Errorf("ceo is not in eng but received %+v", msgs)
	}
}

func TestAccess(t *testing.T) {
	allowed := []struct {
		from string
		req  api.SendRequest
	}{
		{"user", dm("writer", "x")},
		{"user", post("company", "x")},
		{"writer", dm("critic", "x")},
		{"writer", dm("user", "x")},
		{"lead", post("company", "x")},
		{"lead", dm("ceo", "x")},
	}
	for _, c := range allowed {
		if _, err := fresh(t).Send(c.from, c.req); err != nil {
			t.Errorf("%s %+v: %v", c.from, c.req, err)
		}
	}
	denied := []struct {
		from string
		req  api.SendRequest
	}{
		{"writer", dm("ceo", "x")},
		{"writer", post("company", "x")},
		{"ceo", post("eng", "x")},
		{"writer", dm("writer", "x")},
		{"writer", post("nope", "x")},
		{"writer", api.SendRequest{Hub: "eng", To: "critic", Body: "x"}},
		{"writer", api.SendRequest{Body: "x"}},
		{"writer", post("eng", "  ")},
	}
	for _, c := range denied {
		if _, err := fresh(t).Send(c.from, c.req); err == nil {
			t.Errorf("%s %+v must fail", c.from, c.req)
		}
	}
}

func fresh(t *testing.T) *Server {
	srv, _ := newTestServer(t)
	return srv
}

func TestTakeWaitsForWakingMessage(t *testing.T) {
	s, journal := newTestServer(t)
	ctx := context.Background()
	s.Send("writer", post("eng", "draft done")) // reaches critic without waking it
	if msgs := s.Take(ctx, "critic", 20*time.Millisecond); msgs != nil {
		t.Fatalf("Take returned %+v before a waking message", msgs)
	}
	go func() {
		time.Sleep(20 * time.Millisecond)
		s.Send("writer", dm("critic", "please review"))
	}()
	msgs := s.Take(ctx, "critic", time.Second)
	if len(msgs) != 2 || msgs[0].Hub != "eng" || msgs[1].To != "critic" {
		t.Fatalf("Take = %+v, want the hub post then the direct message", msgs)
	}
	if n := strings.Count(journal.String(), "\n"); n != 2 {
		t.Fatalf("journal has %d lines, want 2", n)
	}
}

func TestTakeWithoutWaitDrainsInbox(t *testing.T) {
	s, _ := newTestServer(t)
	s.Send("writer", post("eng", "draft done"))
	if msgs := s.Take(context.Background(), "critic", 0); len(msgs) != 1 {
		t.Fatalf("Take(0) = %+v, want the passive message", msgs)
	}
}

func TestRequeueKeepsOrder(t *testing.T) {
	s, _ := newTestServer(t)
	ctx := context.Background()
	s.Send("lead", dm("writer", "one"))
	taken := s.Take(ctx, "writer", 0)
	s.Send("lead", dm("writer", "two"))
	s.Requeue("writer", taken)
	msgs := s.Take(ctx, "writer", time.Second)
	if len(msgs) != 2 || msgs[0].Body != "one" || msgs[1].Body != "two" {
		t.Fatalf("after requeue = %+v", msgs)
	}
}

func TestSnapshotOfNewServerHasEmptyMessageList(t *testing.T) {
	s, _ := newTestServer(t)
	events, cancel := s.Subscribe()
	defer cancel()
	data, _ := json.Marshal((<-events).Snapshot)
	if !strings.Contains(string(data), `"messages":[]`) {
		t.Fatalf("snapshot = %s, want an empty messages list, not null", data)
	}
}

func TestNodeAPI(t *testing.T) {
	s, _ := newTestServer(t)
	srv := httptest.NewServer(s.Handler("ui-secret"))
	defer srv.Close()
	ctx := context.Background()

	lead := &api.Client{URL: srv.URL, Token: s.IssueToken("lead"), HTTP: srv.Client()}
	writer := &api.Client{URL: srv.URL, Token: s.IssueToken("writer"), HTTP: srv.Client()}

	self, err := lead.Self(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(self.Hubs) != 2 || self.Hubs[1].Name != "eng" {
		t.Fatalf("lead hubs = %+v", self.Hubs)
	}
	eng := self.Hubs[1].Participants
	if eng[0].Name != "lead" || eng[0].Role != api.RoleOrchestrator || eng[len(eng)-1].Role != api.RoleOperator {
		t.Fatalf("eng participants = %+v", eng)
	}

	if _, err := writer.Send(ctx, dm("ceo", "x")); err == nil || !strings.Contains(err.Error(), "can send direct messages to critic, lead, user") {
		t.Fatalf("writer->ceo error = %v", err)
	}
	if _, err := lead.Send(ctx, post("eng", "@writer go")); err != nil {
		t.Fatal(err)
	}
	msgs, err := writer.Inbox(ctx, time.Second)
	if err != nil || len(msgs) != 1 || msgs[0].Hub != "eng" {
		t.Fatalf("writer inbox = %+v, %v", msgs, err)
	}

	if err := writer.SetStatus(ctx, api.Status{State: api.StateWorking}); err != nil {
		t.Fatal(err)
	}
	if got := s.status["writer"].State; got != api.StateWorking {
		t.Fatalf("writer status = %q", got)
	}

	forged := &api.Client{URL: srv.URL, Token: "nope", HTTP: srv.Client()}
	if _, err := forged.Self(ctx); err == nil {
		t.Fatal("unknown token must be rejected")
	}
}

func TestUIRequiresToken(t *testing.T) {
	s, _ := newTestServer(t)
	srv := httptest.NewServer(s.Handler("ui-secret"))
	defer srv.Close()

	// A node token must not open the UI API.
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/ui/send", strings.NewReader(`{"to":"lead","body":"x"}`))
	req.Header.Set("Authorization", "Bearer "+s.IssueToken("lead"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("UI send without cookie = %s", resp.Status)
	}

	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar}
	if resp, _ := browser.Get(srv.URL + "/api/ui/session"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("session before login = %s", resp.Status)
	}
	if resp, _ := browser.Post(srv.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"wrong"}`)); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("login with a wrong token = %s", resp.Status)
	}
	resp, err = browser.Post(srv.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"ui-secret"}`))
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("UI login = %v, %v", resp.Status, err)
	}
	if resp, _ := browser.Get(srv.URL + "/api/ui/session"); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("session after login = %s", resp.Status)
	}

	events, err := browser.Get(srv.URL + "/api/ui/events")
	if err != nil {
		t.Fatal(err)
	}
	defer events.Body.Close()
	lines := bufio.NewScanner(events.Body)
	next := func() Event {
		for lines.Scan() {
			if data, ok := strings.CutPrefix(lines.Text(), "data: "); ok {
				var e Event
				json.Unmarshal([]byte(data), &e)
				return e
			}
		}
		t.Fatal("event stream ended")
		return Event{}
	}
	if e := next(); e.Type != "snapshot" || len(e.Snapshot.Agents) != 4 || len(e.Snapshot.Hubs) != 2 {
		t.Fatalf("first event = %+v", e)
	}

	for _, body := range []string{`{"hub":"company","body":"hello"}`, `{"to":"critic","body":"hi"}`} {
		resp, err = browser.Post(srv.URL+"/api/ui/send", "application/json", strings.NewReader(body))
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("UI send %s = %v, %v", body, resp.Status, err)
		}
		if e := next(); e.Type != "message" || e.Message.From != "user" {
			t.Fatalf("message event = %+v", e)
		}
	}

	resp, _ = browser.Post(srv.URL+"/api/ui/send", "application/json", strings.NewReader(`{"hub":"nope","body":"hello"}`))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("UI post to unknown hub = %s", resp.Status)
	}
}

func TestSystemRoutes(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR", "GIT_COMMITTER"} {
		t.Setenv(k+"_NAME", "Test User")
		t.Setenv(k+"_EMAIL", "user@example.com")
	}
	dir := t.TempDir()
	repo := filepath.Join(dir, "shop")
	os.MkdirAll(filepath.Join(repo, "orders"), 0o755)
	os.WriteFile(filepath.Join(repo, "orders", "create.py"), []byte("x\n"), 0o644)
	for _, args := range [][]string{{"init", "--quiet"}, {"add", "--all"}, {"commit", "--quiet", "-m", "initial"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	topo, err := topology.Parse([]byte(`
name: test
x-hikari:
  repos: {shop: {path: shop, branch: hikari/work}}
  hubs: {h: {orchestrator: lead, members: [orders]}}
services:
  lead: {x-hikari: {}}
  orders: {x-hikari: {system: {repo: shop, scope: orders}, grants: {deliverables: [lib]}}}
`), dir)
	if err != nil {
		t.Fatal(err)
	}
	deliverable := filepath.Join(dir, "store", "shared", "deliverables", "lib")
	os.MkdirAll(deliverable, 0o755)
	os.WriteFile(filepath.Join(deliverable, "deliverable.yaml"), []byte("version: 1\ndescription: The library.\nlayout: {require: [README.md]}\n"), 0o644)
	os.WriteFile(filepath.Join(deliverable, "SKILL.md"), []byte("# lib\n"), 0o644)
	sys, err := systems.New(topo)
	if err != nil {
		t.Fatal(err)
	}
	s := New(topo, nil, sys)
	srv := httptest.NewServer(s.Handler("ui-secret"))
	defer srv.Close()
	ctx := context.Background()
	orders := &api.Client{URL: srv.URL, Token: s.IssueToken("orders"), HTTP: srv.Client()}
	lead := &api.Client{URL: srv.URL, Token: s.IssueToken("lead"), HTTP: srv.Client()}

	events, cancel := s.Subscribe()
	defer cancel()
	snap := (<-events).Snapshot
	if len(snap.Systems) != 1 || snap.Systems[0].Owner != "orders" || len(snap.Vocabularies) != 1 {
		t.Fatalf("snapshot systems = %+v, vocabularies = %+v", snap.Systems, snap.Vocabularies)
	}
	if len(snap.Repos) != 1 || snap.Repos[0].Name != "shop" || snap.Repos[0].Branch != "hikari/work" || !slices.Equal(snap.Repos[0].Systems, []string{"orders"}) {
		t.Errorf("N2: repos = %+v", snap.Repos)
	}
	if len(snap.Deliverables) != 1 || snap.Deliverables[0].Name != "lib" || !slices.Equal(snap.Deliverables[0].Holders, []string{"orders"}) {
		t.Errorf("N3: deliverables = %+v", snap.Deliverables)
	}
	if g := snap.Systems[0].Grants; !slices.Equal(g.Deliverables, []string{"lib"}) || g.Releases == nil {
		t.Errorf("N1: grants = %+v", g)
	}
	if data, _ := json.Marshal(snap.Repos); strings.Contains(string(data), dir) {
		t.Errorf("N2: repos hold a host path: %s", data)
	}

	self, err := orders.Self(ctx)
	if err != nil || self.System == nil || self.System.WorkDir != "/workspace/orders" {
		t.Fatalf("owner self = %+v, %v", self.System, err)
	}
	purpose := "## Purpose\n\nA customer places an order.\n"
	if _, err := orders.ChangeWiki(ctx, "", []wiki.Write{{Op: "create", ID: "create-order", Name: "Create an order", Description: "Stores an order.", Body: &purpose}}); err != nil {
		t.Fatal(err)
	}
	// One change sends the wiki, the system, and the support of its hub.
	got := map[string]bool{}
	for range 3 {
		e := <-events
		switch {
		case e.Type == "wiki" && e.Wiki.Ref == "system:orders" && len(e.Wiki.Documents) == 1:
			got["system wiki"] = true
		case e.Type == "system" && e.System.Owner == "orders" && len(e.System.Features) == 1:
			got["system"] = true
		case e.Type == "wiki" && e.Wiki.Ref == "hub:h":
			got["hub wiki"] = true
		default:
			t.Fatalf("event = %+v", e)
		}
	}
	if len(got) != 3 {
		t.Errorf("events = %v", got)
	}
	if _, err := orders.ChangeWiki(ctx, "", []wiki.Write{{Op: "create", ID: "Bad ID", Name: "Bad", Description: "Bad.", Body: &purpose}}); err == nil || !strings.Contains(err.Error(), "W1") {
		t.Errorf("invalid feature error = %v", err)
	}

	systemsList, err := lead.ListSystems(ctx)
	if err != nil || len(systemsList) != 1 || systemsList[0].Features[testmap.Untested] != 1 {
		t.Errorf("lead list = %+v, %v", systemsList, err)
	}
	if _, err := orders.ListSystems(ctx); err == nil || !strings.Contains(err.Error(), "only orchestrators") {
		t.Errorf("owner list error = %v", err)
	}
	if _, err := lead.GetSystem(ctx, "ghost"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("unknown system error = %v", err)
	}
	if err := lead.ChangeVocabulary(ctx, "h", api.VocabularyChange{Op: "define_term", Term: &vocab.Term{Term: "order", Definition: "A request."}}); err != nil {
		t.Fatal(err)
	}
	if e := <-events; e.Type != "vocabulary" || len(e.Vocabulary.Terms) != 1 {
		t.Fatalf("vocabulary event = %+v", e)
	}
	if err := orders.ChangeVocabulary(ctx, "h", api.VocabularyChange{Op: "remove", Name: "order"}); err == nil {
		t.Error("an owner changed the vocabulary")
	}
	if commit, err := orders.Commit(ctx, "Add the map"); err != nil || commit == "" {
		t.Errorf("commit = %q, %v", commit, err)
	}
	if tc, err := orders.TurnContext(ctx); err != nil || tc.MapState != sysmap.Unbuilt {
		t.Errorf("turn context = %+v, %v", tc, err)
	}
}

func TestPublishAnnouncesTheRelease(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR", "GIT_COMMITTER"} {
		t.Setenv(k+"_NAME", "Test User")
		t.Setenv(k+"_EMAIL", "user@example.com")
	}
	dir := t.TempDir()
	repo := filepath.Join(dir, "shop")
	os.MkdirAll(filepath.Join(repo, "orders"), 0o755)
	os.WriteFile(filepath.Join(repo, "orders", "create.py"), []byte("x\n"), 0o644)
	for _, args := range [][]string{{"init", "--quiet"}, {"add", "--all"}, {"commit", "--quiet", "-m", "initial"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	topo, err := topology.Parse([]byte(`
name: test
x-hikari:
  repos: {shop: {path: shop, branch: hikari/work}}
  hubs: {h: {orchestrator: lead, members: [orders]}}
services:
  lead: {x-hikari: {}}
  orders: {x-hikari: {system: {repo: shop, scope: orders}, grants: {deliverables: [lib]}}}
`), dir)
	if err != nil {
		t.Fatal(err)
	}
	deliverable := filepath.Join(dir, "store", "shared", "deliverables", "lib")
	os.MkdirAll(deliverable, 0o755)
	os.WriteFile(filepath.Join(deliverable, "deliverable.yaml"), []byte("version: 1\ndescription: The library.\nlayout: {require: [README.md]}\n"), 0o644)
	os.WriteFile(filepath.Join(deliverable, "SKILL.md"), []byte("# lib\n"), 0o644)
	sys, err := systems.New(topo)
	if err != nil {
		t.Fatal(err)
	}
	s := New(topo, nil, sys)
	srv := httptest.NewServer(s.Handler("ui"))
	defer srv.Close()
	ctx := context.Background()
	orders := &api.Client{URL: srv.URL, Token: s.IssueToken("orders"), HTTP: srv.Client()}

	if _, err := orders.MarkMapCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	staging := filepath.Join(dir, "store", "systems", "orders", "staging", "lib")
	os.MkdirAll(staging, 0o755)
	if _, err := orders.CheckRelease(ctx, "lib"); err == nil || !strings.Contains(err.Error(), `no file matches "README.md"`) {
		t.Errorf("R15 check of an empty product: %v", err)
	}
	os.WriteFile(filepath.Join(staging, "README.md"), []byte("# orders\n"), 0o644)
	if check, err := orders.CheckRelease(ctx, "lib"); err != nil || check.Files != 1 {
		t.Errorf("R15 check = %+v, %v", check, err)
	}

	if _, err := orders.PublishRelease(ctx, api.PublishRequest{Deliverable: "lib", Version: "1.0.0"}); err == nil {
		t.Error("a release without notes must be refused")
	}
	rel, err := orders.PublishRelease(ctx, api.PublishRequest{Deliverable: "lib", Version: "1.0.0", Notes: "First release."})
	if err != nil {
		t.Fatal(err)
	}
	if rel.Version != "1.0.0" || rel.Deliverable != "lib" || rel.Product.Files != 1 {
		t.Errorf("release = %+v", rel)
	}
	msgs := s.Take(ctx, "lead", time.Second)
	if len(msgs) != 1 || msgs[0].From != "orders" || msgs[0].Hub != "h" || msgs[0].Body != "Released orders lib 1.0.0: First release." || !slices.Contains(msgs[0].Woke, "lead") {
		t.Errorf("R18 announcement = %+v", msgs)
	}
	self := s.Self("orders")
	if len(self.Deliverables) != 1 || self.Deliverables[0].Description != "The library." {
		t.Errorf("Self.Deliverables = %+v", self.Deliverables)
	}
	if lead := s.Self("lead"); len(lead.MemberDeliverables["orders"]) != 1 {
		t.Errorf("Self.MemberDeliverables = %+v", lead.MemberDeliverables)
	}
}

func TestStoreRoutes(t *testing.T) {
	topo, err := topology.Parse([]byte(testTopology), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sys, err := systems.New(topo)
	if err != nil {
		t.Fatal(err)
	}
	s := New(topo, nil, sys)
	srv := httptest.NewServer(s.Handler("ui-secret"))
	defer srv.Close()
	root := sys.Store().Root
	os.MkdirAll(filepath.Join(root, "shared", "skills", "style"), 0o755)
	os.WriteFile(filepath.Join(root, "shared", "skills", "style", "SKILL.md"), []byte("# style\n"), 0o644)
	os.WriteFile(filepath.Join(root, "shared", "policies", "security.md"), []byte("Never log secrets.\n"), 0o644)
	os.WriteFile(filepath.Join(root, "shared", "policies", "logo.png"), []byte{0x89, 0x50, 0x00}, 0o644)
	os.WriteFile(filepath.Join(root, "shared", "agents", "binary.md"), []byte{0x00, 0x01}, 0o644)
	outside := filepath.Join(t.TempDir(), "secret.txt")
	os.WriteFile(outside, []byte("secret"), 0o644)
	os.Symlink(outside, filepath.Join(root, "shared", "policies", "escape.md"))
	os.MkdirAll(filepath.Join(root, "shared", "deliverables", "docs-site"), 0o755)
	os.WriteFile(filepath.Join(root, "shared", "deliverables", "docs-site", "deliverable.yaml"), []byte("version: 1\ndescription: A site.\ntargets: [{type: dockerhub}]\n"), 0o644)
	os.WriteFile(filepath.Join(root, "shared", "deliverables", "docs-site", "SKILL.md"), []byte("# site\n"), 0o644)
	os.MkdirAll(filepath.Join(root, "shared", "deliverables", "broken"), 0o755)

	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar}
	browser.Post(srv.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"ui-secret"}`))
	get := func(path string) (int, string) {
		resp, err := browser.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	code, body := get("/api/ui/store/shared")
	if code != 200 || !strings.Contains(body, `"name":"style"`) || !strings.Contains(body, `"path":"shared/policies/security.md"`) || strings.Contains(body, "logo") {
		t.Errorf("shared listing = %d %s", code, body)
	}
	if !strings.Contains(body, `"name":"docs-site","path":"shared/deliverables/docs-site","skill":"shared/deliverables/docs-site/SKILL.md","description":"A site."`) ||
		!strings.Contains(body, `"targets":[{"type":"dockerhub"}],"holders":[]`) || !strings.Contains(body, `"name":"broken"`) || !strings.Contains(body, `"error":"deliverable \"broken\" has no deliverable.yaml`) {
		t.Errorf("U12 deliverables listing = %s", body)
	}
	if code, body := get("/api/ui/store/file?path=shared/policies/security.md"); code != 200 || body != "Never log secrets.\n" {
		t.Errorf("text file = %d %q", code, body)
	}
	for path, want := range map[string]int{
		"../../etc/passwd":           400,
		"/etc/passwd":                400,
		"shared/policies/escape.md":  400,
		"shared/agents/binary.md":    415,
		"shared/policies/missing.md": 404,
		"shared/policies":            404,
	} {
		if code, _ := get("/api/ui/store/file?path=" + path); code != want {
			t.Errorf("U10: %s = %d, want %d", path, code, want)
		}
	}
	if code, _ := get("/api/ui/store/releases/nope/lib/1.0.0"); code != 404 {
		t.Errorf("unknown release = %d", code)
	}
	resp, _ := http.Get(srv.URL + "/api/ui/store/shared")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("the store needs the UI token: %s", resp.Status)
	}
}

func TestActivity(t *testing.T) {
	s, _ := newTestServer(t)
	events, cancel := s.Subscribe()
	defer cancel()
	<-events // the snapshot

	s.AddActivity("writer", api.Activity{ID: 1, Turn: 1, Kind: api.ActivityTurn, Text: "woken by user in #eng"})
	s.AddActivity("writer", api.Activity{ID: 2, Turn: 1, Kind: api.ActivityTool, Tool: "Read"})
	s.AddActivity("writer", api.Activity{ID: 2, Turn: 1, Kind: api.ActivityTool, Tool: "Read", Output: "ok", Done: true})
	s.AddActivity("writer", api.Activity{ID: 3, Turn: 1, Kind: api.ActivityDone, Text: "Should I add it?", Done: true})
	for range 4 {
		if e := <-events; e.Type != "activity" || e.Node != "writer" {
			t.Errorf("event = %+v", e)
		}
	}
	list := s.snapshot().Activity["writer"]
	if len(list) != 3 || list[1].Output != "ok" || !list[1].Done {
		t.Errorf("an entry with a kept ID must replace it: %+v", list)
	}
	if !list[2].Unheard {
		t.Error("a closing text with no message sent must be unheard")
	}

	s.AddActivity("writer", api.Activity{ID: 4, Turn: 2, Kind: api.ActivityTurn})
	s.Send("writer", post("eng", "done"))
	s.AddActivity("writer", api.Activity{ID: 5, Turn: 2, Kind: api.ActivityDone, Text: "Posted the draft.", Done: true})
	list = s.snapshot().Activity["writer"]
	if list[len(list)-1].Unheard {
		t.Error("a closing text after a message is heard")
	}

	for i := range int64(maxActivity + 10) {
		s.AddActivity("critic", api.Activity{ID: 100 + i, Turn: 10 + i/50, Kind: api.ActivityText})
	}
	list = s.snapshot().Activity["critic"]
	if len(list) > maxActivity || list[0].Turn != 11 || list[0].ID != 150 {
		t.Errorf("old turns must be dropped whole: %d entries, first %+v", len(list), list[0])
	}
}
