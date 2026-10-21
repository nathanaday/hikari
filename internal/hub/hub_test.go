package hub

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/topology"
)

// user - alice - bob
const lineTopology = `
nodes:
  alice: {description: lead}
  bob: {description: worker}
edges:
  - [user, alice]
  - [alice, bob]
`

func newTestHub(t *testing.T) (*Hub, *bytes.Buffer) {
	t.Helper()
	topo, err := topology.Parse([]byte(lineTopology))
	if err != nil {
		t.Fatal(err)
	}
	var journal bytes.Buffer
	return New(topo, &journal), &journal
}

func TestSendEnforcesAdjacency(t *testing.T) {
	h, journal := newTestHub(t)
	if _, err := h.Send("alice", "bob", "hi"); err != nil {
		t.Fatalf("alice->bob: %v", err)
	}
	if _, err := h.Send("bob", "user", "hi"); err == nil {
		t.Fatal("bob->user must fail: they are not connected")
	}
	if _, err := h.Send("user", "bob", "hi"); err == nil {
		t.Fatal("user->bob must fail: they are not connected")
	}
	if _, err := h.Send("alice", "user", "  "); err == nil {
		t.Fatal("empty body must fail")
	}
	if n := strings.Count(journal.String(), "\n"); n != 1 {
		t.Fatalf("journal has %d lines, want 1", n)
	}
}

func TestTakeWaitsForMessage(t *testing.T) {
	h, _ := newTestHub(t)
	go func() {
		time.Sleep(20 * time.Millisecond)
		h.Send("alice", "bob", "one")
	}()
	msgs := h.Take(context.Background(), "bob", time.Second)
	if len(msgs) != 1 || msgs[0].Body != "one" {
		t.Fatalf("Take = %+v", msgs)
	}
	if msgs := h.Take(context.Background(), "bob", 10*time.Millisecond); msgs != nil {
		t.Fatalf("second Take = %+v, want nothing", msgs)
	}
}

func TestMessagesToUserAreNotQueued(t *testing.T) {
	h, _ := newTestHub(t)
	h.Send("alice", "user", "report")
	if len(h.inbox[topology.User]) != 0 {
		t.Fatal("the user has no inbox; the UI reads the log")
	}
}

func TestRequeueKeepsOrder(t *testing.T) {
	h, _ := newTestHub(t)
	h.Send("alice", "bob", "one")
	taken := h.Take(context.Background(), "bob", 0)
	h.Send("alice", "bob", "two")
	h.Requeue("bob", taken)
	msgs := h.Take(context.Background(), "bob", 0)
	if len(msgs) != 2 || msgs[0].Body != "one" || msgs[1].Body != "two" {
		t.Fatalf("after requeue = %+v", msgs)
	}
}

func TestNodeAPI(t *testing.T) {
	h, _ := newTestHub(t)
	srv := httptest.NewServer(h.Handler("ui-secret"))
	defer srv.Close()
	ctx := context.Background()

	alice := &api.Client{URL: srv.URL, Token: h.IssueToken("alice"), HTTP: srv.Client()}
	bob := &api.Client{URL: srv.URL, Token: h.IssueToken("bob"), HTTP: srv.Client()}

	self, err := bob.Self(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if self.Name != "bob" || len(self.Neighbors) != 1 || self.Neighbors[0].Name != "alice" {
		t.Fatalf("bob self = %+v", self)
	}

	if _, err := bob.Send(ctx, "user", "sneak"); err == nil || !strings.Contains(err.Error(), "neighbors of \"bob\" are alice") {
		t.Fatalf("bob->user error = %v", err)
	}
	if _, err := alice.Send(ctx, "bob", "task"); err != nil {
		t.Fatal(err)
	}
	msgs, err := bob.Inbox(ctx, time.Second)
	if err != nil || len(msgs) != 1 || msgs[0].From != "alice" {
		t.Fatalf("bob inbox = %+v, %v", msgs, err)
	}

	if err := bob.SetStatus(ctx, api.Status{State: api.StateWorking}); err != nil {
		t.Fatal(err)
	}
	if got := h.status["bob"].State; got != api.StateWorking {
		t.Fatalf("bob status = %q", got)
	}

	forged := &api.Client{URL: srv.URL, Token: "nope", HTTP: srv.Client()}
	if _, err := forged.Self(ctx); err == nil {
		t.Fatal("unknown token must be rejected")
	}
}

func TestUIRequiresToken(t *testing.T) {
	h, _ := newTestHub(t)
	srv := httptest.NewServer(h.Handler("ui-secret"))
	defer srv.Close()

	// A node token must not open the UI API.
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/ui/send", strings.NewReader(`{"to":"alice","body":"x"}`))
	req.Header.Set("Authorization", "Bearer "+h.IssueToken("alice"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("UI send without cookie = %s", resp.Status)
	}

	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar}
	resp, err = browser.Get(srv.URL + "/?token=ui-secret")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("UI login = %v, %v", resp.Status, err)
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
	if e := next(); e.Type != "snapshot" || len(e.Snapshot.Nodes) != 3 {
		t.Fatalf("first event = %+v", e)
	}

	resp, err = browser.Post(srv.URL+"/api/ui/send", "application/json", strings.NewReader(`{"to":"alice","body":"hello"}`))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("UI send = %v, %v", resp.Status, err)
	}
	if e := next(); e.Type != "message" || e.Message.From != "user" || e.Message.To != "alice" {
		t.Fatalf("message event = %+v", e)
	}

	resp, _ = browser.Post(srv.URL+"/api/ui/send", "application/json", strings.NewReader(`{"to":"bob","body":"hello"}`))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("UI send to non-neighbor = %s", resp.Status)
	}
}
