package tasks

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/nathanaday/hikari/internal/forge"
)

// QueuesDir is the folder of the queues, inside the module's directory.
const QueuesDir = "queues"

// Store holds the documents of the module's directory: the tasks in it,
// and the queues in queues/. Its paths are relative to that directory.
type Store struct {
	dir    string
	Tasks  map[string]*Task
	Queues map[string]*Queue
	// Problems maps the path of each document that hikari leaves alone to
	// the reason (rule TK6).
	Problems map[string]string
	// texts holds each document's text as hikari last read or wrote it.
	texts map[string]string
	// queueOf maps each scope to its queue, from the queue documents.
	queueOf map[string]string
	check   Check
	now     func() time.Time
	// lines are the log lines written since TakeLines, for the feed, and
	// hooks the moments since TakeHooks, for the trackers.
	lines []Line
	hooks []Hook
	asks  []Ask
}

// Check is what the store checks a document against (rule TK4).
type Check struct {
	// Partitions are the partition agents of the topology.
	Partitions []string
	// Orchestration is the orchestration agent, or "".
	Orchestration string
	// Feature reports whether id is a feature of partition's feature map,
	// or of the orchestration feature map when partition is empty. A nil
	// Feature checks no feature.
	Feature func(partition, id string) bool
	// Health is the health of a feature, as Feature names it.
	Health func(partition, id string) (string, bool)
	// Reviewed reports whether a partition's parts need a review before
	// done: its agent's review is required. A nil Reviewed requires none.
	Reviewed func(partition string) bool
}

// Open reads the store of dir, which it makes when missing.
func Open(dir string, check Check, now func() time.Time) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, QueuesDir), 0o755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, check: check, now: now}
	_, err := s.Reload()
	return s, err
}

func taskPath(id string) string  { return id + ".md" }
func queuePath(id string) string { return path.Join(QueuesDir, id+".md") }

// files lists the module's documents, as paths relative to dir.
func (s *Store) files() ([]string, error) {
	var out []string
	for _, sub := range []string{".", QueuesDir} {
		entries, err := os.ReadDir(filepath.Join(s.dir, sub))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() || strings.HasPrefix(e.Name(), ".") || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			out = append(out, path.Join(sub, e.Name()))
		}
	}
	return out, nil
}

// Reload reads every document again, and returns the paths whose text
// differs from what hikari last read or wrote: your edits (rule TK7).
func (s *Store) Reload() ([]string, error) {
	files, err := s.files()
	if err != nil {
		return nil, err
	}
	texts := map[string]string{}
	var changed []string
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(s.dir, filepath.FromSlash(f)))
		if err != nil {
			return nil, err
		}
		texts[f] = string(data)
		if old, ok := s.texts[f]; !ok || old != string(data) {
			changed = append(changed, f)
		}
	}
	for f := range s.texts {
		if _, ok := texts[f]; !ok {
			changed = append(changed, f)
		}
	}
	slices.Sort(changed)
	if s.texts != nil && len(changed) == 0 {
		return nil, nil
	}
	s.texts = texts
	s.parse()
	return changed, nil
}

// parse reads the texts into documents, and finds the problems.
func (s *Store) parse() {
	s.Tasks, s.Queues, s.Problems = map[string]*Task{}, map[string]*Queue{}, map[string]string{}
	for f, text := range s.texts {
		base := strings.TrimSuffix(path.Base(f), ".md")
		if path.Dir(f) == QueuesDir {
			q, err := ParseQueue(text)
			switch {
			case err != nil:
				s.Problems[f] = err.Error()
			case !validID.MatchString(base):
				s.Problems[f] = fmt.Sprintf("the file's name is not an id; rename it to %s.md", Slug(base))
			case q.ID != "" && q.ID != base:
				s.Problems[f] = fmt.Sprintf("its id %q differs from the file's name %q", q.ID, base)
			default:
				q.ID = base
				s.Queues[base] = q
			}
			continue
		}
		t, err := ParseTask(text)
		switch {
		case err != nil:
			s.Problems[f] = err.Error()
		case !validID.MatchString(base):
			s.Problems[f] = fmt.Sprintf("the file's name is not an id; rename it to %s.md", Slug(base))
		case t.ID != "" && t.ID != base:
			s.Problems[f] = fmt.Sprintf("its id %q differs from the file's name %q", t.ID, base)
		case t.Stage != StageStub && t.Stage != StageScope:
			s.Problems[f] = fmt.Sprintf("its stage must be %s or %s, not %q", StageStub, StageScope, t.Stage)
		default:
			t.ID = base
			if t.Name == "" {
				t.Name = base
			}
			s.Tasks[base] = t
		}
	}
	s.validate()
}

