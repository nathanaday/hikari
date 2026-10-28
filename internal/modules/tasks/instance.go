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
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nathanaday/hikari/internal/module"
	"github.com/nathanaday/hikari/internal/topology"
)

// instance is the host half of the pipeline, for one run.
type instance struct {
	mu    sync.Mutex
	host  module.Host
	topo  *topology.Topology
	store *Store
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
	i := &instance{host: h, topo: topo, store: st}
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

// apply commits each write as its author, and sends hikari's messages.
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
	if len(c.Writes) > 0 {
		go i.host.Changed()
	}
}

// Watch reloads the folder each second, so that your edits take effect,
// and commits them as yours (rule TK7).
func (i *instance) Watch(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			i.reload()
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

// StubRequest plants a stub.
type StubRequest struct {
	Name string `json:"name"`
	Body string `json:"body,omitempty"`
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

func (i *instance) plant(r *http.Request, caller string) (any, error) {
	var req StubRequest
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	return i.do("plant a stub", func(c *Change) (any, error) {
		t, paths, err := i.store.PlantStub(req.Name, req.Body, caller)
		if err != nil {
			return nil, err
		}
		c.wrote(caller, paths...)
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
		paths, err := i.store.ChangeQueue(op)
		if err != nil {
			return nil, err
		}
		c.wrote(caller, paths...)
		return nil, nil
	})
}

// AgentRoutes are the agents' API (rules TK8, TK10, TK11, TK14, TK16, and
// TK21).
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
		"POST /stub":   i.plant,
		"POST /status": i.setStatus,
		"POST /queue":  orchestration(i.changeQueue),
		"POST /scope": orchestration(func(r *http.Request, caller string) (any, error) {
			var in ScopeInput
			if err := decode(r, &in); err != nil {
				return nil, err
			}
			return i.do("scope [["+in.ID+"]]", func(c *Change) (any, error) {
				t, paths, err := i.store.Scope(in)
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

// UIRoutes are yours (rules TK8, TK11, TK12, TK16, and TK28).
func (i *instance) UIRoutes() module.Routes {
	return module.Routes{
		"POST /stub":   i.plant,
		"POST /status": i.setStatus,
		"POST /queue":  i.changeQueue,
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

// View is the pipeline, for the UI (rule TK27).
type View struct {
	Stubs         []TaskView  `json:"stubs"`
	Drafts        []TaskView  `json:"drafts"`
	Queues        []QueueView `json:"queues"`
	Problems      []Problem   `json:"problems"`
	Orchestration string      `json:"orchestration"`
	Partitions    []string    `json:"partitions"`
}

func (i *instance) View() any {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.view()
}

func (i *instance) view() View {
	return i.store.View(i.topo.Orchestration, i.topo.PartitionAgents())
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
		return []string{"Scopes in progress: " + strings.Join(lines, "; ") + ". The hikari-scope-work skill says how to carry them out and validate them."}
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
	Stubs  []TaskView  `json:"stubs,omitempty"`
	Drafts []TaskView  `json:"drafts,omitempty"`
	Queues []QueueView `json:"queues,omitempty"`
	// Scopes are a partition agent's scopes.
	Scopes []TaskView `json:"scopes,omitempty"`
}

func (i *instance) listFor(caller string) List {
	if caller == i.topo.Orchestration {
		v := i.view()
		return List{Stubs: v.Stubs, Drafts: v.Drafts, Queues: v.Queues}
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
