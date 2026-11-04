package forge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	cases := []struct {
		url   string
		owner string
		name  string
		api   string
		ok    bool
	}{
		{"https://github.com/nathanaday/hikari", "nathanaday", "hikari", "https://api.github.com", true},
		{"https://github.com/nathanaday/hikari.git", "nathanaday", "hikari", "https://api.github.com", true},
		{"git@github.com:nathanaday/hikari.git", "nathanaday", "hikari", "https://api.github.com", true},
		{"ssh://git@github.com/nathanaday/hikari.git", "nathanaday", "hikari", "https://api.github.com", true},
		{"https://github.example.com/team/app.git", "team", "app", "https://github.example.com/api/v3", true},
		{"git@github.example.com:team/app", "team", "app", "https://github.example.com/api/v3", true},
		{"https://gitlab.com/team/app.git", "", "", "", false},
		{"file:///tmp/remote.git", "", "", "", false},
		{"https://github.com/onlyowner", "", "", "", false},
		{"", "", "", "", false},
		{"../relative/path", "", "", "", false},
	}
	for _, c := range cases {
		r, ok := Parse(c.url)
		if ok != c.ok {
			t.Errorf("Parse(%q) ok = %v, want %v", c.url, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		if r.Kind != KindGitHub || r.Owner != c.owner || r.Name != c.name || r.API != c.api {
			t.Errorf("Parse(%q) = %+v", c.url, r)
		}
		if r.Slug() != c.owner+"/"+c.name || !strings.HasSuffix(r.Web, c.owner+"/"+c.name) {
			t.Errorf("Parse(%q): slug %q, web %q", c.url, r.Slug(), r.Web)
		}
	}
}

func TestClosedIssuesAndSummary(t *testing.T) {
	got := ClosedIssues("This PR closes #12 and Fixes: #7.\nResolves #12 again, and mentions #99.")
	if !slices.Equal(got, []int{12, 7, 12}) {
		t.Errorf("ClosedIssues = %v", got)
	}
	c := Summarize([]Check{{Name: "a", State: Passing}, {Name: "b", State: Pending}, {Name: "c", State: Skipped}})
	if c.State != Pending || c.Passed != 1 || c.Pending != 1 || c.Failed != 0 {
		t.Errorf("Summarize = %+v", c)
	}
	if Summarize(nil).State != None || Summarize([]Check{{State: Failing}, {State: Passing}}).State != Failing {
		t.Error("Summarize states")
	}
}

// fakeGitHub answers the routes that GitHub uses, and records the writes.
type fakeGitHub struct {
	mu     sync.Mutex
	t      *testing.T
	writes []string
	labels map[string]bool
}

func (f *fakeGitHub) handler() http.Handler {
	mux := http.NewServeMux()
	page := func(r *http.Request) int {
		var n int
		fmt.Sscanf(r.URL.Query().Get("page"), "%d", &n)
		return max(n, 1)
	}
	record := func(r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		b, _ := json.Marshal(body)
		f.writes = append(f.writes, r.Method+" "+r.URL.Path+" "+string(b))
	}
	auth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer tok" {
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"message": "Bad credentials"})
				return
			}
			next(w, r)
		}
	}
	mux.HandleFunc("GET /repos/o/r", auth(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"default_branch": "main"})
	}))
	mux.HandleFunc("GET /repos/o/r/pulls", auth(func(w http.ResponseWriter, r *http.Request) {
		if page(r) > 1 {
			json.NewEncoder(w).Encode([]any{})
			return
		}
		json.NewEncoder(w).Encode([]map[string]any{
			{"number": 5, "title": "Cancel orders", "body": "Closes #12", "html_url": "https://github.com/o/r/pull/5", "state": "open", "draft": false,
				"user": map[string]string{"login": "alex"}, "head": map[string]string{"ref": "hikari", "sha": "abc"}, "base": map[string]string{"ref": "main", "sha": "def"},
				"updated_at": "2026-11-04T10:00:00Z", "created_at": "2026-11-03T10:00:00Z"},
			{"number": 4, "title": "Old", "html_url": "https://github.com/o/r/pull/4", "state": "closed", "merged_at": "2026-11-01T10:00:00Z",
				"user": map[string]string{"login": "bob"}, "head": map[string]string{"ref": "feat", "sha": "111"}, "base": map[string]string{"ref": "main", "sha": "def"},
				"updated_at": "2026-11-01T10:00:00Z"},
			{"number": 1, "title": "Ancient", "html_url": "https://github.com/o/r/pull/1", "state": "closed",
				"head": map[string]string{"ref": "x", "sha": "222"}, "base": map[string]string{"ref": "main"}, "updated_at": "2025-01-01T10:00:00Z"},
		})
	}))
	mux.HandleFunc("GET /repos/o/r/pulls/5", auth(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"number": 5, "state": "open", "mergeable": true, "additions": 10, "deletions": 2, "changed_files": 3,
			"head": map[string]string{"ref": "hikari", "sha": "abc"}, "base": map[string]string{"ref": "main"}})
	}))
	mux.HandleFunc("GET /repos/o/r/pulls/5/reviews", auth(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"user": map[string]string{"login": "carol"}, "state": "CHANGES_REQUESTED"},
			{"user": map[string]string{"login": "carol"}, "state": "APPROVED"},
			{"user": map[string]string{"login": "dan"}, "state": "COMMENTED"},
		})
	}))
	mux.HandleFunc("GET /repos/o/r/commits/abc/check-runs", auth(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"check_runs": []map[string]any{
			{"name": "go test", "status": "completed", "conclusion": "success", "html_url": "u1"},
			{"name": "lint", "status": "in_progress", "conclusion": nil, "html_url": "u2"},
			{"name": "docs", "status": "completed", "conclusion": "skipped"},
		}})
	}))
	mux.HandleFunc("GET /repos/o/r/commits/abc/status", auth(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"statuses": []map[string]any{{"context": "ci/legacy", "state": "failure", "target_url": "u3"}}})
	}))
	mux.HandleFunc("PUT /repos/o/r/pulls/5/merge", auth(func(w http.ResponseWriter, r *http.Request) {
		record(r)
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"message": "Pull Request is not mergeable"})
	}))
	mux.HandleFunc("POST /repos/o/r/pulls", auth(func(w http.ResponseWriter, r *http.Request) {
		record(r)
		json.NewEncoder(w).Encode(map[string]any{"number": 6, "title": "New", "state": "open", "html_url": "https://github.com/o/r/pull/6",
			"head": map[string]string{"ref": "hikari", "sha": "abc"}, "base": map[string]string{"ref": "main"}})
	}))
	mux.HandleFunc("GET /repos/o/r/issues", auth(func(w http.ResponseWriter, r *http.Request) {
		if page(r) > 1 {
			json.NewEncoder(w).Encode([]any{})
			return
		}
		json.NewEncoder(w).Encode([]map[string]any{
			{"number": 12, "title": "Gift wrap", "body": "Wrap gifts.", "html_url": "https://github.com/o/r/issues/12", "state": "open",
				"user": map[string]string{"login": "alex"}, "labels": []map[string]string{{"name": "feature"}}, "comments": 2,
				"created_at": "2026-11-01T10:00:00Z", "updated_at": "2026-11-04T10:00:00Z"},
			{"number": 5, "title": "A PR", "state": "open", "pull_request": map[string]string{"url": "x"}, "updated_at": "2026-11-04T10:00:00Z"},
			{"number": 3, "title": "Closed long ago", "state": "closed", "updated_at": "2025-01-01T10:00:00Z", "closed_at": "2025-01-01T10:00:00Z"},
		})
	}))
	mux.HandleFunc("GET /repos/o/r/issues/12", auth(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"number": 12, "title": "Gift wrap", "body": "Wrap gifts.", "state": "open", "labels": []any{}})
	}))
	mux.HandleFunc("GET /repos/o/r/issues/12/comments", auth(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{{"id": 1, "user": map[string]string{"login": "bob"}, "body": "Why?", "created_at": "2026-11-02T10:00:00Z"}})
	}))
	mux.HandleFunc("POST /repos/o/r/issues", auth(func(w http.ResponseWriter, r *http.Request) {
		record(r)
		json.NewEncoder(w).Encode(map[string]any{"number": 13, "title": "New issue", "state": "open", "html_url": "https://github.com/o/r/issues/13", "labels": []any{}})
	}))
	mux.HandleFunc("PATCH /repos/o/r/issues/12", auth(func(w http.ResponseWriter, r *http.Request) {
		record(r)
		json.NewEncoder(w).Encode(map[string]any{"number": 12, "title": "Gift wrap", "state": "closed", "labels": []any{}})
	}))
	mux.HandleFunc("POST /repos/o/r/issues/12/comments", auth(func(w http.ResponseWriter, r *http.Request) {
		record(r)
		json.NewEncoder(w).Encode(map[string]any{"id": 2, "body": "hi", "user": map[string]string{"login": "hikari"}})
	}))
	mux.HandleFunc("POST /repos/o/r/labels", auth(func(w http.ResponseWriter, r *http.Request) {
		var l struct{ Name string }
		json.NewDecoder(r.Body).Decode(&l)
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.labels[l.Name] {
			w.WriteHeader(http.StatusUnprocessableEntity)
			json.NewEncoder(w).Encode(map[string]any{"message": "Validation Failed", "errors": []map[string]string{{"code": "already_exists", "field": "name"}}})
			return
		}
		f.labels[l.Name] = true
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(l)
	}))
	mux.HandleFunc("POST /repos/o/r/issues/12/labels", auth(func(w http.ResponseWriter, r *http.Request) {
		record(r)
		json.NewEncoder(w).Encode([]any{})
	}))
	mux.HandleFunc("DELETE /repos/o/r/issues/12/labels/{name}", auth(func(w http.ResponseWriter, r *http.Request) {
		record(r)
		if r.PathValue("name") == "hikari:done" {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"message": "Label does not exist"})
			return
		}
		json.NewEncoder(w).Encode([]any{})
	}))
	return mux
}

