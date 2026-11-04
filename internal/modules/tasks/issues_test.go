package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nathanaday/hikari/internal/forge"
	"github.com/nathanaday/hikari/internal/module"
)

// fakeTracker is an in-memory issue tracker that records its writes.
type fakeTracker struct {
	mu       sync.Mutex
	issues   []forge.Issue
	comments map[int][]forge.Comment
	labels   map[int][]string
	writes   []string
	fail     error
}

func newTracker(issues ...forge.Issue) *fakeTracker {
	return &fakeTracker{issues: issues, comments: map[int][]forge.Comment{}, labels: map[int][]string{}}
}

func (f *fakeTracker) Remote() forge.Remote {
	return forge.Remote{Kind: forge.KindGitHub, Host: "github.com", Owner: "acme", Name: "shop", Web: "https://github.com/acme/shop"}
}

func (f *fakeTracker) Issues(context.Context) ([]forge.Issue, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return nil, f.fail
	}
	out := slices.Clone(f.issues)
	for i := range out {
		out[i].Labels = slices.Clone(f.labels[out[i].Number])
	}
	return out, nil
}

func (f *fakeTracker) Issue(_ context.Context, n int) (forge.Issue, []forge.Comment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, is := range f.issues {
		if is.Number == n {
			is.Labels = slices.Clone(f.labels[n])
			return is, slices.Clone(f.comments[n]), nil
		}
	}
	return forge.Issue{}, nil, fmt.Errorf("no issue #%d", n)
}

func (f *fakeTracker) CreateIssue(_ context.Context, in forge.NewIssue) (forge.Issue, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	is := forge.Issue{Number: len(f.issues) + 1, Title: in.Title, Body: in.Body, State: forge.Open, Labels: []string{}, URL: fmt.Sprintf("https://github.com/acme/shop/issues/%d", len(f.issues)+1), Author: "hikari-user"}
	f.issues = append(f.issues, is)
	f.writes = append(f.writes, "create "+in.Title)
	return is, nil
}

func (f *fakeTracker) UpdateIssue(_ context.Context, n int, p forge.IssuePatch) (forge.Issue, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.issues {
		if f.issues[i].Number == n {
			if p.Title != nil {
				f.issues[i].Title = *p.Title
			}
			if p.Body != nil {
				f.issues[i].Body = *p.Body
			}
			if p.State != nil {
				f.issues[i].State = *p.State
			}
			f.writes = append(f.writes, fmt.Sprintf("update #%d", n))
			return f.issues[i], nil
		}
	}
	return forge.Issue{}, fmt.Errorf("no issue #%d", n)
}

func (f *fakeTracker) Comment(_ context.Context, n int, body string) (forge.Comment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := forge.Comment{ID: int64(len(f.comments[n]) + 1), Author: "hikari", Body: body}
	f.comments[n] = append(f.comments[n], c)
	f.writes = append(f.writes, fmt.Sprintf("comment #%d: %s", n, firstLine(body)))
	return c, nil
}

func (f *fakeTracker) AddLabels(_ context.Context, n int, labels ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, l := range labels {
		if !slices.Contains(f.labels[n], l) {
			f.labels[n] = append(f.labels[n], l)
		}
	}
	f.writes = append(f.writes, fmt.Sprintf("label #%d +%s", n, strings.Join(labels, ",")))
	return nil
}

func (f *fakeTracker) RemoveLabel(_ context.Context, n int, label string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.labels[n] = slices.DeleteFunc(f.labels[n], func(l string) bool { return l == label })
	return nil
}

func firstLine(s string) string {
	l, _, _ := strings.Cut(s, "\n")
	return l
}

// wait polls until cond holds, for the hooks that run in the background.
func wait(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// call sends one request to a route of the instance.
func call(t *testing.T, routes module.Routes, method, p, caller string, in any) (any, error) {
	t.Helper()
	mux := http.NewServeMux()
	for pattern, h := range routes {
		m, path, _ := strings.Cut(pattern, " ")
		mux.HandleFunc(m+" "+path, func(w http.ResponseWriter, r *http.Request) {
			out, err := h(r, caller)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, err.Error())
				return
			}
			json.NewEncoder(w).Encode(out)
		})
	}
	var body strings.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = *strings.NewReader(string(b))
	}
	req := httptest.NewRequest(method, p, &body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		return nil, fmt.Errorf("%s %s: %s", method, p, rec.Body.String())
	}
	var out any
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out, nil
}

