// Package editor serves the topology API of the UI (section 6 of the
// topology API spec): it reads, lints, plans, and saves the topology file,
// and commits each save in a project repository.
package editor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/gitrepo"
	"github.com/nathanaday/hikari/internal/launcher"
	"github.com/nathanaday/hikari/internal/linked"
	"github.com/nathanaday/hikari/internal/scaffold"
	"github.com/nathanaday/hikari/internal/topoedit"
	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/internal/vault"
)

// Committer commits paths of the project repository, relative to it, as
// the user, and returns the commit. During a run, the partitions manager
// commits (decision 8 of the topology API spec).
type Committer func(paths []string, message string) (string, error)

// Editor edits one topology file.
type Editor struct {
	// File is the topology file's absolute path.
	File string
	// WriteSkills writes the host skills into a new project repository.
	// The command sets it, so that this package needs nothing of the
	// agents' side.
	WriteSkills func(dir string) error

	mu        sync.Mutex
	committer Committer
	running   bool
}

func New(file string) (*Editor, error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}
	return &Editor{File: abs}, nil
}

// SetRun tells the editor whether agents run, and who commits while they
// do. A nil committer commits with git directly.
func (e *Editor) SetRun(running bool, c Committer) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.running, e.committer = running, c
}

func (e *Editor) dir() string { return filepath.Dir(e.File) }

// Revision is the SHA-256 of the file's bytes, or "" for a missing file
// (rule TA1).
func Revision(text []byte, exists bool) string {
	if !exists {
		return ""
	}
	sum := sha256.Sum256(text)
	return hex.EncodeToString(sum[:])
}

func (e *Editor) read() ([]byte, bool, error) {
	text, err := os.ReadFile(e.File)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	return text, err == nil, err
}

// CurrentRevision returns the revision of the file on disk.
func (e *Editor) CurrentRevision() string {
	text, ok, _ := e.read()
	return Revision(text, ok)
}

// Lint runs the stages of rules TL1 to TL3 on a text of the file.
func (e *Editor) Lint(ctx context.Context, text []byte) []topology.Finding {
	return Lint(ctx, text, e.File)
}

// Lint runs the stages of rules TL1 to TL3 on a text of file.
func Lint(ctx context.Context, text []byte, file string) []topology.Finding {
	out, _ := lint(ctx, text, file)
	return out
}

// lint is Lint, and returns the topology when the text loads.
func lint(ctx context.Context, text []byte, file string) ([]topology.Finding, *topology.Topology) {
	if f := topology.SyntaxFinding(text); f != nil {
		return []topology.Finding{*f}, nil
	}
	topo, err := topology.LoadText(ctx, text, file)
	if err != nil {
		return topology.Findings(err, text), nil
	}
	out := []topology.Finding{}
	for _, w := range topo.Warnings {
		out = append(out, topology.Finding{Severity: topology.SeverityWarning, Stage: topology.StageTopology, Message: w})
	}
	for _, f := range linked.Check(topo, true) {
		out = append(out, topology.Finding{Severity: topology.SeverityWarning, Stage: topology.StageStart, Message: f.Text})
	}
	return out, topo
}

// MountView is one mount of an agent's container, as its inspector shows
// it (rule V14 of the vault layout spec).
type MountView struct {
	Container string `json:"container"`
	// Host is relative to the project when it is inside it, and "" for
	// the empty directory that hides a path.
	Host string `json:"host"`
	Mode string `json:"mode"`
}

const (
	ModeReadOnly = "read-only"
	ModeWritable = "writable"
)

// mounts returns the hikari mounts of each agent, by service id, from the
// function that writes the override file. It is nil when the text has
// errors.
func (e *Editor) mounts(findings []topology.Finding, topo *topology.Topology) map[string][]MountView {
	if topo == nil || HasError(findings) {
		return nil
	}
	out := map[string][]MountView{}
	for _, name := range topo.AgentNames() {
		views := []MountView{}
		for _, m := range launcher.AgentMounts(topo, name, "") {
			v := MountView{Container: m.Target, Host: m.Source, Mode: ModeWritable}
			if m.ReadOnly {
				v.Mode = ModeReadOnly
			}
			if rel, err := filepath.Rel(e.dir(), m.Source); m.Source != "" && err == nil && filepath.IsLocal(rel) {
				v.Host = filepath.ToSlash(rel)
			}
			views = append(views, v)
		}
		out[topo.Agents[name].ID] = views
	}
	return out
}

