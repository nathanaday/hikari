package api

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/nathanaday/hikari/internal/featuremap"
	"github.com/nathanaday/hikari/internal/vault"
	"github.com/nathanaday/hikari/internal/vocab"
	"github.com/nathanaday/hikari/internal/wiki"
)

// WorkspaceRoot is where a partition agent's container sees its repo.
const WorkspaceRoot = "/workspace"

// Where every container sees hikari: its documents, and the hikari binary.
const (
	HikariRoot  = "/hikari"
	BinaryPath  = "/hikari/bin/hikari"
	PrivateDir  = "/hikari/private"
	ExportDir   = "/hikari/public/export"
	ReadonlyDir = "/hikari/public/readonly"
)

// SharedDir is the shared area of the vault in a container.
var SharedDir = path.Join(ReadonlyDir, "shared")

// PublicWikiPath is where every container sees the public wiki.
var PublicWikiPath = path.Join(SharedDir, "wiki")

// FeatureMapRef and PublicWikiRef name a wiki.
func FeatureMapRef(partition string) string { return "partition:" + partition }

const PublicWikiRef = "public"

// ReleasesPath returns where a container sees the releases of a partition that
// it can read.
func ReleasesPath(partition string) string {
	return path.Join(ReadonlyDir, "systems", partition, "releases")
}

// WorkDir returns the container path of a scope.
func WorkDir(scope string) string {
	return path.Join(WorkspaceRoot, scope)
}

// SelfPartition describes the partition that an agent owns.
type SelfPartition struct {
	Repo    string   `json:"repo"`
	Branch  string   `json:"branch"`
	Scope   string   `json:"scope"`
	Read    []string `json:"read,omitempty"`
	WorkDir string   `json:"work_dir"`
}

// TurnContext tells the driver what changed before an agent's next turn.
type TurnContext struct {
	MapState            string                    `json:"map_state,omitempty"`
	StaleFeatures       []featuremap.StaleFeature `json:"stale_features,omitempty"`
	UndescribedFeatures []string                  `json:"undescribed_features,omitempty"`
	UnmappedPaths       []string                  `json:"unmapped_paths,omitempty"`
	FailingFeatures     []FailingFeature          `json:"failing_features,omitempty"`
	PendingFeatures     []string                  `json:"pending_features,omitempty"`
	PrivateErrors       []string                  `json:"private_errors,omitempty"`
	VocabularyChanged   bool                      `json:"vocabulary_changed,omitempty"`
	// WikiChanged lists the documents of the agent's readable wikis that
	// someone else changed since its last turn (rule E1).
	WikiChanged []WikiChange `json:"wiki_changed,omitempty"`
	// NoLocalResults says that the map records test runs that this machine
	// has no results of (rule SY10 of the linked repositories spec).
	NoLocalResults bool `json:"no_local_results,omitempty"`
}

// WikiChange names the changed documents of one wiki. Wiki is its label:
// "public" for the public wiki, or the partition's name.
type WikiChange struct {
	Wiki string      `json:"wiki"`
	Docs []DocChange `json:"docs"`
}

type DocChange struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Removed bool   `json:"removed,omitempty"`
}

// FailingFeature is a failing feature with the ids of its failing tests.
type FailingFeature struct {
	ID    string   `json:"id"`
	Tests []string `json:"tests"`
}

// RunRequest asks the server to record the report of one run of a suite.
type RunRequest struct {
	Suite string `json:"suite"`
}

// RunResult summarizes a recorded run: the outcomes of its tests, the
// tests that no key of the test map covers, and each feature whose health
// changed.
type RunResult struct {
	Suite    string         `json:"suite"`
	Passed   int            `json:"passed"`
	Failed   int            `json:"failed"`
	Skipped  int            `json:"skipped"`
	Unmapped int            `json:"unmapped"`
	Changed  []HealthChange `json:"changed,omitempty"`
}

type HealthChange struct {
	Feature string `json:"feature"`
	From    string `json:"from"`
	To      string `json:"to"`
}

// ChangeRequest is one change to a wiki: a partition agent's, or with Public, the
// public wiki.
type ChangeRequest struct {
	Public bool         `json:"public,omitempty"`
	Writes []wiki.Write `json:"writes"`
}

// ChangeResult says what a change did, with notes on what the author must
// do next, such as private documents that still name a renamed id.
type ChangeResult struct {
	wiki.Result
	// Rewritten names the documents of other wikis whose links the change
	// rewrote.
	Rewritten []string `json:"rewritten,omitempty"`
	Notes     []string `json:"notes,omitempty"`
}

