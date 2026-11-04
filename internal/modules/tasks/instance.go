package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nathanaday/hikari/internal/forge"
	"github.com/nathanaday/hikari/internal/module"
	"github.com/nathanaday/hikari/internal/topology"
)

// WorkQueue is the id of the queue that Work on this uses (decision 9 of
// the repositories, forges, and issues spec).
const WorkQueue = "work"

// SyncInterval is how often the trackers' issues are read (rule IS2).
const SyncInterval = time.Minute

const trackerTimeout = 45 * time.Second

// instance is the host half of the pipeline, for one run.
type instance struct {
	mu    sync.Mutex
	host  module.Host
	topo  *topology.Topology
	store *Store
	// issues holds each tracker's issues by repo, synced the last time the
	// tracker was read, and syncErr why it could not be.
	issues  map[string][]forge.Issue
	synced  map[string]time.Time
	syncErr map[string]string
	// syncing serializes the syncs, and hooks carries each change's hooks
	// to one worker, so that the comments reach an issue in order.
	syncing sync.Mutex
	hooks   chan []Hook
}

// open opens the store of the vault, completes the tasks you wrote by
// hand, writes each scope's queue, and starts what can start.
func open(h module.Host) (*instance, error) {
	topo := h.Topology()
	check := Check{
		Partitions:    topo.PartitionAgents(),
		Orchestration: topo.Orchestration,
		Feature: func(p, id string) bool {
			_, ok := h.Health(p, id)
			return ok
		},
		Health: h.Health,
	}
	st, err := Open(filepath.Join(h.VaultDir(), Dir), check, time.Now)
	if err != nil {
		return nil, err
	}
	i := &instance{host: h, topo: topo, store: st, issues: map[string][]forge.Issue{}, synced: map[string]time.Time{}, syncErr: map[string]string{}, hooks: make(chan []Hook, 256)}
	go func() {
		for hooks := range i.hooks {
			i.runHooks(hooks)
		}
	}()
	i.mu.Lock()
	defer i.mu.Unlock()
	i.settle(&Change{})
	return i, nil
}

// settle completes, syncs, and schedules after a change, and applies it.
func (i *instance) settle(c *Change) {
	if filled, err := i.store.Fill(); err != nil {
		log.Printf("tasks: %v", err)
	} else {
		c.wrote(Hikari, filled...)
	}
	if synced, err := i.store.SyncQueues(); err != nil {
		log.Printf("tasks: %v", err)
	} else {
		c.wrote(Hikari, synced...)
	}
	if err := i.store.Schedule(c); err != nil {
		log.Printf("tasks: %v", err)
	}
	i.apply(c, "")
}

// apply commits each write as its author, sends hikari's messages, and
// runs the hooks of the trackers.
func (i *instance) apply(c *Change, what string) {
	for _, w := range c.Writes {
		paths := make([]string, len(w.Paths))
		for k, p := range w.Paths {
			paths[k] = path.Join(Dir, p)
		}
		msg := what
		if msg == "" || w.Author != c.Writes[0].Author {
			msg = "tasks: move the queues forward"
		}
		i.host.Commit(w.Author, w.Author+": "+msg, paths...)
	}
	for _, n := range c.Notes {
		i.host.Notify(n.To, n.Body)
	}
	for _, l := range i.store.TakeLines() {
		i.host.Feed(l.Text, l.Task)
	}
	if hooks := i.store.TakeHooks(); len(hooks) > 0 {
		select {
		case i.hooks <- hooks:
		default:
			go i.runHooks(hooks)
		}
	}
	if len(c.Writes) > 0 {
		go i.host.Changed()
	}
}

// Watch reloads the folder each second, so that your edits take effect,
// and commits them as yours (rule TK7); and reads the trackers' issues
// every minute (rule IS2).
func (i *instance) Watch(ctx context.Context) {
	go i.sync(ctx)
	t := time.NewTicker(time.Second)
	defer t.Stop()
	s := time.NewTicker(SyncInterval)
	defer s.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			i.reload()
		case <-s.C:
			go i.sync(ctx)
		}
	}
}

func (i *instance) reload() {
	i.mu.Lock()
	defer i.mu.Unlock()
	changed, err := i.store.Reload()
	if err != nil {
		log.Printf("tasks: %v", err)
		return
	}
	if len(changed) == 0 {
		return
	}
	c := &Change{}
	c.wrote(User, changed...)
	c.Writes[0].Paths = changed
	i.applyEdits(c)
	i.settle(&Change{})
	go i.host.Changed()
}

