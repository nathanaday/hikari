package api

import (
	"time"

	"github.com/nathanaday/hikari/internal/forge"
)

// RepoView is one repository of the topology, for the UI and the
// orchestration agent (rule RP1 of the repositories, forges, and issues
// spec): your branch, the partitions with a writable folder in it, its
// forge, its branches, and its pull requests. It holds no host path.
type RepoView struct {
	Name string `json:"name"`
	// Branch is your branch: the one the agents commit to.
	Branch     string   `json:"branch"`
	Partitions []string `json:"partitions"`
	// Project marks the project repository.
	Project bool `json:"project,omitempty"`
	// URL is the origin's URL, and Forge what it says, when hikari knows
	// the host.
	URL   string        `json:"url,omitempty"`
	Forge *forge.Remote `json:"forge,omitempty"`
	// Default is the default branch: the forge's, or what origin's HEAD
	// says, or main or master.
	Default string `json:"default_branch,omitempty"`
	// Head is the commit of your branch, and Dirty says that the working
	// tree has uncommitted changes.
	Head         string            `json:"head,omitempty"`
	Dirty        bool              `json:"dirty,omitempty"`
	Branches     []BranchView      `json:"branches"`
	PullRequests []PullRequestView `json:"pull_requests"`
	// Synced is when the forge was last read, and Fetched when origin was
	// last fetched.
	Synced  time.Time `json:"synced,omitzero"`
	Fetched time.Time `json:"fetched,omitzero"`
	// Error is the forge's last error, and GitError the last fetch's.
	Error    string `json:"error,omitempty"`
	GitError string `json:"git_error,omitempty"`
}

// BranchView is one branch of a repository (rule RP2): a local branch, a
// branch of origin, or both.
type BranchView struct {
	Name    string    `json:"name"`
	SHA     string    `json:"sha"`
	Subject string    `json:"subject,omitempty"`
	Author  string    `json:"author,omitempty"`
	Time    time.Time `json:"time,omitzero"`
	Local   bool      `json:"local"`
	Remote  bool      `json:"remote"`
	// Ahead and Behind compare the branch with the default branch.
	Ahead  int `json:"ahead"`
	Behind int `json:"behind"`
	// Unpushed counts the local commits that origin's branch lacks, or
	// every commit that no branch of origin has when origin lacks it.
	Unpushed int `json:"unpushed"`
	// Yours marks your branch, Default the default branch, and Merged a
	// branch whose head is in the default branch.
	Yours   bool `json:"yours,omitempty"`
	Default bool `json:"default,omitempty"`
	Merged  bool `json:"merged,omitempty"`
	// PullRequest is the number of the open pull request whose head the
	// branch is, or 0.
	PullRequest int `json:"pull_request,omitempty"`
}

// PullRequestView is a pull request with the checks of its head (rule RP3).
type PullRequestView struct {
	forge.PullRequest
	Checks forge.Checks `json:"checks"`
}

// NewPullRequestRequest opens a pull request from your branch (rule RP5).
// Head and Base default to your branch and the default branch.
type NewPullRequestRequest struct {
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
	Head  string `json:"head,omitempty"`
	Base  string `json:"base,omitempty"`
	Draft bool   `json:"draft,omitempty"`
}

// MergeRequest merges a pull request with a method: merge, squash, or
// rebase.
type MergeRequest struct {
	Method string `json:"method,omitempty"`
}
