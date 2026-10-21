package api

import (
	"context"
	"net/http"
	"net/url"
	"path"

	"github.com/nathanaday/hikari/internal/sysmap"
	"github.com/nathanaday/hikari/internal/vocab"
)

// WorkspaceRoot is where an owner's container sees its repo.
const WorkspaceRoot = "/workspace"

// WorkDir returns the container path of a scope.
func WorkDir(scope string) string {
	return path.Join(WorkspaceRoot, scope)
}

// SelfSystem describes the system that an agent owns.
type SelfSystem struct {
	Repo        string   `json:"repo"`
	Branch      string   `json:"branch"`
	Scope       string   `json:"scope"`
	Read        []string `json:"read,omitempty"`
	WorkDir     string   `json:"work_dir"`
	TestCommand string   `json:"test_command,omitempty"`
}

// TurnContext tells the driver what changed before an agent's next turn.
type TurnContext struct {
	MapState          string   `json:"map_state,omitempty"`
	Changed           []string `json:"changed,omitempty"`
	VocabularyChanged []string `json:"vocabulary_changed,omitempty"`
}

type SystemSummary struct {
	Name    string `yaml:"name" json:"name"`
	Summary string `yaml:"summary,omitempty" json:"summary,omitempty"`
	State   string `yaml:"state" json:"state"`
	// Features counts the features by health.
	Features map[string]int `yaml:"features" json:"features"`
}

// OrchestratorSystem is a system as an orchestrator sees it.
type OrchestratorSystem struct {
	sysmap.Redacted `yaml:",inline"`
	Flags           []vocab.Flag `yaml:"flags,omitempty" json:"flags"`
	// Providers maps each interface the system consumes to the visible
	// systems that provide it.
	Providers map[string][]string `yaml:"providers,omitempty" json:"providers"`
}

type FeatureHit struct {
	System  string                 `yaml:"system" json:"system"`
	Feature sysmap.RedactedFeature `yaml:"feature" json:"feature"`
}

// SystemView is the full view of a system for the UI.
type SystemView struct {
	sysmap.View
	Owner     string              `json:"owner"`
	Repo      string              `json:"repo"`
	Scope     string              `json:"scope"`
	Branch    string              `json:"branch"`
	Flags     []vocab.Flag        `json:"flags"`
	Providers map[string][]string `json:"providers"`
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

func (c *Client) MyMap(ctx context.Context) (sysmap.View, error) {
	var v sysmap.View
	err := c.do(ctx, http.MethodGet, "/api/node/map", nil, &v)
	return v, err
}

func (c *Client) SetSystemInfo(ctx context.Context, p sysmap.SystemPatch) error {
	return c.do(ctx, http.MethodPatch, "/api/node/map/system", p, nil)
}

func (c *Client) UpsertFeature(ctx context.Context, f sysmap.FeatureInput) error {
	return c.do(ctx, http.MethodPut, "/api/node/map/features", f, nil)
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