func (i *instance) applyEdits(c *Change) {
	for _, w := range c.Writes {
		paths := make([]string, len(w.Paths))
		for k, p := range w.Paths {
			paths[k] = path.Join(Dir, p)
		}
		i.host.Commit(topology.User, "user: task edits", paths...)
	}
}

// sync reads every tracker's issues, and tells the UI when they changed.
func (i *instance) sync(ctx context.Context) {
	i.syncing.Lock()
	defer i.syncing.Unlock()
	changed := false
	for _, repo := range i.host.TrackerRepos() {
		tr, ok := i.host.Tracker(repo)
		if !ok {
			continue
		}
		tctx, cancel := context.WithTimeout(ctx, trackerTimeout)
		issues, err := tr.Issues(tctx)
		cancel()
		i.mu.Lock()
		switch {
		case err != nil:
			if i.syncErr[repo] != err.Error() {
				changed = true
			}
			i.syncErr[repo] = err.Error()
		default:
			before, _ := json.Marshal(i.issues[repo])
			after, _ := json.Marshal(issues)
			if string(before) != string(after) || i.syncErr[repo] != "" {
				changed = true
			}
			i.issues[repo], i.syncErr[repo], i.synced[repo] = issues, "", time.Now().UTC()
		}
		i.mu.Unlock()
	}
	if changed {
		i.host.Changed()
	}
}

// do runs one change of the pipeline under the lock, and settles it.
func (i *instance) do(what string, fn func(c *Change) (any, error)) (any, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	c := &Change{}
	out, err := fn(c)
	if err != nil {
		if errors.Is(err, errNotFound) {
			err = fmt.Errorf("%w: %s", module.ErrNotFound, err.Error())
		} else if errors.Is(err, errForbidden) {
			err = fmt.Errorf("%w: %s", module.ErrForbidden, err.Error())
		}
		return nil, err
	}
	i.apply(c, what)
	i.settle(&Change{})
	return out, nil
}

func decode(r *http.Request, v any) error {
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(v); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}

// IssueRequest creates an issue: on the tracker of Repo, on the first
// tracker when Repo is empty, or in the vault when the project has none.
type IssueRequest struct {
	Repo   string   `json:"repo,omitempty"`
	Title  string   `json:"title"`
	Body   string   `json:"body,omitempty"`
	Labels []string `json:"labels,omitempty"`
	// Local asks for a local issue even when a tracker exists.
	Local bool `json:"local,omitempty"`
}

// IssuePatchRequest changes an issue. A nil field is left as it is. For a
// local issue, state closed removes it.
type IssuePatchRequest struct {
	Title  *string   `json:"title,omitempty"`
	Body   *string   `json:"body,omitempty"`
	State  *string   `json:"state,omitempty"`
	Labels *[]string `json:"labels,omitempty"`
}

// CommentRequest adds a comment to a tracker issue.
type CommentRequest struct {
	Body string `json:"body"`
}

// WorkRequest hands an issue to the agents: it goes to the front of the
// work queue, which starts (decision 9).
type WorkRequest struct {
	Issue string `json:"issue"`
	// Queue is the queue to use instead of the work queue.
	Queue string `json:"queue,omitempty"`
}

// StatusRequest sets a status: the caller's part, the scope, or for the
// user, the part that Part names.
type StatusRequest struct {
	Task   string `json:"task"`
	Part   string `json:"part,omitempty"`
	Status string `json:"status"`
	Note   string `json:"note,omitempty"`
}

// StateRequest starts or pauses a queue.
type StateRequest struct {
	Queue string `json:"queue"`
	State string `json:"state"`
}

// tracker returns the tracker of a repo, or an error that names the ones
// the project has.
func (i *instance) tracker(repo string) (forge.Tracker, error) {
	if tr, ok := i.host.Tracker(repo); ok {
		return tr, nil
	}
	repos := i.host.TrackerRepos()
	if len(repos) == 0 {
		return nil, fmt.Errorf("%w: no repository of the project has an issue tracker", errNotFound)
	}
	return nil, fmt.Errorf("%w: %q has no issue tracker; the repositories with one are %s", errNotFound, repo, strings.Join(repos, ", "))
}

// cachedIssue finds an issue in the last sync.
func (i *instance) cachedIssue(repo string, number int) (forge.Issue, bool) {
	for _, is := range i.issues[repo] {
		if is.Number == number {
			return is, true
		}
	}
	return forge.Issue{}, false
}

