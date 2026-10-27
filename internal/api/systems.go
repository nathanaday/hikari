package api

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/nathanaday/hikari/internal/store"
	"github.com/nathanaday/hikari/internal/sysmap"
	"github.com/nathanaday/hikari/internal/vocab"
	"github.com/nathanaday/hikari/internal/wiki"
)

// WorkspaceRoot is where an owner's container sees its repo.
const WorkspaceRoot = "/workspace"

// Where every container sees hikari: its documents, and the hikari binary.
const (
	HikariRoot  = "/hikari"
	BinaryPath  = "/hikari/bin/hikari"
	PrivateDir  = "/hikari/private"
	ExportDir   = "/hikari/public/export"
	ReadonlyDir = "/hikari/public/readonly"
)

// SharedDir is the shared area of the store in a container.
var SharedDir = path.Join(ReadonlyDir, "shared")

// HubWikiPath returns where a container sees the wiki of a hub that it can
// read (rule A1 of the wikis spec).
func HubWikiPath(hub string) string {
	return path.Join(ReadonlyDir, "hubs", hub, "wiki")
}

// SystemWikiRef and HubWikiRef name a wiki.
func SystemWikiRef(system string) string { return "system:" + system }
func HubWikiRef(hub string) string       { return "hub:" + hub }

// ReleasesPath returns where a container sees the releases of a system that
// it can read.
func ReleasesPath(system string) string {
	return path.Join(ReadonlyDir, "systems", system, "releases")
}

// WorkDir returns the container path of a scope.
func WorkDir(scope string) string {
	return path.Join(WorkspaceRoot, scope)
}

// SelfSystem describes the system that an agent owns.
type SelfSystem struct {
	Repo    string   `json:"repo"`
	Branch  string   `json:"branch"`
	Scope   string   `json:"scope"`
	Read    []string `json:"read,omitempty"`
	WorkDir string   `json:"work_dir"`
}

// TurnContext tells the driver what changed before an agent's next turn.
type TurnContext struct {
	MapState            string                `json:"map_state,omitempty"`
	StaleFeatures       []sysmap.StaleFeature `json:"stale_features,omitempty"`
	UndescribedFeatures []string              `json:"undescribed_features,omitempty"`
	UnmappedPaths       []string              `json:"unmapped_paths,omitempty"`
	FailingFeatures     []FailingFeature      `json:"failing_features,omitempty"`
	PendingFeatures     []string              `json:"pending_features,omitempty"`
	PrivateErrors       []string              `json:"private_errors,omitempty"`
	VocabularyChanged   []string              `json:"vocabulary_changed,omitempty"`
	// WikiChanged lists the documents of the agent's readable wikis that
	// someone else changed since its last turn (rule E1).
	WikiChanged []WikiChange `json:"wiki_changed,omitempty"`
	// NoLocalResults says that the map records test runs that this machine
	// has no results of (rule SY10 of the linked repositories spec).
	NoLocalResults bool `json:"no_local_results,omitempty"`
}

// WikiChange names the changed documents of one wiki. Wiki is its label:
// "#shop" for a hub, or the system's name.
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

