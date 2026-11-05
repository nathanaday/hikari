// Package forge reads and changes the service that hosts a repository's
// remote: its pull requests, its checks, and its issues (the repositories,
// forges, and issues spec). GitHub is the first forge. A forge is detected
// from the repository's origin URL (decision 1), and the credential is the
// machine's (decision 3).
package forge

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Kind names a forge.
type Kind string

// KindGitHub is GitHub, github.com or a GitHub Enterprise host.
const KindGitHub Kind = "github"

// Remote is what the origin URL says about a repository's forge.
type Remote struct {
	Kind  Kind   `json:"kind"`
	Host  string `json:"host"`
	Owner string `json:"owner"`
	Name  string `json:"name"`
	// Web is the repository's page, and API the base of its API.
	Web string `json:"web"`
	API string `json:"api"`
}

// Slug is the repository as owner/name.
func (r Remote) Slug() string { return r.Owner + "/" + r.Name }

var scpLike = regexp.MustCompile(`^(?:[A-Za-z0-9._-]+@)?([A-Za-z0-9.-]+):([^/].*)$`)

// Parse reads an https://, ssh://, or git@host: URL into a remote (rule
// FG1). A URL whose host hikari does not know, or that names no owner and
// name, has no forge.
func Parse(raw string) (Remote, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Remote{}, false
	}
	var host, p string
	if m := scpLike.FindStringSubmatch(raw); m != nil && !strings.Contains(raw, "://") {
		host, p = m[1], m[2]
	} else {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return Remote{}, false
		}
		switch u.Scheme {
		case "https", "http", "ssh", "git", "git+ssh", "ssh+git":
		default:
			return Remote{}, false
		}
		host, p = u.Hostname(), u.Path
	}
	host = strings.ToLower(host)
	p = strings.Trim(p, "/")
	p = strings.TrimSuffix(p, ".git")
	parts := strings.Split(p, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Remote{}, false
	}
	r := Remote{Host: host, Owner: parts[0], Name: parts[1], Web: "https://" + host + "/" + parts[0] + "/" + parts[1]}
	switch {
	case host == "github.com" || host == "www.github.com":
		r.Kind, r.Host, r.API = KindGitHub, "github.com", "https://api.github.com"
		r.Web = "https://github.com/" + parts[0] + "/" + parts[1]
	case strings.HasPrefix(host, "github.") || strings.Contains(host, ".github.") || strings.HasPrefix(host, "ghe."):
		// A GitHub Enterprise host, by convention: github.<company>.com.
		r.Kind, r.API = KindGitHub, "https://"+host+"/api/v3"
	default:
		return Remote{}, false
	}
	return r, true
}

// The states of a pull request.
const (
	Open   = "open"
	Closed = "closed"
	Merged = "merged"
)

// PullRequest is one pull request of a forge (rule FG2).
type PullRequest struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body,omitempty"`
	URL    string `json:"url"`
	// State is open, closed, or merged.
	State  string    `json:"state"`
	Draft  bool      `json:"draft,omitempty"`
	Author string    `json:"author,omitempty"`
	Head   string    `json:"head"`
	Base   string    `json:"base"`
	SHA    string    `json:"sha"`
	Merged time.Time `json:"merged,omitzero"`
	// Mergeable is mergeable, conflicting, or unknown.
	Mergeable string `json:"mergeable,omitempty"`
	// Review is approved, changes_requested, or "" when no review decides.
	Review    string    `json:"review,omitempty"`
	Additions int       `json:"additions,omitempty"`
	Deletions int       `json:"deletions,omitempty"`
	Files     int       `json:"files,omitempty"`
	Created   time.Time `json:"created,omitzero"`
	Updated   time.Time `json:"updated,omitzero"`
	// Closes lists the issues that the body closes, by number.
	Closes []int `json:"closes,omitempty"`
}

// The states of a check, and of the summary of a commit's checks.
const (
	Passing = "passing"
	Failing = "failing"
	Pending = "pending"
	Skipped = "skipped"
	None    = "none"
)

// Check is one check run, or one commit status, of a commit.
type Check struct {
	Name  string `json:"name"`
	State string `json:"state"`
	URL   string `json:"url,omitempty"`
}

// Checks is every check of a commit, with a summary (rule RP3).
type Checks struct {
	State   string  `json:"state"`
	Passed  int     `json:"passed"`
	Failed  int     `json:"failed"`
	Pending int     `json:"pending"`
	Runs    []Check `json:"runs"`
}