// validate finds the problems of rule TK4 and TK5, and the queue of each
// scope.
func (s *Store) validate() {
	bad := func(f, format string, args ...any) {
		if _, ok := s.Problems[f]; !ok {
			s.Problems[f] = fmt.Sprintf(format, args...)
		}
	}
	for _, id := range sortedKeys(s.Tasks) {
		t := s.Tasks[id]
		f := taskPath(id)
		for _, p := range t.Partitions {
			if !slices.Contains(s.check.Partitions, p) {
				bad(f, "%q in partitions is not a partition agent; the partition agents are %s", p, strings.Join(s.check.Partitions, ", "))
			}
		}
		for _, feat := range t.Features {
			if !s.featureOf(t, feat) {
				bad(f, "[[%s]] in features is no feature of its partitions or of the orchestration feature map", feat)
			}
		}
		for _, n := range t.Needs {
			if o, ok := s.Tasks[n]; !ok || o.Stage != StageScope || n == id {
				bad(f, "[[%s]] in needs is not another scope", n)
			}
		}
		if t.Stage == StageScope && len(t.Partitions) == 0 {
			bad(f, "a scope names at least one partition")
		}
	}
	for f := range s.Problems {
		if path.Dir(f) == "." {
			delete(s.Tasks, strings.TrimSuffix(f, ".md"))
		}
	}
	s.queueOf = map[string]string{}
	for _, qid := range sortedKeys(s.Queues) {
		q := s.Queues[qid]
		f := queuePath(qid)
		if q.State != Running && q.State != Paused && q.State != "" {
			bad(f, "its state must be %s or %s, not %q", Running, Paused, q.State)
		}
		for _, id := range q.Scopes {
			_, ok := s.Tasks[id]
			switch {
			case !ok:
				bad(f, "[[%s]] is not a task without problems", id)
			case s.queueOf[id] != "":
				bad(f, "[[%s]] is in the queue %s already", id, s.queueOf[id])
			}
			if _, problem := s.Problems[f]; !problem {
				s.queueOf[id] = qid
			}
		}
		if _, problem := s.Problems[f]; problem {
			for id, in := range s.queueOf {
				if in == qid {
					delete(s.queueOf, id)
				}
			}
			delete(s.Queues, qid)
		}
	}
}

// featureOf reports whether id is a feature of t's partitions or of the
// orchestration feature map.
func (s *Store) featureOf(t *Task, id string) bool {
	if s.check.Feature == nil {
		return true
	}
	if s.check.Feature("", id) {
		return true
	}
	return slices.ContainsFunc(t.Partitions, func(p string) bool { return s.check.Feature(p, id) })
}

// QueueOf returns the queue of a scope, or "".
func (s *Store) QueueOf(id string) string { return s.queueOf[id] }

// write writes one document, and returns its path.
func (s *Store) write(f, text string) (string, error) {
	p := filepath.Join(s.dir, filepath.FromSlash(f))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	tmp := filepath.Join(filepath.Dir(p), "."+filepath.Base(p)+".new")
	if err := os.WriteFile(tmp, []byte(text), 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, p); err != nil {
		return "", err
	}
	s.texts[f] = text
	return f, nil
}