// ChangeRequest is one change to a wiki: an owner's, or with Hub, a hub's.
type ChangeRequest struct {
	Hub    string       `json:"hub,omitempty"`
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
	Key    bool     `json:"key,omitempty"`
	Status string   `json:"status,omitempty"`
	// Health is a feature's health; Support is the health of the features
	// that a hub's story or rule links (rule H2).
	Health  string    `json:"health,omitempty"`
	Support string    `json:"support,omitempty"`
	Created time.Time `json:"created,omitzero"`
	// File is the document's path in the store, and Hash a digest of its
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

type SystemSummary struct {
	Name    string `yaml:"name" json:"name"`
	Summary string `yaml:"summary,omitempty" json:"summary,omitempty"`
	State   string `yaml:"state" json:"state"`
	// Features counts the features by health.
	Features map[string]int `yaml:"features" json:"features"`
}

// OrchestratorSystem is a system as an orchestrator sees it: the public map
// with interface flags and providers.
type OrchestratorSystem struct {
	sysmap.View `yaml:",inline"`
	Flags       []vocab.Flag `yaml:"flags,omitempty" json:"flags"`
	// Releases lists the system's releases, newest first.
	Releases []store.Manifest `yaml:"releases,omitempty" json:"releases"`
	// Providers maps each interface the system consumes to the visible
	// systems that provide it.
	Providers map[string][]string `yaml:"providers,omitempty" json:"providers"`
}

// SystemView is everything about a system, for the UI: the owner's view and
// the private documents.
type SystemView struct {
	sysmap.OwnerView
	// Dirty says whether the scope has uncommitted changes (rule N1).
	Dirty     bool                `json:"dirty"`
	Grants    Grants              `json:"grants"`
	Private   sysmap.Private      `json:"private"`
	Releases  []store.Manifest    `json:"releases"`
	Owner     string              `json:"owner"`
	Repo      string              `json:"repo"`
	Scope     string              `json:"scope"`
	Branch    string              `json:"branch"`
	Flags     []vocab.Flag        `json:"flags"`
	Providers map[string][]string `json:"providers"`
}

// Grants are what the topology grants an owner: the systems whose releases
// it reads, and the deliverables it holds.
type Grants struct {
	Releases     []string `json:"releases"`
	Deliverables []string `json:"deliverables"`
}

// RepoInfo is one repository of the topology, for the UI: its hikari
// branch and the systems whose scope is in it. It holds no host path.
type RepoInfo struct {
	Name    string   `json:"name"`
	Branch  string   `json:"branch"`
	Systems []string `json:"systems"`
}

type PublishRequest struct {
	Deliverable  string              `json:"deliverable"`
	Version      string              `json:"version"`
	Notes        string              `json:"notes"`
	Certificates []store.Certificate `json:"certificates,omitempty"`
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

// VocabularyChange is one change to a hub vocabulary. Op is "define_term",
// "define_interface", or "remove".
type VocabularyChange struct {
	Op        string           `json:"op"`
	Term      *vocab.Term      `json:"term,omitempty"`
	Interface *vocab.Interface `json:"interface,omitempty"`
	Name      string           `json:"name,omitempty"`
}

func (c *Client) MyMap(ctx context.Context) (sysmap.OwnerView, error) {
	var v sysmap.OwnerView
	err := c.do(ctx, http.MethodGet, "/api/node/map", nil, &v)
	return v, err
}

func (c *Client) SetSystemInfo(ctx context.Context, p sysmap.SystemPatch) error {
	return c.do(ctx, http.MethodPatch, "/api/node/map/system", p, nil)
}

// ChangeFeatures applies one change to the owner's feature wiki.
// ChangeWiki applies one change to the owner's wiki, or with hub set, to
// the wiki of a hub the agent orchestrates.
func (c *Client) ChangeWiki(ctx context.Context, hub string, writes []wiki.Write) (ChangeResult, error) {
	var out ChangeResult
	err := c.do(ctx, http.MethodPost, "/api/node/wiki", ChangeRequest{Hub: hub, Writes: writes}, &out)
	return out, err
}

// RecordRun records the report of one run of a suite.
func (c *Client) RecordRun(ctx context.Context, suite string) (RunResult, error) {
	var out RunResult
	err := c.do(ctx, http.MethodPost, "/api/node/map/tests", RunRequest{Suite: suite}, &out)
	return out, err
}

func (c *Client) MarkMapCurrent(ctx context.Context) (string, error) {
	var r CommitResponse
	err := c.do(ctx, http.MethodPost, "/api/node/map/current", nil, &r)
	return r.Commit, err
}

// Commit commits the changes in the owner's scope. An empty message makes
// the automatic end-of-turn commit.
func (c *Client) Commit(ctx context.Context, message string) (string, error) {
	var r CommitResponse
	err := c.do(ctx, http.MethodPost, "/api/node/commit", CommitRequest{Message: message}, &r)
	return r.Commit, err
}

func (c *Client) PublishRelease(ctx context.Context, req PublishRequest) (store.Manifest, error) {
	var m store.Manifest
	err := c.do(ctx, http.MethodPost, "/api/node/releases", req, &m)
	return m, err
}

// CheckRelease checks the staged product of a deliverable without
// publishing it.
func (c *Client) CheckRelease(ctx context.Context, deliverable string) (store.ProductCheck, error) {
	var out store.ProductCheck
	err := c.do(ctx, http.MethodPost, "/api/node/releases/check", CheckRequest{Deliverable: deliverable}, &out)
	return out, err
}

func (c *Client) TurnContext(ctx context.Context) (TurnContext, error) {
	var tc TurnContext
	err := c.do(ctx, http.MethodGet, "/api/node/turn-context", nil, &tc)
	return tc, err
}

func (c *Client) ListSystems(ctx context.Context) ([]SystemSummary, error) {
	var out []SystemSummary
	err := c.do(ctx, http.MethodGet, "/api/node/systems", nil, &out)
	return out, err
}

func (c *Client) GetSystem(ctx context.Context, name string) (OrchestratorSystem, error) {
	var out OrchestratorSystem
	err := c.do(ctx, http.MethodGet, "/api/node/systems/"+url.PathEscape(name), nil, &out)
	return out, err
}

// SearchWiki searches every wiki the agent can read.
func (c *Client) SearchWiki(ctx context.Context, query string) ([]DocumentHit, error) {
	var out []DocumentHit
	err := c.do(ctx, http.MethodGet, "/api/node/wiki/search?q="+url.QueryEscape(query), nil, &out)
	return out, err
}

// GetDocument returns one document of the wikis the agent can read.
func (c *Client) GetDocument(ctx context.Context, id string) (DocumentHit, error) {
	var out DocumentHit
	err := c.do(ctx, http.MethodGet, "/api/node/wiki/docs/"+url.PathEscape(id), nil, &out)
	return out, err
}

func (c *Client) GetVocabulary(ctx context.Context, hub string) (vocab.Vocabulary, error) {
	var out vocab.Vocabulary
	err := c.do(ctx, http.MethodGet, "/api/node/vocabulary/"+url.PathEscape(hub), nil, &out)
	return out, err
}

func (c *Client) ChangeVocabulary(ctx context.Context, hub string, ch VocabularyChange) error {
	return c.do(ctx, http.MethodPost, "/api/node/vocabulary/"+url.PathEscape(hub), ch, nil)
}