// HasError reports whether a finding is an error.
func HasError(fs []topology.Finding) bool {
	for _, f := range fs {
		if f.Severity == topology.SeverityError {
			return true
		}
	}
	return false
}

// State is the answer of GET /api/ui/topology.
type State struct {
	File     string             `json:"file"`
	Exists   bool               `json:"exists"`
	Revision string             `json:"revision"`
	Text     string             `json:"text"`
	Model    *topoedit.Model    `json:"model"`
	Findings []topology.Finding `json:"findings"`
	// Mounts maps each agent's service id to its mounts, when the text
	// passes the checks.
	Mounts  map[string][]MountView `json:"mounts"`
	Mode    string                 `json:"mode"`
	Commits bool                   `json:"commits"`
	// Packs maps each role of an agent to its built-in skills, which the
	// inspector lists as from hikari (rule K14 of the skill packs spec).
	Packs map[string][]string `json:"packs"`
}

func (e *Editor) State(ctx context.Context) (State, error) {
	text, ok, err := e.read()
	if err != nil {
		return State{}, err
	}
	s := State{File: linked.Shown(e.File), Exists: ok, Revision: Revision(text, ok), Text: string(text), Findings: []topology.Finding{}, Mode: e.mode(), Packs: api.Packs}
	if ok {
		s.Model = topoedit.ReadModel(text, e.dir())
		var topo *topology.Topology
		s.Findings, topo = lint(ctx, text, e.File)
		s.Mounts = e.mounts(s.Findings, topo)
		s.Commits = e.commits(s.Model)
	}
	return s, nil
}

func (e *Editor) mode() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running {
		return "running"
	}
	return "editing"
}

// commits reports whether a save makes a commit (rule TC1).
func (e *Editor) commits(m *topoedit.Model) bool {
	if m == nil || !m.Project.Repo {
		return false
	}
	_, err := os.Stat(filepath.Join(e.dir(), ".git"))
	return err == nil
}

// Plan is the answer of POST /api/ui/topology/plan.
type Plan struct {
	Revision    string                `json:"revision"`
	Text        string                `json:"text"`
	Diff        []topoedit.DiffLine   `json:"diff"`
	Findings    []topology.Finding    `json:"findings"`
	Effects     []string              `json:"effects"`
	Occurrences []topoedit.Occurrence `json:"occurrences"`
	Summary     []string              `json:"summary"`
	// Model is the model of the new text, which the Structure tab shows
	// (decision 1 of the structure view spec).
	Model *topoedit.Model `json:"model"`
	// Mounts maps each agent's service id to its mounts, when the planned
	// text passes the checks (rule V14).
	Mounts         map[string][]MountView `json:"mounts"`
	AppliesAtStart bool                   `json:"applies_at_start"`
	Packs          map[string][]string    `json:"packs"`
}

// ConflictError is a write whose revision is not the file's (rule TA2).
type ConflictError struct {
	Revision string `json:"revision"`
	Text     string `json:"text"`
}

func (e *ConflictError) Error() string {
	return "the topology file changed since you opened it; reload it, or apply your changes again"
}

func (e *Editor) checkRevision(revision string) ([]byte, error) {
	text, ok, err := e.read()
	if err != nil {
		return nil, err
	}
	if rev := Revision(text, ok); rev != revision {
		return nil, &ConflictError{Revision: rev, Text: string(text)}
	}
	if !ok {
		return nil, fmt.Errorf("%w: %s does not exist; create it first", topoedit.ErrNotFound, linked.Shown(e.File))
	}
	return text, nil
}

func (e *Editor) vault(m *topoedit.Model) string {
	if m == nil {
		return ""
	}
	if m.Project.Vault == "" {
		return filepath.Join(e.dir(), topology.DefaultVault)
	}
	if topoedit.IsExpression(m.Project.Vault) {
		return ""
	}
	if filepath.IsAbs(m.Project.Vault) {
		return m.Project.Vault
	}
	return filepath.Join(e.dir(), m.Project.Vault)
}