// resolve returns the task of an issue ref, which it makes for a tracker
// issue that has none yet (rule IS1), and the paths it wrote. A ref that is
// not <repo>#<n> is a task id.
func (i *instance) resolve(ctx context.Context, ref string) (*Task, []string, error) {
	repo, n, ok := ParseIssueRef(ref)
	if !ok {
		t, err := i.store.task(ref)
		return t, nil, err
	}
	if t := i.store.TaskForIssue(IssueRef(repo, n)); t != nil {
		return t, nil, nil
	}
	tr, err := i.tracker(repo)
	if err != nil {
		return nil, nil, err
	}
	is, ok := i.cachedIssue(repo, n)
	if !ok {
		tctx, cancel := context.WithTimeout(ctx, trackerTimeout)
		defer cancel()
		is, _, err = tr.Issue(tctx, n)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %s", errNotFound, err.Error())
		}
	}
	return i.store.EnsureIssueTask(repo, is)
}

func (i *instance) createIssue(r *http.Request, caller string) (any, error) {
	var req IssueRequest
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Title) == "" {
		return nil, errors.New("an issue needs a title")
	}
	repo := req.Repo
	if repo == "" && !req.Local {
		if repos := i.host.TrackerRepos(); len(repos) > 0 {
			repo = repos[0]
		}
	}
	if repo == "" {
		return i.do("create an issue", func(c *Change) (any, error) {
			t, paths, err := i.store.PlantStub(req.Title, req.Body, caller)
			if err != nil {
				return nil, err
			}
			c.wrote(caller, paths...)
			return i.issueOfTask(t), nil
		})
	}
	tr, err := i.tracker(repo)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", module.ErrNotFound, err.Error())
	}
	body := strings.TrimSpace(req.Body)
	if caller != User {
		body = strings.TrimSpace(body + "\n\n_Opened by the hikari agent " + caller + "._")
	}
	ctx, cancel := context.WithTimeout(r.Context(), trackerTimeout)
	defer cancel()
	is, err := tr.CreateIssue(ctx, forge.NewIssue{Title: strings.TrimSpace(req.Title), Body: body, Labels: req.Labels})
	if err != nil {
		return nil, err
	}
	i.mu.Lock()
	i.issues[repo] = append([]forge.Issue{is}, i.issues[repo]...)
	i.mu.Unlock()
	i.host.Feed(fmt.Sprintf("%s opened the issue %s: %s", caller, IssueRef(repo, is.Number), is.Title), "")
	go i.host.Changed()
	v := i.issueOf(repo, is, nil)
	return v, nil
}

// IssueDetail is one issue with its comments and its task's document.
type IssueDetail struct {
	Issue    IssueView       `json:"issue"`
	Comments []forge.Comment `json:"comments"`
	Task     *TaskText       `json:"task,omitempty"`
}

func (i *instance) getIssue(r *http.Request, caller string) (any, error) {
	ref := r.PathValue("ref")
	repo, n, ok := ParseIssueRef(ref)
	i.mu.Lock()
	if !ok {
		t, err := i.store.task(ref)
		if err != nil || t.Issue != "" {
			i.mu.Unlock()
			return nil, fmt.Errorf("%w: no issue %s", module.ErrNotFound, ref)
		}
		d := IssueDetail{Issue: i.issueOfTask(t), Comments: []forge.Comment{}, Task: &TaskText{Task: i.taskView(t), Text: t.Format()}}
		i.mu.Unlock()
		return d, nil
	}
	task := i.store.TaskForIssue(IssueRef(repo, n))
	i.mu.Unlock()
	tr, err := i.tracker(repo)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", module.ErrNotFound, err.Error())
	}
	ctx, cancel := context.WithTimeout(r.Context(), trackerTimeout)
	defer cancel()
	is, comments, err := tr.Issue(ctx, n)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", module.ErrNotFound, err.Error())
	}
	if comments == nil {
		comments = []forge.Comment{}
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	d := IssueDetail{Issue: i.issueOf(repo, is, task), Comments: comments}
	if task != nil {
		d.Task = &TaskText{Task: i.taskView(task), Text: task.Format()}
	}
	return d, nil
}