func (s *Store) remove(f string) (string, error) {
	if err := os.Remove(filepath.Join(s.dir, filepath.FromSlash(f))); err != nil {
		return "", err
	}
	delete(s.texts, f)
	return f, nil
}

// saveTask writes t, with its updated time.
func (s *Store) saveTask(t *Task) (string, error) {
	t.Updated = s.now().UTC().Truncate(time.Second)
	return s.write(taskPath(t.ID), t.Format())
}

func (s *Store) saveQueue(q *Queue) (string, error) {
	q.Updated = s.now().UTC().Truncate(time.Second)
	return s.write(queuePath(q.ID), q.Format())
}

// Fill completes each task that you wrote by hand (rule TK2): its id, its
// kind, its version, and its created time. It returns the paths it wrote.
func (s *Store) Fill() ([]string, error) {
	var out []string
	for _, id := range sortedKeys(s.Tasks) {
		t := s.Tasks[id]
		text := s.texts[taskPath(id)]
		parsed, _ := ParseTask(text)
		if parsed.ID != "" && parsed.Kind != "" && parsed.Version != 0 && !parsed.Created.IsZero() {
			continue
		}
		t.Kind, t.Version = KindTask, Version
		if t.Created.IsZero() {
			t.Created = s.now().UTC().Truncate(time.Second)
		}
		f, err := s.saveTask(t)
		if err != nil {
			return out, err
		}
		out = append(out, f)
	}
	return out, nil
}

// unique returns base, or base with the first number that no document of
// kind uses.
func unique(base string, taken func(string) bool) string {
	if base == "" {
		base = "task"
	}
	id := base
	for n := 2; taken(id); n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	return id
}

func (s *Store) taken(id string) bool {
	_, task := s.Tasks[id]
	_, file := s.texts[taskPath(id)]
	return task || file
}

// stubName takes a stub's name from its words: the first line, cut at a
// word near 60 characters. When the name does not hold all the words and
// the body is empty, the words become the body, so nothing is lost.
func stubName(words, body string) (string, string) {
	words = strings.TrimSpace(words)
	first, _, _ := strings.Cut(words, "\n")
	name := strings.TrimRight(strings.TrimSpace(first), ".")
	if len(name) > 60 {
		cut := strings.LastIndex(name[:60], " ")
		if cut < 20 {
			cut = 60
		}
		name = strings.TrimRight(name[:cut], ",;:") + "…"
	}
	if strings.TrimSpace(body) == "" && (strings.Contains(words, "\n") || strings.HasSuffix(name, "…")) {
		body = words
	}
	return name, body
}

// PlantStub writes a stub (rule TK8). The name's first line names it, cut
// near 60 characters; see stubName.
func (s *Store) PlantStub(name, body, author string) (*Task, []string, error) {
	name, body = stubName(name, body)
	if name == "" {
		return nil, nil, errors.New("a stub needs a name")
	}
	now := s.now().UTC().Truncate(time.Second)
	t := &Task{Version: Version, ID: unique(Slug(name), s.taken), Kind: KindTask, Stage: StageStub, Name: name, Author: author, Created: now, Body: strings.TrimSpace(body) + "\n"}
	if strings.TrimSpace(body) == "" {
		t.Body = ""
	}
	f, err := s.saveTask(t)
	if err != nil {
		return nil, nil, err
	}
	s.Tasks[t.ID] = t
	return t, []string{f}, nil
}

// ScopeInput is what scope_task writes (rule TK10).
type ScopeInput struct {
	ID         string            `json:"id"`
	Partitions []string          `json:"partitions"`
	Features   []string          `json:"features,omitempty"`
	Needs      []string          `json:"needs,omitempty"`
	Goal       string            `json:"goal"`
	Parts      map[string]string `json:"parts"`
	DoneWhen   string            `json:"done_when"`
	Answers    string            `json:"answers,omitempty"`
}