func (e *Editor) Plan(ctx context.Context, revision string, ops []topoedit.Op) (Plan, error) {
	text, err := e.checkRevision(revision)
	if err != nil {
		return Plan{}, err
	}
	res, err := topoedit.Apply(text, ops, e.dir())
	if err != nil {
		return Plan{}, err
	}
	m := topoedit.ReadModel(text, e.dir())
	findings, topo := lint(ctx, res.Text, e.File)
	return Plan{
		Revision: revision, Text: string(res.Text), Diff: topoedit.Diff(string(text), string(res.Text)),
		Findings: findings, Effects: nonNil(res.Effects), Summary: nonNil(res.Summary),
		Occurrences: topoedit.Occurrences(e.vault(m), res.Renames), AppliesAtStart: e.mode() == "running",
		Model: topoedit.ReadModel(res.Text, e.dir()), Mounts: e.mounts(findings, topo), Packs: api.Packs,
	}, nil
}

// Saved is the answer of a save.
type Saved struct {
	Revision string   `json:"revision"`
	Commit   string   `json:"commit,omitempty"`
	Note     string   `json:"note,omitempty"`
	Skipped  []string `json:"skipped"`
}

func (e *Editor) Apply(revision string, ops []topoedit.Op, propagate []string) (Saved, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	text, err := e.checkRevision(revision)
	if err != nil {
		return Saved{}, err
	}
	res, err := topoedit.Apply(text, ops, e.dir())
	if err != nil {
		return Saved{}, err
	}
	m := topoedit.ReadModel(text, e.dir())
	if err := writeFile(e.File, res.Text); err != nil {
		return Saved{}, err
	}
	paths := []string{filepath.Base(e.File)}
	var skipped []string
	if len(propagate) > 0 {
		vault := e.vault(m)
		changed, sk, err := topoedit.Propagate(vault, res.Renames, propagate)
		if err != nil {
			return Saved{}, err
		}
		skipped = sk
		for _, c := range changed {
			if rel, err := filepath.Rel(e.dir(), filepath.Join(vault, filepath.FromSlash(c))); err == nil && filepath.IsLocal(rel) {
				paths = append(paths, filepath.ToSlash(rel))
			}
		}
	}
	saved := Saved{Revision: Revision(res.Text, true), Skipped: nonNil(skipped)}
	saved.Commit, saved.Note = e.commit(m, paths, message(res.Summary))
	return saved, nil
}

// message makes the commit message of rule TC2: at most four parts, then
// a count.
func message(summary []string) string {
	if len(summary) == 0 {
		return "user: edit the topology"
	}
	parts := summary
	if len(parts) > 4 {
		parts = append(append([]string{}, summary[:4]...), fmt.Sprintf("%d more", len(summary)-4))
	}
	return "user: " + strings.Join(parts, "; ")
}

func (e *Editor) PutSource(revision string, text []byte) (Saved, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	old, err := e.checkRevision(revision)
	if err != nil {
		return Saved{}, err
	}
	if err := writeFile(e.File, text); err != nil {
		return Saved{}, err
	}
	saved := Saved{Revision: Revision(text, true), Skipped: []string{}}
	// Whether the project commits is the old file's or the new one's.
	m := topoedit.ReadModel(text, e.dir())
	if m == nil {
		m = topoedit.ReadModel(old, e.dir())
	}
	saved.Commit, saved.Note = e.commit(m, []string{filepath.Base(e.File)}, "user: edit "+filepath.Base(e.File))
	return saved, nil
}

// commit commits paths as the user (rules TC1 to TC4). It returns the
// commit, or a note that says why there is none.
func (e *Editor) commit(m *topoedit.Model, paths []string, msg string) (string, string) {
	if !e.commits(m) {
		return "", "not committed: the topology is not in a project repository"
	}
	c := e.committer
	if c == nil {
		repo := gitrepo.Open(e.dir())
		c = func(paths []string, msg string) (string, error) {
			return repo.CommitPaths(repo.Branch(), paths, "", msg)
		}
	}
	commit, err := c(paths, msg)
	if err != nil {
		log.Printf("topology commit: %v", err)
		return "", "not committed: " + err.Error()
	}
	return commit, ""
}

// Create writes a new project or topology file (rule TA6).
func (e *Editor) Create() (Saved, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok, _ := e.read(); ok {
		return Saved{}, fmt.Errorf("%s exists", linked.Shown(e.File))
	}
	dir := e.dir()
	name := projectName(dir)
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Saved{}, err
	}
	saved := Saved{Skipped: []string{}}
	if scaffold.Empty(entries) && filepath.Base(e.File) == scaffold.TopologyFile {
		writeSkills := e.WriteSkills
		if writeSkills == nil {
			writeSkills = func(string) error { return nil }
		}
		committed, err := scaffold.WriteProject(dir, name, writeSkills)
		if err != nil {
			return Saved{}, err
		}
		if !committed {
			saved.Note = "git has no identity, so the new project is not committed"
		}
	} else if err := writeFile(e.File, []byte(scaffold.StarterTopology(name, false))); err != nil {
		return Saved{}, err
	}
	text, _, _ := e.read()
	saved.Revision = Revision(text, true)
	return saved, nil
}