// WikiDoc is one document of a wiki, without its body.
type WikiDoc struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	// Links are the documents it links, in order of first mention.
	Links  []string `json:"links"`
	Status string   `json:"status,omitempty"`
	// Health is a feature's health; Support is the health of the features
	// that a story, rule, or principle of the public wiki links (rule H2).
	Health  string    `json:"health,omitempty"`
	Support string    `json:"support,omitempty"`
	Created time.Time `json:"created,omitzero"`
	// File is the document's path in the vault, and Hash a digest of its
	// text, so that the UI reads it again when it changes.
	File string `json:"file"`
	Hash string `json:"hash"`
}

// WikiView is one wiki for the UI.
type WikiView struct {
	Ref       string    `json:"ref"`
	Scope     string    `json:"scope"`
	Name      string    `json:"name"`
	Documents []WikiDoc `json:"documents"`
	// Problems are the files that a hand edit broke (rule D4); Warnings
	// the rest.
	Problems []string `json:"problems,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// DocRef names a document in a wiki.
type DocRef struct {
	Wiki string `yaml:"wiki" json:"wiki"`
	ID   string `yaml:"id" json:"id"`
	Kind string `yaml:"kind" json:"kind"`
	Name string `yaml:"name,omitempty" json:"name,omitempty"`
}

// DocumentHit is a document for the read tools (rules T4 and T5).
type DocumentHit struct {
	Wiki        string `yaml:"wiki" json:"wiki"`
	ID          string `yaml:"id" json:"id"`
	Kind        string `yaml:"kind" json:"kind"`
	Name        string `yaml:"name,omitempty" json:"name,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Health      string `yaml:"health,omitempty" json:"health,omitempty"`
	State       string `yaml:"state,omitempty" json:"state,omitempty"`
	Support     string `yaml:"support,omitempty" json:"support,omitempty"`
	// Document is the text as it is on disk; a search leaves it out.
	Document  string   `yaml:"document,omitempty" json:"document,omitempty"`
	Backlinks []DocRef `yaml:"backlinks,omitempty" json:"backlinks,omitempty"`
}

type PartitionSummary struct {
	Name    string `yaml:"name" json:"name"`
	Summary string `yaml:"summary,omitempty" json:"summary,omitempty"`
	State   string `yaml:"state" json:"state"`
	// Features counts the features by health.
	Features map[string]int `yaml:"features" json:"features"`
}

// OrchestrationPartition is a partition as an orchestration agent sees it: the public map
// with interface flags and providers.
type OrchestrationPartition struct {
	featuremap.View `yaml:",inline"`
	Flags           []vocab.Flag `yaml:"flags,omitempty" json:"flags"`
	// Releases lists the partition's releases, newest first.
	Releases []vault.Manifest `yaml:"releases,omitempty" json:"releases"`
	// Providers maps each interface the partition consumes to the visible
	// partitions that provide it.
	Providers map[string][]string `yaml:"providers,omitempty" json:"providers"`
}

// PartitionView is everything about a partition, for the UI: the partition agent's view and
// the private documents.
type PartitionView struct {
	featuremap.PartitionAgentView
	// Dirty says whether the scope has uncommitted changes (rule N1).
	Dirty          bool                `json:"dirty"`
	Grants         Grants              `json:"grants"`
	Private        featuremap.Private  `json:"private"`
	Releases       []vault.Manifest    `json:"releases"`
	PartitionAgent string              `json:"partition_agent"`
	Repo           string              `json:"repo"`
	Scope          string              `json:"scope"`
	Branch         string              `json:"branch"`
	Flags          []vocab.Flag        `json:"flags"`
	Providers      map[string][]string `json:"providers"`
}

// Grants are what the topology grants a partition agent: the partitions whose releases
// it reads, and the deliverables it holds.
type Grants struct {
	Releases     []string `json:"releases"`
	Deliverables []string `json:"deliverables"`
}

// RepoInfo is one repository of the topology, for the UI: its hikari
// branch and the partitions whose scope is in it. It holds no host path.
type RepoInfo struct {
	Name       string   `json:"name"`
	Branch     string   `json:"branch"`
	Partitions []string `json:"partitions"`
}

type PublishRequest struct {
	Deliverable  string              `json:"deliverable"`
	Version      string              `json:"version"`
	Notes        string              `json:"notes"`
	Certificates []vault.Certificate `json:"certificates,omitempty"`
}

type CheckRequest struct {
	Deliverable string `json:"deliverable"`
}

type CommitRequest struct {
	Message string `json:"message"`
}

type CommitResponse struct {
	Commit string `json:"commit"`
}

