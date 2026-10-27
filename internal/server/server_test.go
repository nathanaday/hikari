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
	"github.com/nathanaday/hikari/internal/featuremap"
	"github.com/nathanaday/hikari/internal/partitions"
	"github.com/nathanaday/hikari/internal/testmap"
	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/internal/vocab"
	"github.com/nathanaday/hikari/internal/wiki"
)

// testTopology is the room of the tests: lead orchestrates the partition agents
// writer and critic, whose code is in the repo REPO.
const testTopology = `
name: test
x-hikari:
  repos: {app: {path: 'REPO', branch: hikari/work}}
  orchestration: lead
services:
  lead: {x-hikari: {description: leads engineering}}
  writer: {x-hikari: {description: writes, partition: {repo: app, scope: writer}}}
  critic: {x-hikari: {description: reviews, partition: {repo: app, scope: critic}}}
`

// flatTopology is the room without an orchestration agent: lead has a partition too.
var flatTopology = strings.NewReplacer(
	"  orchestration: lead\n", "",
	"lead: {x-hikari: {description: leads engineering}}", "lead: {x-hikari: {description: leads engineering, partition: {repo: app, scope: lead}}}",
).Replace(testTopology)

// gitRepo makes a git repo with one file in each of dirs, and returns its
// path.
func gitRepo(t *testing.T, dir string, dirs ...string) string {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR", "GIT_COMMITTER"} {
		t.Setenv(k+"_NAME", "Test User")
		t.Setenv(k+"_EMAIL", "user@example.com")
	}
	repo := filepath.Join(dir, "app")
	for _, d := range dirs {
		os.MkdirAll(filepath.Join(repo, d), 0o755)
		os.WriteFile(filepath.Join(repo, d, "main.py"), []byte("x\n"), 0o644)
	}
	for _, args := range [][]string{{"init", "--quiet"}, {"add", "--all"}, {"commit", "--quiet", "-m", "initial"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	return repo
}

// parseRoom parses src, a topology of the tests, with its repo.
func parseRoom(t *testing.T, src string) *topology.Topology {
	t.Helper()
	dir := t.TempDir()
	repo := gitRepo(t, dir, "writer", "critic", "lead")
	topo, err := topology.Parse([]byte(strings.ReplaceAll(src, "REPO", repo)), dir)
	if err != nil {
		t.Fatal(err)
	}
	return topo
}

func newServerOf(t *testing.T, src string) (*Server, *bytes.Buffer) {
	t.Helper()
	topo := parseRoom(t, src)
	sys, err := partitions.New(topo)
	if err != nil {
		t.Fatal(err)
	}
	var journal bytes.Buffer
	return New(topo, &journal, sys), &journal
}

func newTestServer(t *testing.T) (*Server, *bytes.Buffer) { return newServerOf(t, testTopology) }

func post(body string) api.SendRequest   { return api.SendRequest{Body: body} }
func dm(to, body string) api.SendRequest { return api.SendRequest{To: to, Body: body} }

// TestWakeRules runs the cases of web/src/lib/fixtures/wake.json, which
// web/src/lib/rules.test.ts runs too (rule H6 of the remove-hubs spec).
func TestWakeRules(t *testing.T) {
	data, err := os.ReadFile("../../web/src/lib/fixtures/wake.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Cases []struct {
			Name, Room, From, To, Body string
			Woke                       []string
		}
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	rooms := map[string]string{"orchestrated": testTopology, "flat": flatTopology}
	for _, c := range file.Cases {
		t.Run(c.Name, func(t *testing.T) {
			s, _ := newServerOf(t, rooms[c.Room])
			m, err := s.Send(c.From, api.SendRequest{To: c.To, Body: c.Body})
			if err != nil {
				t.Fatal(err)
			}
			woke := slices.Sorted(slices.Values(m.Woke))
			if want := slices.Sorted(slices.Values(c.Woke)); !slices.Equal(woke, want) {
				t.Errorf("woke %v, want %v", woke, want)
			}
		})
	}
}

func TestEveryParticipantReceivesPosts(t *testing.T) {
	s, _ := newTestServer(t)
	s.Send("user", post("hello"))
	for _, n := range []string{"lead", "writer", "critic"} {
		if msgs := s.Take(context.Background(), n, 0); len(msgs) != 1 {
			t.Errorf("%s received %d messages, want 1", n, len(msgs))
		}
	}
}

func TestAccess(t *testing.T) {
	allowed := []struct {
		from string
		req  api.SendRequest
	}{
		{"user", dm("writer", "x")},
		{"user", post("x")},
		{"writer", dm("critic", "x")},
		{"writer", dm("user", "x")},
		{"writer", dm("lead", "x")},
		{"lead", post("x")},
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
		{"writer", dm("writer", "x")},
		{"writer", post("  ")},
		{"writer", dm("critic", "")},
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
	s.Send("writer", post("draft done")) // reaches critic without waking it
	if msgs := s.Take(ctx, "critic", 20*time.Millisecond); msgs != nil {
		t.Fatalf("Take returned %+v before a waking message", msgs)
	}
	go func() {
		time.Sleep(20 * time.Millisecond)
		s.Send("writer", dm("critic", "please review"))
	}()
	msgs := s.Take(ctx, "critic", time.Second)
	if len(msgs) != 2 || msgs[0].To != "" || msgs[1].To != "critic" {
		t.Fatalf("Take = %+v, want the post then the direct message", msgs)
	}
	if n := strings.Count(journal.String(), "\n"); n != 2 {
		t.Fatalf("journal has %d lines, want 2", n)
	}
}

func TestTakeWithoutWaitDrainsInbox(t *testing.T) {
	s, _ := newTestServer(t)
	s.Send("writer", post("draft done"))
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

func TestAgentAPI(t *testing.T) {
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
	room := self.Participants
	if !self.Orchestration || len(room) != 4 || room[0].Name != "lead" || room[0].Role != api.RoleOrchestration || room[1].Role != api.RolePartition || room[3].Role != api.RoleOperator {
		t.Fatalf("lead's room = %+v", room)
	}

	if _, err := writer.Send(ctx, dm("ceo", "x")); err == nil || !strings.Contains(err.Error(), "the room is lead, critic, user") {
		t.Fatalf("writer->ceo error = %v", err)
	}
	if _, err := lead.Send(ctx, post("@writer go")); err != nil {
		t.Fatal(err)
	}
	msgs, err := writer.Inbox(ctx, time.Second)
	if err != nil || len(msgs) != 1 || msgs[0].To != "" {
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

	// An agent token must not open the UI API.
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
	if e := next(); e.Type != "snapshot" || len(e.Snapshot.Agents) != 3 || len(e.Snapshot.Participants) != 4 {
		t.Fatalf("first event = %+v", e)
	}

	for _, body := range []string{`{"body":"hello"}`, `{"to":"critic","body":"hi"}`} {
		resp, err = browser.Post(srv.URL+"/api/ui/send", "application/json", strings.NewReader(body))
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("UI send %s = %v, %v", body, resp.Status, err)
		}
		if e := next(); e.Type != "message" || e.Message.From != "user" {
			t.Fatalf("message event = %+v", e)
		}
	}

	resp, _ = browser.Post(srv.URL+"/api/ui/send", "application/json", strings.NewReader(`{"to":"nope","body":"hello"}`))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("UI message to an unknown agent = %s", resp.Status)
	}
}

func TestPartitionRoutes(t *testing.T) {
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
  orchestration: lead
services:
  lead: {x-hikari: {}}
  orders: {x-hikari: {partition: {repo: shop, scope: orders}, grants: {deliverables: [lib]}}}
`), dir)
	if err != nil {
		t.Fatal(err)
	}
	deliverable := filepath.Join(dir, "vault", "public", "deliverables", "lib")
	os.MkdirAll(deliverable, 0o755)
	os.WriteFile(filepath.Join(deliverable, "deliverable.yaml"), []byte("version: 1\ndescription: The library.\nlayout: {require: [README.md]}\n"), 0o644)
	os.WriteFile(filepath.Join(deliverable, "SKILL.md"), []byte("# lib\n"), 0o644)
	sys, err := partitions.New(topo)
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
	if len(snap.Partitions) != 1 || snap.Partitions[0].PartitionAgent != "orders" || snap.Vocabulary.Terms == nil {
		t.Fatalf("snapshot partitions = %+v, vocabulary = %+v", snap.Partitions, snap.Vocabulary)
	}
	if len(snap.Repos) != 1 || snap.Repos[0].Name != "shop" || snap.Repos[0].Branch != "hikari/work" || !slices.Equal(snap.Repos[0].Partitions, []string{"orders"}) {
		t.Errorf("N2: repos = %+v", snap.Repos)
	}
	if len(snap.Deliverables) != 1 || snap.Deliverables[0].Name != "lib" || !slices.Equal(snap.Deliverables[0].Holders, []string{"orders"}) {
		t.Errorf("N3: deliverables = %+v", snap.Deliverables)
	}
	if g := snap.Partitions[0].Grants; !slices.Equal(g.Deliverables, []string{"lib"}) || g.Releases == nil {
		t.Errorf("N1: grants = %+v", g)
	}
	if data, _ := json.Marshal(snap.Repos); strings.Contains(string(data), dir) {
		t.Errorf("N2: repos hold a host path: %s", data)
	}

	self, err := orders.Self(ctx)
	if err != nil || self.Partition == nil || self.Partition.WorkDir != "/workspace/orders" {
		t.Fatalf("partition agent self = %+v, %v", self.Partition, err)
	}
	purpose := "## Purpose\n\nA customer places an order.\n"
	if _, err := orders.ChangeWiki(ctx, false, []wiki.Write{{Op: "create", ID: "create-order", Name: "Create an order", Description: "Stores an order.", Body: &purpose}}); err != nil {
		t.Fatal(err)
	}
	// One change sends the wiki, the partition, and the support of the public
	// wiki.
	got := map[string]bool{}
	for range 3 {
		e := <-events
		switch {
		case e.Type == "wiki" && e.Wiki.Ref == "partition:orders" && len(e.Wiki.Documents) == 1:
			got["feature map"] = true
		case e.Type == "partition" && e.Partition.PartitionAgent == "orders" && len(e.Partition.Features) == 1:
			got["partition"] = true
		case e.Type == "wiki" && e.Wiki.Ref == "public":
			got["public wiki"] = true
		default:
			t.Fatalf("event = %+v", e)
		}
	}
	if len(got) != 3 {
		t.Errorf("events = %v", got)
	}
	if _, err := orders.ChangeWiki(ctx, false, []wiki.Write{{Op: "create", ID: "Bad ID", Name: "Bad", Description: "Bad.", Body: &purpose}}); err == nil || !strings.Contains(err.Error(), "W1") {
		t.Errorf("invalid feature error = %v", err)
	}

	partitionsList, err := lead.ListPartitions(ctx)
	if err != nil || len(partitionsList) != 1 || partitionsList[0].Features[testmap.Untested] != 1 {
		t.Errorf("lead list = %+v, %v", partitionsList, err)
	}
	if _, err := orders.ListPartitions(ctx); err == nil || !strings.Contains(err.Error(), "only \"lead\", the orchestration agent") {
		t.Errorf("partition agent list error = %v", err)
	}
	if _, err := lead.GetPartition(ctx, "ghost"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("unknown partition error = %v", err)
	}
	if err := lead.ChangeVocabulary(ctx, api.VocabularyChange{Op: "define_term", Term: &vocab.Term{Term: "order", Definition: "A request."}}); err != nil {
		t.Fatal(err)
	}
	if e := <-events; e.Type != "vocabulary" || len(e.Vocabulary.Terms) != 1 {
		t.Fatalf("vocabulary event = %+v", e)
	}
	if v, err := orders.GetVocabulary(ctx); err != nil || len(v.Terms) != 1 {
		t.Errorf("a partition agent reads the vocabulary: %+v, %v", v, err)
	}
	if err := orders.ChangeVocabulary(ctx, api.VocabularyChange{Op: "remove", Name: "order"}); err == nil {
		t.Error("a partition agent changed the vocabulary")
	}
	if commit, err := orders.Commit(ctx, "Add the map"); err != nil || commit == "" {
		t.Errorf("commit = %q, %v", commit, err)
	}
	if tc, err := orders.TurnContext(ctx); err != nil || tc.MapState != featuremap.Unbuilt {
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
  orchestration: lead
services:
  lead: {x-hikari: {}}
  orders: {x-hikari: {partition: {repo: shop, scope: orders}, grants: {deliverables: [lib]}}}
`), dir)
	if err != nil {
		t.Fatal(err)
	}
	deliverable := filepath.Join(dir, "vault", "public", "deliverables", "lib")
	os.MkdirAll(deliverable, 0o755)
	os.WriteFile(filepath.Join(deliverable, "deliverable.yaml"), []byte("version: 1\ndescription: The library.\nlayout: {require: [README.md]}\n"), 0o644)
	os.WriteFile(filepath.Join(deliverable, "SKILL.md"), []byte("# lib\n"), 0o644)
	sys, err := partitions.New(topo)
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
	staging := filepath.Join(dir, "vault", "partitions", "orders", "staging", "lib")
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
	if len(msgs) != 1 || msgs[0].From != "orders" || msgs[0].To != "" || msgs[0].Body != "Released orders lib 1.0.0: First release." || !slices.Contains(msgs[0].Woke, "lead") {
		t.Errorf("R18 announcement = %+v", msgs)
	}
	self := s.Self("orders")
	if len(self.Deliverables) != 1 || self.Deliverables[0].Description != "The library." {
		t.Errorf("Self.Deliverables = %+v", self.Deliverables)
	}
	if lead := s.Self("lead"); len(lead.PartitionAgentDeliverables["orders"]) != 1 {
		t.Errorf("Self.OwnerDeliverables = %+v", lead.PartitionAgentDeliverables)
	}
}