// projectName makes a compose project name from a directory, as hikari
// init does.
func projectName(dir string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(filepath.Base(dir)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	name := strings.Trim(b.String(), "-_")
	switch {
	case name == "":
		return "project"
	case name[0] >= '0' && name[0] <= '9':
		// YAML reads a name such as 002 as a number, which compose refuses.
		return "project-" + name
	}
	return name
}

// Options is the answer of GET /api/ui/topology/options (rule TA7).
type Options struct {
	Deliverables []string `json:"deliverables"`
	Skills       []string `json:"skills"`
	Models       []string `json:"models"`
	Templates    []string `json:"templates"`
}

func (e *Editor) Options() Options {
	o := Options{Deliverables: []string{}, Skills: []string{}, Models: []string{"sonnet", "opus", "haiku"}, Templates: []string{}}
	text, ok, _ := e.read()
	if !ok {
		return o
	}
	m := topoedit.ReadModel(text, e.dir())
	if m == nil {
		return o
	}
	if root := e.vault(m); root != "" {
		st := vault.Vault{Root: root}
		if names, err := st.DeliverableNames(); err == nil && names != nil {
			o.Deliverables = names
		}
		if names := st.SkillNames(); names != nil {
			o.Skills = names
		}
	}
	for _, a := range m.Agents {
		o.Templates = append(o.Templates, a.ID)
	}
	return o
}

// Watch calls changed with the new revision within a second of each
// change of the file on disk (rule TA8), until ctx ends.
func (e *Editor) Watch(ctx context.Context, changed func(revision string)) {
	last := e.CurrentRevision()
	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if rev := e.CurrentRevision(); rev != last {
				last = rev
				changed(rev)
			}
		}
	}
}

// writeFile writes with an atomic rename, keeping the file's mode.
func writeFile(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".hikari-new")
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// Routes serves the topology API on mux. ui guards each endpoint with the
// UI cookie.
func (e *Editor) Routes(mux *http.ServeMux, ui func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /api/ui/topology", ui(func(w http.ResponseWriter, r *http.Request) {
		s, err := e.State(r.Context())
		respond(w, s, err)
	}))
	mux.HandleFunc("POST /api/ui/topology/lint", ui(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Text string `json:"text"`
		}
		if !decode(w, r, &req) {
			return
		}
		respond(w, map[string]any{"findings": e.Lint(r.Context(), []byte(req.Text))}, nil)
	}))
	mux.HandleFunc("POST /api/ui/topology/plan", ui(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Revision   string        `json:"revision"`
			Operations []topoedit.Op `json:"operations"`
		}
		if !decode(w, r, &req) {
			return
		}
		p, err := e.Plan(r.Context(), req.Revision, req.Operations)
		respond(w, p, err)
	}))
	mux.HandleFunc("POST /api/ui/topology/apply", ui(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Revision   string        `json:"revision"`
			Operations []topoedit.Op `json:"operations"`
			Propagate  []string      `json:"propagate"`
		}
		if !decode(w, r, &req) {
			return
		}
		s, err := e.Apply(req.Revision, req.Operations, req.Propagate)
		respond(w, s, err)
	}))
	mux.HandleFunc("PUT /api/ui/topology/source", ui(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Revision string `json:"revision"`
			Text     string `json:"text"`
		}
		if !decode(w, r, &req) {
			return
		}
		s, err := e.PutSource(req.Revision, []byte(req.Text))
		respond(w, s, err)
	}))
	mux.HandleFunc("POST /api/ui/topology/create", ui(func(w http.ResponseWriter, r *http.Request) {
		s, err := e.Create()
		respond(w, s, err)
	}))
	mux.HandleFunc("GET /api/ui/topology/options", ui(func(w http.ResponseWriter, r *http.Request) {
		respond(w, e.Options(), nil)
	}))
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return false
	}
	return true
}

// respond writes v, or the error with its status (rules TA2 and TA9).
func respond(w http.ResponseWriter, v any, err error) {
	var conflict *ConflictError
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, v)
	case errors.As(err, &conflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": conflict.Error(), "revision": conflict.Revision, "text": conflict.Text})
	case errors.Is(err, topoedit.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