// Summarize fills the counts and the state from the runs.
func Summarize(runs []Check) Checks {
	c := Checks{Runs: runs, State: None}
	if c.Runs == nil {
		c.Runs = []Check{}
	}
	for _, r := range runs {
		switch r.State {
		case Passing:
			c.Passed++
		case Failing:
			c.Failed++
		case Pending:
			c.Pending++
		}
	}
	switch {
	case c.Failed > 0:
		c.State = Failing
	case c.Pending > 0:
		c.State = Pending
	case c.Passed > 0:
		c.State = Passing
	}
	return c
}

// NewPullRequest is what CreatePullRequest takes.
type NewPullRequest struct {
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
	Head  string `json:"head"`
	Base  string `json:"base"`
	Draft bool   `json:"draft,omitempty"`
}

// The merge methods.
const (
	MergeMerge  = "merge"
	MergeSquash = "squash"
	MergeRebase = "rebase"
)

// Forge reads and changes the code side of a repository's forge (rule FG2).
type Forge interface {
	Remote() Remote
	DefaultBranch(ctx context.Context) (string, error)
	// PullRequests lists the open pull requests, and the closed ones by
	// recent update.
	PullRequests(ctx context.Context) ([]PullRequest, error)
	Checks(ctx context.Context, sha string) (Checks, error)
	CreatePullRequest(ctx context.Context, in NewPullRequest) (PullRequest, error)
	Merge(ctx context.Context, number int, method string) error
}

// Issue is one issue of a tracker (rule FG3).
type Issue struct {
	Number int      `json:"number"`
	Title  string   `json:"title"`
	Body   string   `json:"body,omitempty"`
	URL    string   `json:"url,omitempty"`
	State  string   `json:"state"`
	Labels []string `json:"labels"`
	Author string   `json:"author,omitempty"`
	// Assignees are the forge's assignees, by login.
	Assignees []string  `json:"assignees,omitempty"`
	Comments  int       `json:"comments"`
	Created   time.Time `json:"created,omitzero"`
	Updated   time.Time `json:"updated,omitzero"`
	Closed    time.Time `json:"closed,omitzero"`
}

// Comment is one comment of an issue.
type Comment struct {
	ID      int64     `json:"id"`
	Author  string    `json:"author,omitempty"`
	Body    string    `json:"body"`
	Created time.Time `json:"created,omitzero"`
	URL     string    `json:"url,omitempty"`
}

// NewIssue is what CreateIssue takes.
type NewIssue struct {
	Title  string   `json:"title"`
	Body   string   `json:"body,omitempty"`
	Labels []string `json:"labels,omitempty"`
}

// IssuePatch is what UpdateIssue takes. A nil field is left as it is.
type IssuePatch struct {
	Title  *string   `json:"title,omitempty"`
	Body   *string   `json:"body,omitempty"`
	State  *string   `json:"state,omitempty"`
	Labels *[]string `json:"labels,omitempty"`
}

// Tracker reads and changes the issues of a repository (rule FG3).
type Tracker interface {
	Remote() Remote
	// Issues lists the open issues, and the closed ones by recent update,
	// without pull requests.
	Issues(ctx context.Context) ([]Issue, error)
	Issue(ctx context.Context, number int) (Issue, []Comment, error)
	CreateIssue(ctx context.Context, in NewIssue) (Issue, error)
	UpdateIssue(ctx context.Context, number int, p IssuePatch) (Issue, error)
	Comment(ctx context.Context, number int, body string) (Comment, error)
	// AddLabels adds labels to an issue, making each that does not exist.
	AddLabels(ctx context.Context, number int, labels ...string) error
	RemoveLabel(ctx context.Context, number int, label string) error
}

// ErrNoCredential says that the forge is known but hikari has no token for
// it (rule FG4).
var ErrNoCredential = errors.New("no credential")

// ErrUnknownForge says that the URL names no forge that hikari knows.
var ErrUnknownForge = errors.New("the repository's origin is not on a forge that hikari knows")

// New returns the forge of a remote, or an error that says why there is
// none. It is the constructor of every forge.
func New(r Remote) (Forge, error) {
	switch r.Kind {
	case KindGitHub:
		return NewGitHub(r, TokenFor(r.Host))
	}
	return nil, fmt.Errorf("%w: %s", ErrUnknownForge, r.Host)
}

// TrackerOf returns the tracker of a forge, when it has one.
func TrackerOf(f Forge) (Tracker, bool) {
	t, ok := f.(Tracker)
	return t, ok
}

var closes = regexp.MustCompile(`(?i)\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s*:?\s+#(\d+)`)

// ClosedIssues lists the issue numbers that a pull request's body closes,
// in GitHub's syntax: "closes #12", "fixes #3".
func ClosedIssues(body string) []int {
	var out []int
	for _, m := range closes.FindAllStringSubmatch(body, -1) {
		var n int
		fmt.Sscanf(m[1], "%d", &n)
		if n > 0 {
			out = append(out, n)
		}
	}
	return out
}