// Scope turns a stub or a draft scope into a scope (rules TK3 and TK10).
func (s *Store) Scope(in ScopeInput) (*Task, []string, error) {
	t, err := s.task(in.ID)
	if err != nil {
		return nil, nil, err
	}
	// A scope in a queue is the user's; a task that a queue started before
	// it was scoped is what the orchestration agent scopes now (rule IS4).
	if q := s.queueOf[t.ID]; q != "" && t.Stage == StageScope {
		return nil, nil, fmt.Errorf("[[%s]] is in the queue %s, so only the user changes it; propose the change to the user", t.ID, q)
	}
	if len(in.Partitions) == 0 {
		return nil, nil, errors.New("a scope names at least one partition")
	}
	for _, p := range in.Partitions {
		if !slices.Contains(s.check.Partitions, p) {
			return nil, nil, fmt.Errorf("%q is not a partition agent; the partition agents are %s", p, strings.Join(s.check.Partitions, ", "))
		}
	}
	for p := range in.Parts {
		if !slices.Contains(in.Partitions, p) {
			return nil, nil, fmt.Errorf("the parts name %q, which is not in partitions", p)
		}
	}
	for _, p := range in.Partitions {
		if strings.TrimSpace(in.Parts[p]) == "" {
			return nil, nil, fmt.Errorf("the part of %s is empty; say what %s changes, in terms of its features and interfaces", p, p)
		}
	}
	if strings.TrimSpace(in.Goal) == "" || strings.TrimSpace(in.DoneWhen) == "" {
		return nil, nil, errors.New("a scope needs its goal and what shows that it is done")
	}
	next := *t
	next.Stage, next.Partitions, next.Features, next.Needs = StageScope, in.Partitions, trimLinks(in.Features), trimLinks(in.Needs)
	for _, f := range next.Features {
		if !s.featureOf(&next, f) {
			return nil, nil, fmt.Errorf("[[%s]] is no feature of %s or of the orchestration feature map", f, strings.Join(in.Partitions, ", "))
		}
	}
	for _, n := range next.Needs {
		if o, ok := s.Tasks[n]; !ok || o.Stage != StageScope || n == t.ID {
			return nil, nil, fmt.Errorf("[[%s]] in needs is not another scope", n)
		}
	}
	request := strings.TrimSpace(t.Body)
	if t.Stage == StageScope {
		request, _ = Section(t.Body, SectionRequest)
	}
	log, _ := Section(t.Body, SectionLog)
	next.Body = scopeBody(request, in.Goal, in.Parts, in.Partitions, in.DoneWhen, in.Answers, log)
	if active(next.Status) {
		// The queue started it before it was scoped: its parts begin.
		next.Parts = nil
		for _, p := range in.Partitions {
			next.SetPart(p, Queued)
		}
		s.log(&next, s.check.Orchestration+" scoped it for "+strings.Join(in.Partitions, ", "))
	}
	f, err := s.saveTask(&next)
	if err != nil {
		return nil, nil, err
	}
	s.Tasks[t.ID] = &next
	if s.queueOf[t.ID] != "" {
		s.hook(HookScoped, &next)
		if strings.TrimSpace(in.Answers) != "" {
			s.ask(&next, fmt.Sprintf("The orchestration agent scoped [[%s]] with assumptions, which its comment on the issue lists. Proceed?", t.ID),
				"Proceed as scoped", "Change the scope: I will say what to change")
		}
	}
	return &next, []string{f}, nil
}