// VocabularyChange is one change to the vocabulary. Op is "define_term",
// "define_interface", or "remove".
type VocabularyChange struct {
	Op        string           `json:"op"`
	Term      *vocab.Term      `json:"term,omitempty"`
	Interface *vocab.Interface `json:"interface,omitempty"`
	Name      string           `json:"name,omitempty"`
}

func (c *Client) MyMap(ctx context.Context) (featuremap.PartitionAgentView, error) {
	var v featuremap.PartitionAgentView
	err := c.do(ctx, http.MethodGet, "/api/agent/map", nil, &v)
	return v, err
}

func (c *Client) SetPartitionInfo(ctx context.Context, p featuremap.PartitionPatch) error {
	return c.do(ctx, http.MethodPatch, "/api/agent/map/partition", p, nil)
}

// ChangeWiki applies one change to the partition agent's wiki, or with public set,
// to the public wiki, which only the orchestration agent writes.
func (c *Client) ChangeWiki(ctx context.Context, public bool, writes []wiki.Write) (ChangeResult, error) {
	var out ChangeResult
	err := c.do(ctx, http.MethodPost, "/api/agent/wiki", ChangeRequest{Public: public, Writes: writes}, &out)
	return out, err
}

// RecordRun records the report of one run of a suite.
func (c *Client) RecordRun(ctx context.Context, suite string) (RunResult, error) {
	var out RunResult
	err := c.do(ctx, http.MethodPost, "/api/agent/map/tests", RunRequest{Suite: suite}, &out)
	return out, err
}

func (c *Client) MarkMapCurrent(ctx context.Context) (string, error) {
	var r CommitResponse
	err := c.do(ctx, http.MethodPost, "/api/agent/map/current", nil, &r)
	return r.Commit, err
}

// Commit commits the changes in the partition agent's scope. An empty message makes
// the automatic end-of-turn commit.
func (c *Client) Commit(ctx context.Context, message string) (string, error) {
	var r CommitResponse
	err := c.do(ctx, http.MethodPost, "/api/agent/commit", CommitRequest{Message: message}, &r)
	return r.Commit, err
}

func (c *Client) PublishRelease(ctx context.Context, req PublishRequest) (vault.Manifest, error) {
	var m vault.Manifest
	err := c.do(ctx, http.MethodPost, "/api/agent/releases", req, &m)
	return m, err
}

// CheckRelease checks the staged product of a deliverable without
// publishing it.
func (c *Client) CheckRelease(ctx context.Context, deliverable string) (vault.ProductCheck, error) {
	var out vault.ProductCheck
	err := c.do(ctx, http.MethodPost, "/api/agent/releases/check", CheckRequest{Deliverable: deliverable}, &out)
	return out, err
}

func (c *Client) TurnContext(ctx context.Context) (TurnContext, error) {
	var tc TurnContext
	err := c.do(ctx, http.MethodGet, "/api/agent/turn-context", nil, &tc)
	return tc, err
}

func (c *Client) ListPartitions(ctx context.Context) ([]PartitionSummary, error) {
	var out []PartitionSummary
	err := c.do(ctx, http.MethodGet, "/api/agent/partitions", nil, &out)
	return out, err
}

func (c *Client) GetPartition(ctx context.Context, name string) (OrchestrationPartition, error) {
	var out OrchestrationPartition
	err := c.do(ctx, http.MethodGet, "/api/agent/partitions/"+url.PathEscape(name), nil, &out)
	return out, err
}

// SearchWiki searches every wiki the agent can read.
func (c *Client) SearchWiki(ctx context.Context, query string) ([]DocumentHit, error) {
	var out []DocumentHit
	err := c.do(ctx, http.MethodGet, "/api/agent/wiki/search?q="+url.QueryEscape(query), nil, &out)
	return out, err
}

// GetDocument returns one document of the wikis the agent can read.
func (c *Client) GetDocument(ctx context.Context, id string) (DocumentHit, error) {
	var out DocumentHit
	err := c.do(ctx, http.MethodGet, "/api/agent/wiki/docs/"+url.PathEscape(id), nil, &out)
	return out, err
}

func (c *Client) GetVocabulary(ctx context.Context) (vocab.Vocabulary, error) {
	var out vocab.Vocabulary
	err := c.do(ctx, http.MethodGet, "/api/agent/vocabulary", nil, &out)
	return out, err
}

func (c *Client) ChangeVocabulary(ctx context.Context, ch VocabularyChange) error {
	return c.do(ctx, http.MethodPost, "/api/agent/vocabulary", ch, nil)
}