func (i *instance) patchIssue(r *http.Request, caller string) (any, error) {
	ref := r.PathValue("ref")
	var req IssuePatchRequest
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	repo, n, ok := ParseIssueRef(ref)
	if !ok {
		if req.State != nil && *req.State == forge.Closed {
			return i.do("remove the issue "+ref, func(c *Change) (any, error) {
				paths, err := i.store.RemoveLocal(ref)
				c.wrote(caller, paths...)
				return nil, err
			})
		}
		return i.do("change the issue "+ref, func(c *Change) (any, error) {
			t, paths, err := i.store.UpdateLocal(ref, req.Title, req.Body)
			if err != nil {
				return nil, err
			}
			c.wrote(caller, paths...)
			return i.issueOfTask(t), nil
		})
	}
	tr, err := i.tracker(repo)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", module.ErrNotFound, err.Error())
	}
	ctx, cancel := context.WithTimeout(r.Context(), trackerTimeout)
	defer cancel()
	is, err := tr.UpdateIssue(ctx, n, forge.IssuePatch{Title: req.Title, Body: req.Body, State: req.State, Labels: req.Labels})
	if err != nil {
		return nil, err
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.replaceIssue(repo, is)
	go i.host.Changed()
	return i.issueOf(repo, is, i.store.TaskForIssue(IssueRef(repo, n))), nil
}

// replaceIssue puts a fresh copy of an issue in the cache.
func (i *instance) replaceIssue(repo string, is forge.Issue) {
	list := i.issues[repo]
	if k := slices.IndexFunc(list, func(x forge.Issue) bool { return x.Number == is.Number }); k >= 0 {
		list[k] = is
	} else {
		i.issues[repo] = append([]forge.Issue{is}, list...)
	}
}

func (i *instance) commentIssue(r *http.Request, caller string) (any, error) {
	ref := r.PathValue("ref")
	var req CommentRequest
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Body) == "" {
		return nil, errors.New("a comment needs a body")
	}
	repo, n, ok := ParseIssueRef(ref)
	if !ok {
		return nil, fmt.Errorf("%s is a local issue, which takes no comments; edit its body instead", ref)
	}
	tr, err := i.tracker(repo)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", module.ErrNotFound, err.Error())
	}
	body := strings.TrimSpace(req.Body)
	if caller != User {
		body = "**" + caller + "** (hikari agent):\n\n" + body
	}
	ctx, cancel := context.WithTimeout(r.Context(), trackerTimeout)
	defer cancel()
	c, err := tr.Comment(ctx, n, body)
	if err != nil {
		return nil, err
	}
	i.host.Feed(fmt.Sprintf("%s commented on the issue %s", caller, ref), "")
	return c, nil
}

// work puts an issue at the front of a queue, which it makes when missing,
// and starts the queue (decision 9).
func (i *instance) work(r *http.Request, caller string) (any, error) {
	var req WorkRequest
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Issue) == "" {
		return nil, errors.New("say which issue to work on")
	}
	queue := req.Queue
	if queue == "" {
		queue = WorkQueue
	}
	return i.do("work on "+req.Issue, func(c *Change) (any, error) {
		t, paths, err := i.resolve(r.Context(), req.Issue)
		if err != nil {
			return nil, err
		}
		c.wrote(caller, paths...)
		if _, ok := i.store.Queues[queue]; !ok {
			if queue != WorkQueue {
				return nil, fmt.Errorf("%w: no queue %s", errNotFound, queue)
			}
			paths, err := i.store.ChangeQueue(QueueOp{Op: "create", Name: "Work"})
			if err != nil {
				return nil, err
			}
			c.wrote(caller, paths...)
		}
		if from := i.store.QueueOf(t.ID); from == "" {
			paths, err = i.store.ChangeQueue(QueueOp{Op: "add", Queue: queue, Scope: t.ID, Position: 1})
		} else if movable(t) {
			paths, err = i.store.ChangeQueue(QueueOp{Op: "move", Queue: queue, Scope: t.ID, Position: 1})
		} else {
			paths, err = nil, nil
		}
		if err != nil {
			return nil, err
		}
		c.wrote(caller, paths...)
		if err := i.store.SetQueueState(queue, Running, c); err != nil {
			return nil, err
		}
		return i.taskView(t), nil
	})
}

func (i *instance) setStatus(r *http.Request, caller string) (any, error) {
	var req StatusRequest
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	return i.do("set the status of [["+req.Task+"]]", func(c *Change) (any, error) {
		g, err := i.store.SetStatus(caller, req.Task, req.Part, req.Status, req.Note, c)
		return g, err
	})
}