// TestIssues checks IS1 to IS6 and decisions 9 and 10: an issue of a
// tracker enters the work queue with one click, the orchestration agent
// scopes it after it starts, no other queue starts meanwhile, and hikari
// writes each moment to the issue.
func TestIssues(t *testing.T) {
	h := shopHost(t)
	tr := newTracker(
		forge.Issue{Number: 12, Title: "Gift wrap", Body: "Wrap gifts at checkout.", State: forge.Open, Labels: []string{"feature"}, Author: "alex", URL: "https://github.com/acme/shop/issues/12", Updated: clock},
		forge.Issue{Number: 7, Title: "Old", State: forge.Closed, Labels: []string{}, Updated: clock.Add(-time.Hour)},
	)
	h.trackers = map[string]forge.Tracker{"shop": tr}
	mi, err := Module{}.Open(h)
	if err != nil {
		t.Fatal(err)
	}
	i := mi.(*instance)
	i.sync(context.Background())
	v := i.View().(View)
	if len(v.Trackers) != 1 || v.Trackers[0].Repo != "shop" || v.Trackers[0].Kind != "github" || v.Trackers[0].Synced.IsZero() {
		t.Errorf("IS2 trackers = %+v", v.Trackers)
	}
	if len(v.Issues) != 2 || v.Issues[0].Ref != "shop#12" || v.Issues[0].Task != nil || v.Issues[1].State != forge.Closed {
		t.Errorf("IS2 issues = %+v", v.Issues)
	}

	// Work on this: the issue gets a task, the work queue, and a start.
	ui := i.UIRoutes()
	if _, err := call(t, ui, http.MethodPost, "/work", User, WorkRequest{Issue: "shop#12"}); err != nil {
		t.Fatal(err)
	}
	task := i.store.Tasks["shop-12"]
	if task == nil || task.Issue != "shop#12" || task.Name != "Gift wrap" || task.Author != "alex" || !strings.Contains(task.Body, "Wrap gifts") {
		t.Fatalf("IS1 the issue's task: %+v", task)
	}
	if task.Status != Started || task.Stage != StageStub || i.store.QueueOf("shop-12") != WorkQueue || i.store.Queues[WorkQueue].State != Running {
		t.Errorf("decision 9: %+v, queue %+v", task, i.store.Queues[WorkQueue])
	}
	if n := h.notes; len(n) != 1 || n[0].To != "lead" || !strings.Contains(n[0].Body, "not scoped yet") || !strings.Contains(n[0].Body, `shop#12 "Gift wrap"`) {
		t.Errorf("IS3 lead is told to scope it: %+v", n)
	}
	if n := i.Notices("lead"); len(n) != 1 || !strings.Contains(n[0], "waits for you to scope it") {
		t.Errorf("the notice: %v", n)
	}
	wait(t, "the start hook", func() bool {
		tr.mu.Lock()
		defer tr.mu.Unlock()
		return slices.Contains(tr.labels[12], LabelWorking) && len(tr.comments[12]) == 1
	})
	tr.mu.Lock()
	if !strings.Contains(tr.comments[12][0].Body, "started this issue from the queue `work`") {
		t.Errorf("decision 10 the start comment: %q", tr.comments[12][0].Body)
	}
	tr.mu.Unlock()

	// A second queue waits while the task is being scoped.
	i.do("plant", func(c *Change) (any, error) {
		_, paths, err := i.store.PlantStub("Local idea", "", User)
		c.wrote(User, paths...)
		return nil, err
	})
	i.do("queue", func(c *Change) (any, error) {
		p1, err := i.store.ChangeQueue(QueueOp{Op: "create", Name: "API"})
		c.wrote(User, p1...)
		p2, err2 := i.store.ChangeQueue(QueueOp{Op: "add", Queue: "api", Scope: "local-idea"})
		c.wrote(User, p2...)
		return nil, errorsJoin(err, err2)
	})
	i.do("start", func(c *Change) (any, error) { return nil, i.store.SetQueueState("api", Running, c) })
	if local := i.store.Tasks["local-idea"]; local.Status != Waiting || !strings.Contains(local.Wait, "[[shop-12]] is being scoped") {
		t.Errorf("IS3 the other queue waits: %+v", local)
	}

	// The orchestration agent scopes the started task, by its issue ref.
	agent := i.AgentRoutes()
	out, err := call(t, agent, http.MethodPost, "/scope", "lead", ScopeInput{ID: "shop#12", Partitions: []string{"orders"}, Features: []string{"create-order"}, Goal: "Gifts get wrapped.", Parts: map[string]string{"orders": "Add a wrap option to create-order."}, DoneWhen: "create-order passes.", Answers: "Assumed one wrap style."})
	if err != nil {
		t.Fatal(err)
	}
	if m := out.(map[string]any); m["status"] != Started || m["stage"] != StageScope {
		t.Errorf("IS4 the scope keeps the started status: %v", m)
	}
	task = i.store.Tasks["shop-12"]
	if task.PartStatus("orders") != Queued || !strings.Contains(task.Body, "## Request\n\nWrap gifts at checkout.") {
		t.Errorf("IS4 parts queued and the request kept: %+v\n%s", task.Parts, task.Body)
	}
	if local := i.store.Tasks["local-idea"]; local.Status != Started {
		t.Errorf("IS3 the other queue starts once the scope is written: %+v", local)
	}
	if _, err := call(t, agent, http.MethodPost, "/scope", "lead", ScopeInput{ID: "shop-12", Partitions: []string{"orders"}, Goal: "g", Parts: map[string]string{"orders": "x"}, DoneWhen: "d"}); err == nil {
		t.Error("IS4 a scoped task in a queue is the user's")
	}
	wait(t, "the scope hook", func() bool {
		tr.mu.Lock()
		defer tr.mu.Unlock()
		return len(tr.comments[12]) == 2
	})
	tr.mu.Lock()
	scope := tr.comments[12][1].Body
	tr.mu.Unlock()
	for _, want := range []string{"scoped this issue as `shop-12`", "**Goal**", "Gifts get wrapped.", "**orders**", "**Done when**", "Assumed one wrap style."} {
		if !strings.Contains(scope, want) {
			t.Errorf("decision 10 the scope comment lacks %q:\n%s", want, scope)
		}
	}

	// A block and done reach the issue too.
	status(t, i.store, "orders", "shop-12", "", Blocked, "need a decision")
	i.do("block", func(c *Change) (any, error) { return nil, nil })
	wait(t, "the block hook", func() bool {
		tr.mu.Lock()
		defer tr.mu.Unlock()
		return slices.Contains(tr.labels[12], LabelBlocked)
	})
	h.health["orders/create-order"] = "passing"
	i.do("done", func(c *Change) (any, error) {
		if _, err := i.store.SetStatus("orders", "shop-12", "", Done, "", c); err != nil {
			return nil, err
		}
		_, err := i.store.SetStatus("lead", "shop-12", "", Done, "validated", c)
		return nil, err
	})
	if i.store.Tasks["shop-12"].Status != Done {
		t.Fatalf("done: %+v", i.store.Tasks["shop-12"])
	}
	wait(t, "the done hook", func() bool {
		tr.mu.Lock()
		defer tr.mu.Unlock()
		return slices.Contains(tr.labels[12], LabelDone) && !slices.Contains(tr.labels[12], LabelBlocked) && !slices.Contains(tr.labels[12], LabelWorking)
	})
	tr.mu.Lock()
	last := tr.comments[12][len(tr.comments[12])-1].Body
	tr.mu.Unlock()
	if !strings.Contains(last, "finished this issue") || !strings.Contains(last, "`hikari` of shop") || !strings.Contains(last, "create-order") {
		t.Errorf("decision 10 the done comment: %q", last)
	}

	// The view joins the issue with its task, and the feed saw nothing fail.
	i.sync(context.Background())
	v = i.View().(View)
	k := slices.IndexFunc(v.Issues, func(is IssueView) bool { return is.Ref == "shop#12" })
	if k < 0 || v.Issues[k].Task == nil || v.Issues[k].Task.Status != Done || !slices.Contains(v.Issues[k].Labels, LabelDone) {
		t.Errorf("IS2 the joined view: %+v", v.Issues)
	}
	if local := v.Issues[slices.IndexFunc(v.Issues, func(is IssueView) bool { return is.Ref == "local-idea" })]; local.Body != "" {
		t.Errorf("a local issue's body holds no log: %q", local.Body)
	}
	for _, l := range h.lines {
		if strings.Contains(l.Text, "could not update") {
			t.Errorf("a hook failed: %s", l.Text)
		}
	}
}