func TestVaultRoutes(t *testing.T) {
	topo := parseRoom(t, testTopology)
	sys, err := partitions.New(topo)
	if err != nil {
		t.Fatal(err)
	}
	s := New(topo, nil, sys)
	srv := httptest.NewServer(s.Handler("ui-secret"))
	defer srv.Close()
	root := sys.Vault().Root
	os.MkdirAll(filepath.Join(root, "skills", "style"), 0o755)
	os.WriteFile(filepath.Join(root, "skills", "style", "SKILL.md"), []byte("# style\n"), 0o644)
	os.WriteFile(filepath.Join(root, "public", "policies", "security.md"), []byte("Never log secrets.\n"), 0o644)
	os.WriteFile(filepath.Join(root, "public", "policies", "logo.png"), []byte{0x89, 0x50, 0x00}, 0o644)
	os.WriteFile(filepath.Join(root, "public", "policies", "binary.md"), []byte{0x00, 0x01}, 0o644)
	outside := filepath.Join(t.TempDir(), "secret.txt")
	os.WriteFile(outside, []byte("secret"), 0o644)
	os.Symlink(outside, filepath.Join(root, "public", "policies", "escape.md"))
	os.MkdirAll(filepath.Join(root, "public", "deliverables", "docs-site"), 0o755)
	os.WriteFile(filepath.Join(root, "public", "deliverables", "docs-site", "deliverable.yaml"), []byte("version: 1\ndescription: A site.\ntargets: [{type: dockerhub}]\n"), 0o644)
	os.WriteFile(filepath.Join(root, "public", "deliverables", "docs-site", "SKILL.md"), []byte("# site\n"), 0o644)
	os.MkdirAll(filepath.Join(root, "public", "deliverables", "broken"), 0o755)

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

	code, body := get("/api/ui/vault/public")
	if code != 200 || !strings.Contains(body, `"name":"style","path":"skills/style/SKILL.md","holders":[]`) || !strings.Contains(body, `"path":"public/policies/security.md"`) || strings.Contains(body, "logo") {
		t.Errorf("V13 public listing = %d %s", code, body)
	}
	if !strings.Contains(body, `"name":"docs-site","path":"public/deliverables/docs-site","skill":"public/deliverables/docs-site/SKILL.md","description":"A site."`) ||
		!strings.Contains(body, `"targets":[{"type":"dockerhub"}],"holders":[]`) || !strings.Contains(body, `"name":"broken"`) || !strings.Contains(body, `"error":"deliverable \"broken\" has no deliverable.yaml`) {
		t.Errorf("U12 deliverables listing = %s", body)
	}
	if code, body := get("/api/ui/vault/file?path=public/policies/security.md"); code != 200 || body != "Never log secrets.\n" {
		t.Errorf("text file = %d %q", code, body)
	}
	for path, want := range map[string]int{
		"../../etc/passwd":           400,
		"/etc/passwd":                400,
		"public/policies/escape.md":  400,
		"public/policies/binary.md":  415,
		"public/policies/missing.md": 404,
		"public/policies":            404,
	} {
		if code, _ := get("/api/ui/vault/file?path=" + path); code != want {
			t.Errorf("U10: %s = %d, want %d", path, code, want)
		}
	}
	if code, _ := get("/api/ui/vault/releases/nope/lib/1.0.0"); code != 404 {
		t.Errorf("unknown release = %d", code)
	}
	resp, _ := http.Get(srv.URL + "/api/ui/vault/public")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("the vault needs the UI token: %s", resp.Status)
	}
}