func newFake(t *testing.T) (*GitHub, *fakeGitHub) {
	f := &fakeGitHub{t: t, labels: map[string]bool{"feature": true}}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	g, _ := NewGitHub(Remote{Kind: KindGitHub, Host: "github.com", Owner: "o", Name: "r", API: srv.URL, Web: "https://github.com/o/r"}, "tok")
	g.Now = func() time.Time { return time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC) }
	return g, f
}

func TestGitHubReads(t *testing.T) {
	g, _ := newFake(t)
	ctx := context.Background()
	if b, err := g.DefaultBranch(ctx); err != nil || b != "main" {
		t.Errorf("DefaultBranch = %q, %v", b, err)
	}
	prs, err := g.PullRequests(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 2 {
		t.Fatalf("PullRequests: %d, want the open one and the recently merged one, not the ancient one", len(prs))
	}
	open := prs[0]
	if open.Number != 5 || open.State != Open || open.Head != "hikari" || open.SHA != "abc" || open.Author != "alex" || !slices.Equal(open.Closes, []int{12}) {
		t.Errorf("open PR = %+v", open)
	}
	if open.Mergeable != "mergeable" || open.Additions != 10 || open.Files != 3 || open.Review != "approved" {
		t.Errorf("open PR detail = mergeable %q, +%d, %d files, review %q", open.Mergeable, open.Additions, open.Files, open.Review)
	}
	if prs[1].State != Merged || prs[1].Merged.IsZero() {
		t.Errorf("merged PR = %+v", prs[1])
	}
	checks, err := g.Checks(ctx, "abc")
	if err != nil {
		t.Fatal(err)
	}
	if checks.State != Failing || checks.Passed != 1 || checks.Pending != 1 || checks.Failed != 1 || len(checks.Runs) != 4 {
		t.Errorf("Checks = %+v", checks)
	}
	issues, err := g.Issues(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || issues[0].Number != 12 || !slices.Equal(issues[0].Labels, []string{"feature"}) || issues[0].Comments != 2 {
		t.Errorf("Issues = %+v; want only the open issue, not the pull request or the old one", issues)
	}
	issue, comments, err := g.Issue(ctx, 12)
	if err != nil || issue.Title != "Gift wrap" || len(comments) != 1 || comments[0].Author != "bob" {
		t.Errorf("Issue = %+v, %+v, %v", issue, comments, err)
	}
}

func TestGitHubWrites(t *testing.T) {
	g, f := newFake(t)
	ctx := context.Background()
	pr, err := g.CreatePullRequest(ctx, NewPullRequest{Title: "New", Head: "hikari", Base: "main", Body: "Closes #12"})
	if err != nil || pr.Number != 6 {
		t.Errorf("CreatePullRequest = %+v, %v", pr, err)
	}
	if err := g.Merge(ctx, 5, MergeSquash); err == nil || !strings.Contains(err.Error(), "not mergeable") {
		t.Errorf("Merge of a conflicting PR: %v", err)
	}
	if err := g.Merge(ctx, 5, "fast"); err == nil {
		t.Error("an unknown merge method is refused before any call")
	}
	issue, err := g.CreateIssue(ctx, NewIssue{Title: "New issue", Body: "b"})
	if err != nil || issue.Number != 13 {
		t.Errorf("CreateIssue = %+v, %v", issue, err)
	}
	closed := Closed
	if _, err := g.UpdateIssue(ctx, 12, IssuePatch{State: &closed}); err != nil {
		t.Error(err)
	}
	if _, err := g.Comment(ctx, 12, "hi"); err != nil {
		t.Error(err)
	}
	if err := g.AddLabels(ctx, 12, "feature", "hikari:working"); err != nil {
		t.Error(err)
	}
	if !f.labels["hikari:working"] {
		t.Error("AddLabels makes the label that does not exist")
	}
	if err := g.RemoveLabel(ctx, 12, "hikari:done"); err != nil {
		t.Errorf("removing a label the issue lacks is not an error: %v", err)
	}
	if err := g.RemoveLabel(ctx, 12, "hikari:queued"); err != nil {
		t.Error(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	want := []string{
		`POST /repos/o/r/pulls {"base":"main","body":"Closes #12","head":"hikari","title":"New"}`,
		`PUT /repos/o/r/pulls/5/merge {"merge_method":"squash"}`,
		`POST /repos/o/r/issues {"body":"b","title":"New issue"}`,
		`PATCH /repos/o/r/issues/12 {"state":"closed"}`,
		`POST /repos/o/r/issues/12/comments {"body":"hi"}`,
		`POST /repos/o/r/issues/12/labels {"labels":["feature","hikari:working"]}`,
		`DELETE /repos/o/r/issues/12/labels/hikari:done null`,
		`DELETE /repos/o/r/issues/12/labels/hikari:queued null`,
	}
	if !slices.Equal(f.writes, want) {
		t.Errorf("writes:\n%s\nwant:\n%s", strings.Join(f.writes, "\n"), strings.Join(want, "\n"))
	}
}

func TestGitHubErrors(t *testing.T) {
	g, _ := newFake(t)
	g.token = "wrong"
	_, err := g.Issues(context.Background())
	if err == nil || !strings.Contains(err.Error(), "Bad credentials") {
		t.Errorf("a GitHub error names GitHub's message: %v", err)
	}
	none, _ := NewGitHub(g.remote, "")
	if _, err := none.Issues(context.Background()); !errors.Is(err, ErrNoCredential) {
		t.Errorf("without a token: %v", err)
	}
}

func TestTokenFor(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	old := ghToken
	defer func() { ghToken = old }()
	calls := 0
	ghToken = func(host string) string { calls++; return "from-gh-" + host }
	tokenMu.Lock()
	delete(tokenCache, "ghe.test")
	tokenMu.Unlock()
	if got := TokenFor("ghe.test"); got != "from-gh-ghe.test" {
		t.Errorf("TokenFor = %q", got)
	}
	TokenFor("ghe.test")
	if calls != 1 {
		t.Errorf("gh auth token ran %d times; the answer is cached", calls)
	}
	t.Setenv("GH_TOKEN", "env")
	if got := TokenFor("ghe.test"); got != "env" {
		t.Errorf("the environment wins: %q", got)
	}
	if _, err := New(Remote{Kind: "gitlab"}); !errors.Is(err, ErrUnknownForge) {
		t.Errorf("New of an unknown kind: %v", err)
	}
}