// TestIssueRoutes checks the issue routes: create on the tracker and in
// the vault, read with comments, patch, comment, and the agents' limits.
func TestIssueRoutes(t *testing.T) {
	h := shopHost(t)
	tr := newTracker()
	h.trackers = map[string]forge.Tracker{"shop": tr}
	mi, err := Module{}.Open(h)
	if err != nil {
		t.Fatal(err)
	}
	i := mi.(*instance)
	ui, agent := i.UIRoutes(), i.AgentRoutes()

	out, err := call(t, agent, http.MethodPost, "/issues", "inventory", IssueRequest{Title: "Stock drifts", Body: "Seen in reserve."})
	if err != nil {
		t.Fatal(err)
	}
	if m := out.(map[string]any); m["ref"] != "shop#1" || m["local"] == true {
		t.Errorf("IS7 an agent's issue goes to the tracker: %v", m)
	}
	tr.mu.Lock()
	if !strings.Contains(tr.issues[0].Body, "Opened by the hikari agent inventory") {
		t.Errorf("the body names the agent: %q", tr.issues[0].Body)
	}
	tr.mu.Unlock()
	out, err = call(t, ui, http.MethodPost, "/issues", User, IssueRequest{Title: "Keep local", Local: true})
	if err != nil {
		t.Fatal(err)
	}
	if m := out.(map[string]any); m["ref"] != "keep-local" || m["local"] != true {
		t.Errorf("a local issue: %v", m)
	}
	if _, err := call(t, ui, http.MethodPost, "/issues", User, IssueRequest{Title: "x", Repo: "nope"}); err == nil || !strings.Contains(err.Error(), "no issue tracker") {
		t.Errorf("an unknown repo: %v", err)
	}
	if _, err := call(t, ui, http.MethodPost, "/issues", User, IssueRequest{Title: "  "}); err == nil {
		t.Error("an issue needs a title")
	}

	// Read, with comments, as the orchestration agent; a partition agent
	// may not.
	tr.Comment(context.Background(), 1, "Which store?")
	out, err = call(t, agent, http.MethodGet, "/issues/shop%231", "lead", nil)
	if err != nil {
		t.Fatal(err)
	}
	d := out.(map[string]any)
	if d["issue"].(map[string]any)["title"] != "Stock drifts" || len(d["comments"].([]any)) != 1 {
		t.Errorf("get: %v", d)
	}
	if _, err := call(t, agent, http.MethodGet, "/issues/shop%231", "orders", nil); err == nil || !strings.Contains(err.Error(), "only the orchestration agent") {
		t.Errorf("IS7 a partition agent reads no issue: %v", err)
	}
	if _, err := call(t, agent, http.MethodPost, "/issues/shop%231/comments", "lead", CommentRequest{Body: "Plan: ..."}); err != nil {
		t.Error(err)
	}
	tr.mu.Lock()
	if last := tr.comments[1][len(tr.comments[1])-1].Body; !strings.HasPrefix(last, "**lead** (hikari agent):") {
		t.Errorf("an agent's comment names it: %q", last)
	}
	tr.mu.Unlock()
	if _, err := call(t, ui, http.MethodPost, "/issues/keep-local/comments", User, CommentRequest{Body: "x"}); err == nil {
		t.Error("a local issue takes no comments")
	}

	// Patch: a tracker issue on the tracker, a local one in the vault.
	closed := forge.Closed
	title := "Keep it local"
	if _, err := call(t, ui, http.MethodPatch, "/issues/shop%231", User, IssuePatchRequest{State: &closed}); err != nil {
		t.Error(err)
	}
	if _, err := call(t, ui, http.MethodPatch, "/issues/keep-local", User, IssuePatchRequest{Title: &title}); err != nil {
		t.Error(err)
	}
	if i.store.Tasks["keep-local"].Name != "Keep it local" {
		t.Errorf("the local title: %+v", i.store.Tasks["keep-local"])
	}
	if _, err := call(t, ui, http.MethodPatch, "/issues/keep-local", User, IssuePatchRequest{State: &closed}); err != nil {
		t.Error(err)
	}
	if _, ok := i.store.Tasks["keep-local"]; ok {
		t.Error("closing a local issue removes it")
	}
	tr.mu.Lock()
	if tr.issues[0].State != forge.Closed {
		t.Errorf("the tracker issue is closed: %+v", tr.issues[0])
	}
	tr.mu.Unlock()
	v := i.View().(View)
	if len(v.Issues) != 1 || v.Issues[0].State != forge.Closed {
		t.Errorf("the cache follows the patch: %+v", v.Issues)
	}

	// A queue takes an issue by ref, and a tracker that fails is a
	// tracker view with an error.
	if _, err := call(t, ui, http.MethodPost, "/queue", User, QueueOp{Op: "create", Name: "Q"}); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, ui, http.MethodPost, "/queue", User, QueueOp{Op: "add", Queue: "q", Issue: "shop#1"}); err != nil {
		t.Fatal(err)
	}
	if i.store.QueueOf("shop-1") != "q" || i.store.Tasks["shop-1"].Status != Queued {
		t.Errorf("the issue entered the queue: %+v", i.store.Tasks["shop-1"])
	}
	wait(t, "the queued label", func() bool {
		tr.mu.Lock()
		defer tr.mu.Unlock()
		return slices.Contains(tr.labels[1], LabelQueued)
	})
	tr.mu.Lock()
	tr.fail = fmt.Errorf("github: Bad credentials")
	tr.mu.Unlock()
	i.sync(context.Background())
	if v := i.View().(View); v.Trackers[0].Error == "" || len(v.Issues) != 1 {
		t.Errorf("IS2 a failed sync keeps the last issues and says why: %+v", v.Trackers)
	}
}

