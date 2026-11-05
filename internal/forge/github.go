package forge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GitHub is a repository on GitHub, through its REST API. It implements
// Forge and Tracker.
type GitHub struct {
	remote Remote
	token  string
	HTTP   *http.Client
	// Now is the clock, for the recent-closed window.
	Now func() time.Time
}

// NewGitHub returns the forge of r with token. Without a token, every call
// fails with ErrNoCredential (rule FG4).
func NewGitHub(r Remote, token string) (*GitHub, error) {
	return &GitHub{remote: r, token: token, HTTP: &http.Client{Timeout: 30 * time.Second}, Now: time.Now}, nil
}

func (g *GitHub) Remote() Remote { return g.remote }

// recentWindow is how far back closed pull requests and issues are kept.
const recentWindow = 14 * 24 * time.Hour

// perPage is the page size of each list, and maxPages caps a list.
const (
	perPage  = 100
	maxPages = 5
)

type ghError struct {
	Message string `json:"message"`
	Errors  []struct {
		Message string `json:"message"`
		Field   string `json:"field"`
		Code    string `json:"code"`
	} `json:"errors"`
}

// do sends one request and decodes its answer into out (rule FG5). The
// path is relative to the API base, and starts with /.
func (g *GitHub) do(ctx context.Context, method, p string, in, out any) (*http.Response, error) {
	if g.token == "" {
		return nil, fmt.Errorf("%w: set GITHUB_TOKEN, or sign in with gh auth login", ErrNoCredential)
	}
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, g.remote.API+p, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "hikari")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := g.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 300 {
		var e ghError
		msg := resp.Status
		if json.Unmarshal(data, &e) == nil && e.Message != "" {
			msg = e.Message
			for _, d := range e.Errors {
				if d.Message != "" {
					msg += ": " + d.Message
				}
			}
		}
		return resp, fmt.Errorf("github: %s %s: %s", method, p, msg)
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return resp, fmt.Errorf("github: %s %s: %w", method, p, err)
		}
	}
	return resp, nil
}

func (g *GitHub) repoPath(rest string) string {
	return "/repos/" + url.PathEscape(g.remote.Owner) + "/" + url.PathEscape(g.remote.Name) + rest
}

// list reads the pages of a list until a page is short, or maxPages, or
// stop says that the rest is too old.
func (g *GitHub) list(ctx context.Context, p string, stop func(json.RawMessage) bool, each func(json.RawMessage)) error {
	for page := 1; page <= maxPages; page++ {
		var items []json.RawMessage
		sep := "?"
		if strings.Contains(p, "?") {
			sep = "&"
		}
		if _, err := g.do(ctx, http.MethodGet, fmt.Sprintf("%s%sper_page=%d&page=%d", p, sep, perPage, page), nil, &items); err != nil {
			return err
		}
		for _, it := range items {
			if stop != nil && stop(it) {
				return nil
			}
			each(it)
		}
		if len(items) < perPage {
			return nil
		}
	}
	return nil
}

type ghUser struct {
	Login string `json:"login"`
}

type ghRef struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}