func trimLinks(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if m := link.FindStringSubmatch(id); m != nil {
			id = strings.TrimSpace(m[1])
		}
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

func (s *Store) task(id string) (*Task, error) {
	id = strings.TrimSpace(strings.Trim(strings.TrimSpace(id), "[]"))
	if t, ok := s.Tasks[id]; ok {
		return t, nil
	}
	if p, ok := s.Problems[taskPath(id)]; ok {
		return nil, fmt.Errorf("[[%s]] has a problem: %s", id, p)
	}
	return nil, fmt.Errorf("%w: no task [[%s]]", errNotFound, id)
}

func (s *Store) queue(id string) (*Queue, error) {
	if q, ok := s.Queues[id]; ok {
		return q, nil
	}
	if p, ok := s.Problems[queuePath(id)]; ok {
		return nil, fmt.Errorf("the queue %s has a problem: %s", id, p)
	}
	return nil, fmt.Errorf("%w: no queue %s", errNotFound, id)
}

// QueueOp is one change of the queues (rule TK11).
type QueueOp struct {
	// Op is create, rename, remove, add, move, or out.
	Op    string `json:"op"`
	Queue string `json:"queue,omitempty"`
	Name  string `json:"name,omitempty"`
	Scope string `json:"scope,omitempty"`
	// Issue names the task by its issue, <repo>#<number>, in place of
	// Scope; the task is made when the issue has none yet.
	Issue string `json:"issue,omitempty"`
	// Position is the scope's place in the queue, from 1; 0 is the end.
	Position int `json:"position,omitempty"`
}

// movable reports whether a scope may move or leave its queue.
func movable(t *Task) bool { return t.Status == "" || t.Status == Queued || t.Status == Waiting }

// ChangeQueue applies op, and returns the paths it wrote.
func (s *Store) ChangeQueue(op QueueOp) ([]string, error) {
	switch op.Op {
	case "create":
		name := strings.TrimSpace(op.Name)
		if name == "" {
			return nil, errors.New("a queue needs a name")
		}
		id := unique(Slug(name), func(id string) bool { _, ok := s.texts[queuePath(id)]; return ok })
		now := s.now().UTC().Truncate(time.Second)
		q := &Queue{Version: Version, ID: id, Kind: KindQueue, Name: name, State: Paused, Created: now}
		f, err := s.saveQueue(q)
		if err != nil {
			return nil, err
		}
		s.Queues[id] = q
		return []string{f}, nil
	case "rename":
		q, err := s.queue(op.Queue)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(op.Name) == "" {
			return nil, errors.New("a queue needs a name")
		}
		q.Name = strings.TrimSpace(op.Name)
		f, err := s.saveQueue(q)
		return []string{f}, err
	case "remove":
		q, err := s.queue(op.Queue)
		if err != nil {
			return nil, err
		}
		if len(q.Scopes) > 0 {
			return nil, fmt.Errorf("the queue %s still holds %d scopes; take them out or move them first", q.ID, len(q.Scopes))
		}
		f, err := s.remove(queuePath(q.ID))
		delete(s.Queues, q.ID)
		return []string{f}, err
	case "add", "move":
		return s.place(op)
	case "out":
		t, err := s.task(op.Scope)
		if err != nil {
			return nil, err
		}
		from := s.queueOf[t.ID]
		if from == "" {
			return nil, fmt.Errorf("[[%s]] is in no queue", t.ID)
		}
		if !movable(t) {
			return nil, fmt.Errorf("[[%s]] is %s, so it stays in its queue", t.ID, t.Status)
		}
		q := s.Queues[from]
		q.Scopes = slices.DeleteFunc(q.Scopes, func(id string) bool { return id == t.ID })
		delete(s.queueOf, t.ID)
		t.Queue, t.Status, t.Parts, t.Wait = "", "", nil, ""
		s.hook(HookOut, t)
		fq, err := s.saveQueue(q)
		if err != nil {
			return nil, err
		}
		ft, err := s.saveTask(t)
		return []string{fq, ft}, err
	}
	return nil, fmt.Errorf("op must be create, rename, remove, add, move, or out, not %q", op.Op)
}

// place adds a scope to a queue, or moves it within its queue or to
// another, at op.Position.
func (s *Store) place(op QueueOp) ([]string, error) {
	t, err := s.task(op.Scope)
	if err != nil {
		return nil, err
	}
	to, err := s.queue(op.Queue)
	if err != nil {
		return nil, err
	}
	from := s.queueOf[t.ID]
	if op.Op == "add" && from != "" && from != to.ID {
		return nil, fmt.Errorf("[[%s]] is in the queue %s; move it instead", t.ID, from)
	}
	if from != "" && !movable(t) {
		return nil, fmt.Errorf("[[%s]] is %s, so it stays where it is", t.ID, t.Status)
	}
	var paths []string
	if from != "" && from != to.ID {
		q := s.Queues[from]
		q.Scopes = slices.DeleteFunc(q.Scopes, func(id string) bool { return id == t.ID })
		f, err := s.saveQueue(q)
		if err != nil {
			return nil, err
		}
		paths = append(paths, f)
	}
	to.Scopes = slices.DeleteFunc(to.Scopes, func(id string) bool { return id == t.ID })
	pos := op.Position - 1
	if pos < 0 || pos > len(to.Scopes) {
		pos = len(to.Scopes)
	}
	// A scope never goes before one that has started.
	for pos < len(to.Scopes) && !movable(s.Tasks[to.Scopes[pos]]) {
		pos++
	}
	to.Scopes = slices.Insert(to.Scopes, pos, t.ID)
	s.queueOf[t.ID] = to.ID
	f, err := s.saveQueue(to)
	if err != nil {
		return nil, err
	}
	paths = append(paths, f)
	t.Queue = to.ID
	if t.Status == "" {
		t.Status = Queued
		s.hook(HookQueued, t)
	}
	ft, err := s.saveTask(t)
	return append(paths, ft), err
}

var errNotFound = errors.New("not found")

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// SyncQueues writes each task's queue, and its status on entering or
// leaving one, from the queue documents (rule TK5). It returns the paths
// it wrote.
func (s *Store) SyncQueues() ([]string, error) {
	var out []string
	for _, id := range sortedKeys(s.Tasks) {
		t := s.Tasks[id]
		want := s.queueOf[id]
		changed := false
		if t.Queue != want {
			t.Queue, changed = want, true
		}
		switch {
		case want != "" && t.Status == "":
			t.Status, changed = Queued, true
		case want == "" && t.Status != "" && movable(t):
			t.Status, t.Parts, t.Wait, changed = "", nil, "", true
		}
		if !changed {
			continue
		}
		f, err := s.saveTask(t)
		if err != nil {
			return out, err
		}
		out = append(out, f)
	}
	return out, nil
}

// TaskView is t, for the UI and the tools.
func (s *Store) TaskView(t *Task) TaskView {
	text := s.texts[taskPath(t.ID)]
	v := TaskView{ID: t.ID, Name: t.Name, Stage: t.Stage, Author: t.Author, Status: t.Status, Block: t.Block, Wait: t.Wait, Queue: s.QueueOf(t.ID), Issue: t.Issue,
		Partitions: nonNil(t.Partitions), Features: nonNil(t.Features), Needs: nonNil(t.Needs), Parts: nonNil(t.Parts), Created: t.Created, Updated: t.Updated,
		File: path.Join(Dir, taskPath(t.ID)), Hash: fmt.Sprintf("%x-%d", len(text), t.Updated.Unix())}
	if t.Stage == StageScope {
		copy := *t
		copy.Partitions = slices.Clone(t.Partitions)
		v.Scorecard = scorecard(&copy)
	}
	return v
}

// View is the whole pipeline, for the UI and hikari task list (rule TK27):
// every task by when it was made, and the queues by id. The instance adds
// the issues of the trackers.
func (s *Store) View(orchestration string, partitions []string) View {
	v := View{Tasks: []TaskView{}, Issues: []IssueView{}, Trackers: []TrackerView{}, Queues: []QueueView{}, Problems: []Problem{}, Orchestration: orchestration, Partitions: nonNil(partitions)}
	ids := sortedKeys(s.Tasks)
	slices.SortStableFunc(ids, func(a, b string) int { return s.Tasks[a].Created.Compare(s.Tasks[b].Created) })
	for _, id := range ids {
		v.Tasks = append(v.Tasks, s.TaskView(s.Tasks[id]))
	}
	for _, qid := range sortedKeys(s.Queues) {
		q := s.Queues[qid]
		qv := QueueView{ID: q.ID, Name: q.Name, State: q.State, Scopes: []TaskView{}, File: path.Join(Dir, queuePath(q.ID))}
		if qv.State == "" {
			qv.State = Paused
		}
		for _, id := range q.Scopes {
			if t := s.Tasks[id]; t != nil {
				qv.Scopes = append(qv.Scopes, s.TaskView(t))
			}
		}
		v.Queues = append(v.Queues, qv)
	}
	for _, f := range sortedKeys(s.Problems) {
		v.Problems = append(v.Problems, Problem{File: path.Join(Dir, f), Reason: s.Problems[f]})
	}
	return v
}

// TaskForIssue returns the task that stands for a tracker issue, or nil.
func (s *Store) TaskForIssue(ref string) *Task {
	for _, id := range sortedKeys(s.Tasks) {
		if s.Tasks[id].Issue == ref {
			return s.Tasks[id]
		}
	}
	return nil
}

// EnsureIssueTask returns the task of a tracker issue, which it makes when
// missing (rule IS1): the id <repo>-<number>, the issue's title as its
// name, and its body as the request.
func (s *Store) EnsureIssueTask(repo string, is forge.Issue) (*Task, []string, error) {
	ref := IssueRef(repo, is.Number)
	if t := s.TaskForIssue(ref); t != nil {
		return t, nil, nil
	}
	if p, ok := s.Problems[taskPath(fmt.Sprintf("%s-%d", repo, is.Number))]; ok {
		return nil, nil, fmt.Errorf("the task of %s has a problem: %s", ref, p)
	}
	name := strings.TrimSpace(is.Title)
	if name == "" {
		name = ref
	}
	author := is.Author
	if author == "" {
		author = User
	}
	now := s.now().UTC().Truncate(time.Second)
	t := &Task{Version: Version, ID: unique(fmt.Sprintf("%s-%d", repo, is.Number), s.taken), Kind: KindTask, Stage: StageStub, Name: name, Author: author, Created: now, Issue: ref, Body: strings.TrimSpace(is.Body)}
	if t.Body != "" {
		t.Body += "\n"
	}
	f, err := s.saveTask(t)
	if err != nil {
		return nil, nil, err
	}
	s.Tasks[t.ID] = t
	return t, []string{f}, nil
}

// UpdateLocal changes the title and the body of a local issue: a task that
// stands for no tracker issue and is not scoped yet.
func (s *Store) UpdateLocal(id string, name, body *string) (*Task, []string, error) {
	t, err := s.task(id)
	if err != nil {
		return nil, nil, err
	}
	if t.Issue != "" {
		return nil, nil, fmt.Errorf("[[%s]] stands for the issue %s; change it on its tracker", t.ID, t.Issue)
	}
	if t.Stage != StageStub {
		return nil, nil, fmt.Errorf("[[%s]] is scoped; its request is what the orchestration agent scoped", t.ID)
	}
	if name != nil {
		if strings.TrimSpace(*name) == "" {
			return nil, nil, errors.New("an issue needs a title")
		}
		t.Name = strings.TrimSpace(*name)
	}
	if body != nil {
		t.Body = strings.TrimSpace(*body)
		if t.Body != "" {
			t.Body += "\n"
		}
	}
	f, err := s.saveTask(t)
	return t, []string{f}, err
}

// RemoveLocal removes a local issue that is in no queue.
func (s *Store) RemoveLocal(id string) ([]string, error) {
	t, err := s.task(id)
	if err != nil {
		return nil, err
	}
	if t.Issue != "" {
		return nil, fmt.Errorf("[[%s]] stands for the issue %s; close it on its tracker", t.ID, t.Issue)
	}
	if q := s.queueOf[t.ID]; q != "" {
		return nil, fmt.Errorf("[[%s]] is in the queue %s; take it out first", t.ID, q)
	}
	f, err := s.remove(taskPath(t.ID))
	delete(s.Tasks, t.ID)
	return []string{f}, err
}