func (i *instance) changeQueue(r *http.Request, caller string) (any, error) {
	var op QueueOp
	if err := decode(r, &op); err != nil {
		return nil, err
	}
	return i.do("change the queues ("+op.Op+")", func(c *Change) (any, error) {
		if op.Issue != "" && op.Scope == "" {
			t, paths, err := i.resolve(r.Context(), op.Issue)
			if err != nil {
				return nil, err
			}
			c.wrote(caller, paths...)
			op.Scope = t.ID
		}
		paths, err := i.store.ChangeQueue(op)
		if err != nil {
			return nil, err
		}
		c.wrote(caller, paths...)
		return nil, nil
	})
}

func (i *instance) syncNow(r *http.Request, caller string) (any, error) {
	i.sync(r.Context())
	return i.View(), nil
}

// AgentRoutes are the agents' API (rules TK8, TK10, TK11, TK14, TK16, TK21,
// and IS7).
func (i *instance) AgentRoutes() module.Routes {
	orchestration := func(h module.Handler) module.Handler {
		return func(r *http.Request, caller string) (any, error) {
			if caller != i.topo.Orchestration {
				return nil, fmt.Errorf("%w: only the orchestration agent does this", module.ErrForbidden)
			}
			return h(r, caller)
		}
	}
	return module.Routes{
		"POST /issues":                i.createIssue,
		"GET /issues":                 orchestration(func(r *http.Request, caller string) (any, error) { return i.View(), nil }),
		"GET /issues/{ref}":           orchestration(i.getIssue),
		"POST /issues/{ref}/comments": orchestration(i.commentIssue),
		"POST /status":                i.setStatus,
		"POST /queue":                 orchestration(i.changeQueue),
		"POST /scope": orchestration(func(r *http.Request, caller string) (any, error) {
			var in ScopeInput
			if err := decode(r, &in); err != nil {
				return nil, err
			}
			return i.do("scope [["+in.ID+"]]", func(c *Change) (any, error) {
				t, paths, err := i.resolve(r.Context(), in.ID)
				if err != nil {
					return nil, err
				}
				c.wrote(caller, paths...)
				in.ID = t.ID
				t, paths, err = i.store.Scope(in)
				if err != nil {
					return nil, err
				}
				c.wrote(caller, paths...)
				return i.taskView(t), nil
			})
		}),
		"GET /tasks/{id}": func(r *http.Request, caller string) (any, error) {
			i.mu.Lock()
			defer i.mu.Unlock()
			t, err := i.readable(caller, r.PathValue("id"))
			if err != nil {
				return nil, err
			}
			return TaskText{Task: i.taskView(t), Text: t.Format()}, nil
		},
		"GET /list": func(r *http.Request, caller string) (any, error) {
			i.mu.Lock()
			defer i.mu.Unlock()
			return i.listFor(caller), nil
		},
	}
}

// UIRoutes are yours (rules TK8, TK11, TK12, TK16, TK28, and IS8).
func (i *instance) UIRoutes() module.Routes {
	return module.Routes{
		"POST /issues":                i.createIssue,
		"GET /issues/{ref}":           i.getIssue,
		"PATCH /issues/{ref}":         i.patchIssue,
		"POST /issues/{ref}/comments": i.commentIssue,
		"POST /work":                  i.work,
		"POST /sync":                  i.syncNow,
		"POST /status":                i.setStatus,
		"POST /queue":                 i.changeQueue,
		"POST /state": func(r *http.Request, caller string) (any, error) {
			var req StateRequest
			if err := decode(r, &req); err != nil {
				return nil, err
			}
			return i.do("set the queue "+req.Queue+" "+req.State, func(c *Change) (any, error) {
				return nil, i.store.SetQueueState(req.Queue, req.State, c)
			})
		},
		"GET /tasks/{id}": func(r *http.Request, caller string) (any, error) {
			i.mu.Lock()
			defer i.mu.Unlock()
			t, err := i.store.task(r.PathValue("id"))
			if err != nil {
				return nil, fmt.Errorf("%w: %s", module.ErrNotFound, err.Error())
			}
			return TaskText{Task: i.taskView(t), Text: t.Format()}, nil
		},
		"GET /view": func(r *http.Request, caller string) (any, error) { return i.View(), nil },
	}
}

// readable returns a task that caller may read (rule TK21): every task for
// the orchestration agent, and only the scopes that name it for a
// partition agent.
func (i *instance) readable(caller, id string) (*Task, error) {
	t, err := i.store.task(id)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", module.ErrNotFound, err.Error())
	}
	if caller == i.topo.Orchestration {
		return t, nil
	}
	if t.Stage != StageScope || !slices.Contains(t.Partitions, caller) {
		return nil, fmt.Errorf("%w: no scope [[%s]] names you", module.ErrNotFound, strings.Trim(id, "[]"))
	}
	return t, nil
}