type ghPull struct {
	Number         int       `json:"number"`
	Title          string    `json:"title"`
	Body           string    `json:"body"`
	HTMLURL        string    `json:"html_url"`
	State          string    `json:"state"`
	Draft          bool      `json:"draft"`
	User           ghUser    `json:"user"`
	Head           ghRef     `json:"head"`
	Base           ghRef     `json:"base"`
	MergedAt       *string   `json:"merged_at"`
	Mergeable      *bool     `json:"mergeable"`
	MergeableState string    `json:"mergeable_state"`
	Additions      int       `json:"additions"`
	Deletions      int       `json:"deletions"`
	ChangedFiles   int       `json:"changed_files"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (p ghPull) pullRequest() PullRequest {
	pr := PullRequest{
		Number: p.Number, Title: p.Title, Body: p.Body, URL: p.HTMLURL, State: p.State, Draft: p.Draft,
		Author: p.User.Login, Head: p.Head.Ref, Base: p.Base.Ref, SHA: p.Head.SHA,
		Additions: p.Additions, Deletions: p.Deletions, Files: p.ChangedFiles,
		Created: p.CreatedAt, Updated: p.UpdatedAt, Closes: ClosedIssues(p.Body),
	}
	if p.MergedAt != nil && *p.MergedAt != "" {
		pr.State = Merged
		pr.Merged, _ = time.Parse(time.RFC3339, *p.MergedAt)
	}
	if p.Mergeable != nil {
		if *p.Mergeable {
			pr.Mergeable = "mergeable"
		} else {
			pr.Mergeable = "conflicting"
		}
	} else if p.State == Open {
		pr.Mergeable = "unknown"
	}
	return pr
}

// DefaultBranch asks the repository for its default branch.
func (g *GitHub) DefaultBranch(ctx context.Context) (string, error) {
	var repo struct {
		DefaultBranch string `json:"default_branch"`
	}
	_, err := g.do(ctx, http.MethodGet, g.repoPath(""), nil, &repo)
	return repo.DefaultBranch, err
}

// PullRequests lists the open pull requests and the ones closed within the
// recent window, with the detail and the review decision of each open one.
func (g *GitHub) PullRequests(ctx context.Context) ([]PullRequest, error) {
	since := g.Now().Add(-recentWindow)
	var out []PullRequest
	stop := func(raw json.RawMessage) bool {
		var p ghPull
		if json.Unmarshal(raw, &p) != nil {
			return false
		}
		return p.State != Open && p.UpdatedAt.Before(since)
	}
	err := g.list(ctx, g.repoPath("/pulls?state=all&sort=updated&direction=desc"), stop, func(raw json.RawMessage) {
		var p ghPull
		if json.Unmarshal(raw, &p) == nil {
			out = append(out, p.pullRequest())
		}
	})
	if err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].State != Open {
			continue
		}
		// The list omits mergeability and the sizes.
		var p ghPull
		if _, err := g.do(ctx, http.MethodGet, g.repoPath(fmt.Sprintf("/pulls/%d", out[i].Number)), nil, &p); err == nil {
			detail := p.pullRequest()
			out[i].Mergeable, out[i].Additions, out[i].Deletions, out[i].Files = detail.Mergeable, detail.Additions, detail.Deletions, detail.Files
		}
		out[i].Review = g.reviewDecision(ctx, out[i].Number)
	}
	if out == nil {
		out = []PullRequest{}
	}
	return out, nil
}

// reviewDecision reads the latest review of each reviewer: changes
// requested wins over approved, and no decision gives "".
func (g *GitHub) reviewDecision(ctx context.Context, number int) string {
	var reviews []struct {
		User  ghUser `json:"user"`
		State string `json:"state"`
	}
	if _, err := g.do(ctx, http.MethodGet, g.repoPath(fmt.Sprintf("/pulls/%d/reviews?per_page=100", number)), nil, &reviews); err != nil {
		return ""
	}
	latest := map[string]string{}
	for _, r := range reviews {
		switch r.State {
		case "APPROVED", "CHANGES_REQUESTED":
			latest[r.User.Login] = r.State
		case "DISMISSED":
			delete(latest, r.User.Login)
		}
	}
	decision := ""
	for _, s := range latest {
		if s == "CHANGES_REQUESTED" {
			return "changes_requested"
		}
		decision = "approved"
	}
	return decision
}

// Checks reads the check runs and the commit statuses of sha.
func (g *GitHub) Checks(ctx context.Context, sha string) (Checks, error) {
	var runs []Check
	var checkRuns struct {
		CheckRuns []struct {
			Name       string `json:"name"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			HTMLURL    string `json:"html_url"`
		} `json:"check_runs"`
	}
	if _, err := g.do(ctx, http.MethodGet, g.repoPath("/commits/"+url.PathEscape(sha)+"/check-runs?per_page=100"), nil, &checkRuns); err != nil {
		return Checks{}, err
	}
	for _, r := range checkRuns.CheckRuns {
		state := Pending
		if r.Status == "completed" {
			switch r.Conclusion {
			case "success":
				state = Passing
			case "skipped", "neutral", "cancelled":
				state = Skipped
			default:
				state = Failing
			}
		}
		runs = append(runs, Check{Name: r.Name, State: state, URL: r.HTMLURL})
	}
	var status struct {
		Statuses []struct {
			Context   string `json:"context"`
			State     string `json:"state"`
			TargetURL string `json:"target_url"`
		} `json:"statuses"`
	}
	if _, err := g.do(ctx, http.MethodGet, g.repoPath("/commits/"+url.PathEscape(sha)+"/status"), nil, &status); err == nil {
		for _, s := range status.Statuses {
			state := Pending
			switch s.State {
			case "success":
				state = Passing
			case "failure", "error":
				state = Failing
			}
			runs = append(runs, Check{Name: s.Context, State: state, URL: s.TargetURL})
		}
	}
	return Summarize(runs), nil
}