func TestActivity(t *testing.T) {
	s, _ := newTestServer(t)
	events, cancel := s.Subscribe()
	defer cancel()
	<-events // the snapshot

	s.AddActivity("writer", api.Activity{ID: 1, Turn: 1, Kind: api.ActivityTurn, Text: "woken by user in the room"})
	s.AddActivity("writer", api.Activity{ID: 2, Turn: 1, Kind: api.ActivityTool, Tool: "Read"})
	s.AddActivity("writer", api.Activity{ID: 2, Turn: 1, Kind: api.ActivityTool, Tool: "Read", Output: "ok", Done: true})
	s.AddActivity("writer", api.Activity{ID: 3, Turn: 1, Kind: api.ActivityDone, Text: "Should I add it?", Done: true})
	for range 4 {
		if e := <-events; e.Type != "activity" || e.Agent != "writer" {
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
	s.Send("writer", post("done"))
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

// TestRenamedAgentsRouteByName checks that messages, mentions, and prompts
// use names (rule ID12 of the ids spec).
func TestRenamedAgentsRouteByName(t *testing.T) {
	src := strings.NewReplacer(
		"writer: {x-hikari: {description: writes,", "writer: {x-hikari: {name: scribe, description: writes,",
		"lead: {x-hikari: {description: leads engineering}}", "lead: {x-hikari: {name: chief, description: leads engineering}}",
	).Replace(testTopology)
	s, _ := newServerOf(t, src)
	m, err := s.Send("chief", post("@scribe draft it"))
	if err != nil || !slices.Equal(m.Woke, []string{"scribe"}) {
		t.Errorf("a post wakes the renamed agent: %+v, %v", m, err)
	}
	if m, err := s.Send("scribe", post("done")); err != nil || !slices.Equal(m.Woke, []string{"chief"}) {
		t.Errorf("a post wakes the renamed orchestration agent: %+v, %v", m, err)
	}
	if _, err := s.Send("chief", dm("writer", "hi")); err == nil {
		t.Error("an agent's id is not an address")
	}
	self := s.Self("scribe")
	if self.Name != "scribe" || self.Participants[0].Name != "chief" || self.Orchestration {
		t.Errorf("Self(scribe) = %+v", self)
	}
	if snap := s.snapshot(); snap.Project != "test" {
		t.Errorf("ID13: the snapshot names the project: %q", snap.Project)
	}
}
