package api

import (
	"context"
	"net/http"
	"net/url"
	"path"

	"github.com/nathanaday/hikari/internal/store"
	"github.com/nathanaday/hikari/internal/sysmap"
	"github.com/nathanaday/hikari/internal/vocab"
)

// WorkspaceRoot is where an owner's container sees its repo.
const WorkspaceRoot = "/workspace"

// Where every container sees its hikari documents.
const (
	PrivateDir  = "/hikari/private"
	ExportDir   = "/hikari/public/export"
	ReadonlyDir = "/hikari/public/readonly"
)

// SharedDir is the shared area of the store in a container.
var SharedDir = path.Join(ReadonlyDir, "shared")

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
	MapState          string   `json:"map_state,omitempty"`
	Changed           []string `json:"changed,omitempty"`
	PrivateErrors     []string `json:"private_errors,omitempty"`
	VocabularyChanged []string `json:"vocabulary_changed,omitempty"`
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

type FeatureHit struct {
	System  string         `yaml:"system" json:"system"`
	Feature sysmap.Feature `yaml:"feature" json:"feature"`
}

// SystemView is everything about a system, for the UI: the owner's view and
// the private documents.
type SystemView struct {
	sysmap.OwnerView
	Private   sysmap.Private      `json:"private"`
	Releases  []store.Manifest    `json:"releases"`
	Owner     string              `json:"owner"`
	Repo      string              `json:"repo"`
	Scope     string              `json:"scope"`
	Branch    string              `json:"branch"`
	Flags     []vocab.Flag        `json:"flags"`
	Providers map[string][]string `json:"providers"`
}

type PublishRequest struct {
	Version string `json:"version"`
	Notes   string `json:"notes"`
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

func (c *Client) UpsertFeatures(ctx context.Context, fs []sysmap.FeatureInput) error {
	return c.do(ctx, http.MethodPut, "/api/node/map/features", fs, nil)
}

func (c *Client) RemoveFeature(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/node/map/features/"+url.PathEscape(id), nil, nil)
}

func (c *Client) RecordTests(ctx context.Context, results []sysmap.TestResult) error {
	return c.do(ctx, http.MethodPost, "/api/node/map/tests", results, nil)
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

func (c *Client) PublishRelease(ctx context.Context, version, notes string) (store.Manifest, error) {
	var m store.Manifest
	err := c.do(ctx, http.MethodPost, "/api/node/releases", PublishRequest{Version: version, Notes: notes}, &m)
	return m, err
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

func (c *Client) SearchFeatures(ctx context.Context, query string) ([]FeatureHit, error) {
	var out []FeatureHit
	err := c.do(ctx, http.MethodGet, "/api/node/features?q="+url.QueryEscape(query), nil, &out)
	return out, err
}

func (c *Client) GetFeature(ctx context.Context, system, id string) (FeatureHit, error) {
	var out FeatureHit
	err := c.do(ctx, http.MethodGet, "/api/node/systems/"+url.PathEscape(system)+"/features/"+url.PathEscape(id), nil, &out)
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