// TestNoOrchestration checks IS5: a task that no one can scope is blocked.
func TestNoOrchestration(t *testing.T) {
	check := shopCheck
	s, err := Open(t.TempDir(), check, now)
	if err != nil {
		t.Fatal(err)
	}
	s.PlantStub("Idea", "", User)
	for _, op := range []QueueOp{{Op: "create", Name: "Q"}, {Op: "add", Queue: "q", Scope: "idea"}} {
		if _, err := s.ChangeQueue(op); err != nil {
			t.Fatal(err)
		}
	}
	var c Change
	if err := s.SetQueueState("q", Running, &c); err != nil {
		t.Fatal(err)
	}
	if got := s.Tasks["idea"]; got.Status != Blocked || !strings.Contains(got.Block, "no orchestration agent") {
		t.Errorf("IS5: %+v", got)
	}
}

func TestIssueRefs(t *testing.T) {
	for ref, want := range map[string]bool{"shop#12": true, "shop#0": false, "#12": false, "shop-12": false, "shop#1a": false, "shop#01": false} {
		if _, _, ok := ParseIssueRef(ref); ok != want {
			t.Errorf("ParseIssueRef(%q) = %v", ref, ok)
		}
	}
	if repo, n, _ := ParseIssueRef(" shop#12 "); repo != "shop" || n != 12 || IssueRef(repo, n) != "shop#12" {
		t.Error("round trip")
	}
}