// CreatePullRequest opens a pull request.
func (g *GitHub) CreatePullRequest(ctx context.Context, in NewPullRequest) (PullRequest, error) {
	var p ghPull
	if _, err := g.do(ctx, http.MethodPost, g.repoPath("/pulls"), in, &p); err != nil {
		return PullRequest{}, err
	}
	return p.pullRequest(), nil
}

// Merge merges a pull request with method: merge, squash, or rebase.
func (g *GitHub) Merge(ctx context.Context, number int, method string) error {
	if method == "" {
		method = MergeMerge
	}
	if method != MergeMerge && method != MergeSquash && method != MergeRebase {
		return fmt.Errorf("the merge method is merge, squash, or rebase, not %q", method)
	}
	var out struct {
		Merged  bool   `json:"merged"`
		Message string `json:"message"`
	}
	if _, err := g.do(ctx, http.MethodPut, g.repoPath(fmt.Sprintf("/pulls/%d/merge", number)), map[string]string{"merge_method": method}, &out); err != nil {
		return err
	}
	if !out.Merged {
		return errors.New("github: " + out.Message)
	}
	return nil
}

type ghIssue struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
	State   string `json:"state"`
	User    ghUser `json:"user"`
	Labels  []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Assignees   []ghUser  `json:"assignees"`
	Comments    int       `json:"comments"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ClosedAt    *string   `json:"closed_at"`
	PullRequest *struct{} `json:"pull_request"`
}

func (i ghIssue) issue() Issue {
	out := Issue{Number: i.Number, Title: i.Title, Body: i.Body, URL: i.HTMLURL, State: i.State, Author: i.User.Login,
		Comments: i.Comments, Created: i.CreatedAt, Updated: i.UpdatedAt, Labels: []string{}}
	for _, l := range i.Labels {
		out.Labels = append(out.Labels, l.Name)
	}
	for _, a := range i.Assignees {
		out.Assignees = append(out.Assignees, a.Login)
	}
	if i.ClosedAt != nil && *i.ClosedAt != "" {
		out.Closed, _ = time.Parse(time.RFC3339, *i.ClosedAt)
	}
	return out
}

// Issues lists the open issues and the ones closed within the recent
// window, without pull requests.
func (g *GitHub) Issues(ctx context.Context) ([]Issue, error) {
	since := g.Now().Add(-recentWindow)
	out := []Issue{}
	stop := func(raw json.RawMessage) bool {
		var i ghIssue
		if json.Unmarshal(raw, &i) != nil {
			return false
		}
		return i.State != Open && i.UpdatedAt.Before(since)
	}
	err := g.list(ctx, g.repoPath("/issues?state=all&sort=updated&direction=desc"), stop, func(raw json.RawMessage) {
		var i ghIssue
		if json.Unmarshal(raw, &i) == nil && i.PullRequest == nil {
			out = append(out, i.issue())
		}
	})
	return out, err
}

type ghComment struct {
	ID        int64     `json:"id"`
	User      ghUser    `json:"user"`
	Body      string    `json:"body"`
	HTMLURL   string    `json:"html_url"`
	CreatedAt time.Time `json:"created_at"`
}

func (c ghComment) comment() Comment {
	return Comment{ID: c.ID, Author: c.User.Login, Body: c.Body, URL: c.HTMLURL, Created: c.CreatedAt}
}

// Issue reads one issue with its comments.
func (g *GitHub) Issue(ctx context.Context, number int) (Issue, []Comment, error) {
	var i ghIssue
	if _, err := g.do(ctx, http.MethodGet, g.repoPath(fmt.Sprintf("/issues/%d", number)), nil, &i); err != nil {
		return Issue{}, nil, err
	}
	comments := []Comment{}
	err := g.list(ctx, g.repoPath(fmt.Sprintf("/issues/%d/comments", number)), nil, func(raw json.RawMessage) {
		var c ghComment
		if json.Unmarshal(raw, &c) == nil {
			comments = append(comments, c.comment())
		}
	})
	return i.issue(), comments, err
}

// CreateIssue opens an issue.
func (g *GitHub) CreateIssue(ctx context.Context, in NewIssue) (Issue, error) {
	var i ghIssue
	if _, err := g.do(ctx, http.MethodPost, g.repoPath("/issues"), in, &i); err != nil {
		return Issue{}, err
	}
	return i.issue(), nil
}

// UpdateIssue changes the fields of p that are set.
func (g *GitHub) UpdateIssue(ctx context.Context, number int, p IssuePatch) (Issue, error) {
	var i ghIssue
	if _, err := g.do(ctx, http.MethodPatch, g.repoPath(fmt.Sprintf("/issues/%d", number)), p, &i); err != nil {
		return Issue{}, err
	}
	return i.issue(), nil
}

// Comment adds a comment to an issue.
func (g *GitHub) Comment(ctx context.Context, number int, body string) (Comment, error) {
	var c ghComment
	if _, err := g.do(ctx, http.MethodPost, g.repoPath(fmt.Sprintf("/issues/%d/comments", number)), map[string]string{"body": body}, &c); err != nil {
		return Comment{}, err
	}
	return c.comment(), nil
}

// labelColors give hikari's labels a color, by prefix; others are grey.
var labelColors = map[string]string{"hikari:queued": "c5def5", "hikari:working": "1d76db", "hikari:blocked": "d93f0b", "hikari:done": "0e8a16"}

// AddLabels adds labels to an issue; a label that does not exist is made
// first (rule FG3).
func (g *GitHub) AddLabels(ctx context.Context, number int, labels ...string) error {
	if len(labels) == 0 {
		return nil
	}
	for _, l := range labels {
		color := labelColors[l]
		if color == "" {
			color = "ededed"
		}
		resp, err := g.do(ctx, http.MethodPost, g.repoPath("/labels"), map[string]string{"name": l, "color": color, "description": "set by hikari"}, nil)
		if err != nil && (resp == nil || resp.StatusCode != http.StatusUnprocessableEntity) {
			return err
		}
	}
	_, err := g.do(ctx, http.MethodPost, g.repoPath(fmt.Sprintf("/issues/%d/labels", number)), map[string][]string{"labels": labels}, nil)
	return err
}

// RemoveLabel takes a label off an issue; a label that it does not have is
// not an error.
func (g *GitHub) RemoveLabel(ctx context.Context, number int, label string) error {
	resp, err := g.do(ctx, http.MethodDelete, g.repoPath(fmt.Sprintf("/issues/%d/labels/%s", number, url.PathEscape(label))), nil, nil)
	if err != nil && resp != nil && resp.StatusCode == http.StatusNotFound {
		return nil
	}
	return err
}