// TaskText is a task with its document.
type TaskText struct {
	Task TaskView `json:"task"`
	Text string   `json:"text"`
}

// TaskView is one task, for the UI and the tools.
type TaskView struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Stage      string    `json:"stage"`
	Author     string    `json:"author,omitempty"`
	Status     string    `json:"status,omitempty"`
	Block      string    `json:"block,omitempty"`
	Wait       string    `json:"wait,omitempty"`
	Queue      string    `json:"queue,omitempty"`
	Issue      string    `json:"issue,omitempty"`
	Partitions []string  `json:"partitions"`
	Features   []string  `json:"features"`
	Needs      []string  `json:"needs"`
	Parts      []Part    `json:"parts"`
	Created    time.Time `json:"created,omitzero"`
	Updated    time.Time `json:"updated,omitzero"`
	// File is the task's path in the vault, and Hash says when it changed.
	File string `json:"file"`
	Hash string `json:"hash"`
}

// MarshalJSON of a part, for the UI.
func (p Part) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Partition string `json:"partition"`
		Status    string `json:"status"`
	}{p.Partition, p.Status})
}

func (p *Part) UnmarshalJSON(data []byte) error {
	var v struct {
		Partition string `json:"partition"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	p.Partition, p.Status = v.Partition, v.Status
	return nil
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func (i *instance) taskView(t *Task) TaskView { return i.store.TaskView(t) }

// QueueView is one queue with its scopes in order.
type QueueView struct {
	ID     string     `json:"id"`
	Name   string     `json:"name"`
	State  string     `json:"state"`
	Scopes []TaskView `json:"scopes"`
	File   string     `json:"file"`
}

// Problem is a document that hikari leaves alone, with the reason.
type Problem struct {
	File   string `json:"file"`
	Reason string `json:"reason"`
}

// IssueView is one issue, for the UI and the tools (rule IS2): a tracker's
// issue, or a local one, with its task when it has one.
type IssueView struct {
	// Ref is <repo>#<number> for a tracker issue, and the task id for a
	// local one.
	Ref       string    `json:"ref"`
	Repo      string    `json:"repo,omitempty"`
	Number    int       `json:"number,omitempty"`
	Title     string    `json:"title"`
	Body      string    `json:"body,omitempty"`
	URL       string    `json:"url,omitempty"`
	State     string    `json:"state"`
	Labels    []string  `json:"labels"`
	Author    string    `json:"author,omitempty"`
	Comments  int       `json:"comments"`
	Created   time.Time `json:"created,omitzero"`
	Updated   time.Time `json:"updated,omitzero"`
	Local     bool      `json:"local,omitempty"`
	Task      *TaskView `json:"task,omitempty"`
	Assignees []string  `json:"assignees,omitempty"`
}

// TrackerView is one tracker of the project, with its last sync.
type TrackerView struct {
	Repo   string    `json:"repo"`
	Kind   string    `json:"kind"`
	URL    string    `json:"url,omitempty"`
	Synced time.Time `json:"synced,omitzero"`
	Error  string    `json:"error,omitempty"`
}

// View is the pipeline, for the UI (rules TK27 and IS2).
type View struct {
	Issues        []IssueView   `json:"issues"`
	Trackers      []TrackerView `json:"trackers"`
	Tasks         []TaskView    `json:"tasks"`
	Queues        []QueueView   `json:"queues"`
	Problems      []Problem     `json:"problems"`
	Orchestration string        `json:"orchestration"`
	Partitions    []string      `json:"partitions"`
	// WorkQueue is the queue that Work on this uses.
	WorkQueue string `json:"work_queue"`
}

func (i *instance) View() any {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.view()
}

func (i *instance) issueOf(repo string, is forge.Issue, t *Task) IssueView {
	v := IssueView{Ref: IssueRef(repo, is.Number), Repo: repo, Number: is.Number, Title: is.Title, Body: is.Body, URL: is.URL, State: is.State,
		Labels: nonNil(is.Labels), Author: is.Author, Comments: is.Comments, Created: is.Created, Updated: is.Updated, Assignees: is.Assignees}
	if t != nil {
		tv := i.taskView(t)
		v.Task = &tv
	}
	return v
}

func (i *instance) issueOfTask(t *Task) IssueView {
	tv := i.taskView(t)
	body := t.Body
	if t.Stage == StageScope {
		body, _ = Section(t.Body, SectionRequest)
	} else if start, _, ok := sectionBounds(t.Body, SectionLog); ok {
		body = strings.TrimSpace(t.Body[:start])
	}
	state := forge.Open
	if t.Status == Done {
		state = forge.Closed
	}
	return IssueView{Ref: t.ID, Title: t.Name, Body: body, State: state, Labels: []string{}, Author: t.Author, Created: t.Created, Updated: t.Updated, Local: true, Task: &tv}
}

// view joins the trackers' issues with their tasks, and adds the local
// issues: the tasks that stand for no tracker issue.
func (i *instance) view() View {
	v := i.store.View(i.topo.Orchestration, i.topo.PartitionAgents())
	v.WorkQueue = WorkQueue
	for _, repo := range i.host.TrackerRepos() {
		tv := TrackerView{Repo: repo, Synced: i.synced[repo], Error: i.syncErr[repo]}
		if tr, ok := i.host.Tracker(repo); ok {
			tv.Kind, tv.URL = string(tr.Remote().Kind), tr.Remote().Web
		}
		v.Trackers = append(v.Trackers, tv)
		for _, is := range i.issues[repo] {
			v.Issues = append(v.Issues, i.issueOf(repo, is, i.store.TaskForIssue(IssueRef(repo, is.Number))))
		}
	}
	for _, t := range v.Tasks {
		if t.Issue == "" {
			v.Issues = append(v.Issues, i.issueOfTask(i.store.Tasks[t.ID]))
		}
	}
	// Open issues first, the latest change first; then the closed ones.
	slices.SortStableFunc(v.Issues, func(a, b IssueView) int {
		if (a.State == forge.Open) != (b.State == forge.Open) {
			if a.State == forge.Open {
				return -1
			}
			return 1
		}
		return b.Updated.Compare(a.Updated)
	})
	return v
}

// Notices tell an agent, at the start of each turn, where its scopes stand
// (rule TK24).
func (i *instance) Notices(agent string) []string {
	i.mu.Lock()
	defer i.mu.Unlock()
	var lines []string
	for _, id := range sortedKeys(i.store.Tasks) {
		t := i.store.Tasks[id]
		if !active(t.Status) {
			continue
		}
		switch {
		case agent == i.topo.Orchestration && scoping(t):
			lines = append(lines, fmt.Sprintf("[[%s]] is started and waits for you to scope it with scope_task", id))
		case agent == i.topo.Orchestration:
			lines = append(lines, fmt.Sprintf("[[%s]] is %s (%s)", id, t.Status, partsText(t)))
		case slices.Contains(t.Partitions, agent):
			lines = append(lines, fmt.Sprintf("your part of [[%s]] is %s, and the scope is %s", id, orDefault(t.PartStatus(agent), Queued), t.Status))
		}
	}
	if len(lines) == 0 {
		return nil
	}
	if agent == i.topo.Orchestration {
		return []string{"Scopes in progress: " + strings.Join(lines, "; ") + ". The hikari-scope-work skill says how to scope, carry out, and validate them."}
	}
	return []string{"Your work in the task pipeline: " + strings.Join(lines, "; ") + ". Read a scope with get_task, and report each change of your part with set_task_status, as the hikari-task-work skill says."}
}

func partsText(t *Task) string {
	var out []string
	for _, p := range t.Partitions {
		out = append(out, p+" "+orDefault(t.PartStatus(p), Queued))
	}
	return strings.Join(out, ", ")
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// List is what list_tasks returns.
type List struct {
	Issues []IssueView `json:"issues,omitempty"`
	Queues []QueueView `json:"queues,omitempty"`
	// Scopes are a partition agent's scopes.
	Scopes []TaskView `json:"scopes,omitempty"`
}

func (i *instance) listFor(caller string) List {
	if caller == i.topo.Orchestration {
		v := i.view()
		return List{Issues: v.Issues, Queues: v.Queues}
	}
	var out List
	for _, id := range sortedKeys(i.store.Tasks) {
		t := i.store.Tasks[id]
		if t.Stage == StageScope && slices.Contains(t.Partitions, caller) && t.Status != "" {
			out.Scopes = append(out.Scopes, i.taskView(t))
		}
	}
	return out
}

// The labels that follow a task's status on its issue (decision 10).
const (
	LabelQueued  = "hikari:queued"
	LabelWorking = "hikari:working"
	LabelBlocked = "hikari:blocked"
	LabelDone    = "hikari:done"
)

var hikariLabels = []string{LabelQueued, LabelWorking, LabelBlocked, LabelDone}

// runHooks writes each moment to the task's issue: a comment and the
// labels (decision 10, rule IS6). An error reaches the feed.
func (i *instance) runHooks(hooks []Hook) {
	for _, h := range hooks {
		repo, n, ok := ParseIssueRef(h.Task.Issue)
		if !ok {
			continue
		}
		tr, err := i.tracker(repo)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), trackerTimeout)
		err = i.runHook(ctx, tr, n, h)
		cancel()
		if err != nil {
			i.host.Feed(fmt.Sprintf("[[%s]] could not update the issue %s: %v", h.Task.ID, h.Task.Issue, err), h.Task.ID)
		}
	}
	go i.sync(context.Background())
}

func (i *instance) runHook(ctx context.Context, tr forge.Tracker, n int, h Hook) error {
	t := &h.Task
	var errs []error
	label := func(keep string) {
		for _, l := range hikariLabels {
			if l != keep {
				errs = append(errs, tr.RemoveLabel(ctx, n, l))
			}
		}
		if keep != "" {
			errs = append(errs, tr.AddLabels(ctx, n, keep))
		}
	}
	comment := func(body string) {
		_, err := tr.Comment(ctx, n, body)
		errs = append(errs, err)
	}
	switch h.Kind {
	case HookQueued:
		label(LabelQueued)
	case HookOut:
		label("")
	case HookStarted:
		label(LabelWorking)
		comment(fmt.Sprintf("**hikari** started this issue from the queue `%s`. The orchestration agent is scoping it now, and will comment with the scope.", t.Queue))
	case HookScoped:
		label(LabelWorking)
		comment(i.scopeComment(t))
	case HookBlocked:
		label(LabelBlocked)
		comment("**hikari**: the work is blocked. " + blockReason(t))
	case HookUnblocked:
		label(LabelWorking)
	case HookDone:
		label(LabelDone)
		comment(i.doneComment(t))
	}
	return errors.Join(errs...)
}

func blockReason(t *Task) string {
	if t.Block != "" {
		return t.Block
	}
	for _, p := range t.Parts {
		if p.Status == Blocked {
			return "The part of " + p.Partition + " is blocked; its log in the vault says why."
		}
	}
	return ""
}

var partHeading = regexp.MustCompile(`(?m)^### (.+)$`)

// scopeComment writes the scope for the issue: the goal, each part, and
// what shows that it is done.
func (i *instance) scopeComment(t *Task) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**hikari** scoped this issue as `%s` in the vault.\n", t.ID)
	if goal, ok := Section(t.Body, SectionGoal); ok && goal != "" {
		b.WriteString("\n**Goal**\n\n" + goal + "\n")
	}
	if parts, ok := Section(t.Body, SectionParts); ok && parts != "" {
		b.WriteString("\n**Parts**\n\n" + partHeading.ReplaceAllString(parts, "**$1**") + "\n")
	}
	if done, ok := Section(t.Body, SectionDoneWhen); ok && done != "" {
		b.WriteString("\n**Done when**\n\n" + done + "\n")
	}
	if answers, ok := Section(t.Body, SectionAnswers); ok && answers != "" {
		b.WriteString("\n**Assumptions and answers**\n\n" + answers + "\n")
	}
	return strings.TrimSpace(b.String())
}

// doneComment names the features that pass and the branches that hold the
// work.
func (i *instance) doneComment(t *Task) string {
	var b strings.Builder
	b.WriteString("**hikari** finished this issue: every part is done, and the health of its features passes.")
	if len(t.Features) > 0 {
		b.WriteString("\n\nFeatures: " + strings.Join(t.Features, ", ") + ".")
	}
	branches := map[string]string{}
	for _, p := range t.Partitions {
		if a, ok := i.topo.Agents[p]; ok && a.Partition != nil {
			for _, f := range a.Partition.Writable() {
				branches[f.Repo] = i.topo.Repos[f.Repo].Branch
			}
		}
	}
	if len(branches) > 0 {
		var where []string
		for _, repo := range sortedKeys(branches) {
			where = append(where, fmt.Sprintf("`%s` of %s", branches[repo], repo))
		}
		b.WriteString("\n\nThe work is on the branch " + strings.Join(where, " and ") + ". Open a pull request from the Repositories page of hikari to merge it.")
	}
	return b.String()
}
