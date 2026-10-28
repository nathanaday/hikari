package tasks

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Change is what one change of the pipeline did beyond its own files: the
// writes to commit, each with its author, and the messages hikari sends.
type Change struct {
	Writes []Write
	Notes  []Note
}

// Write is paths that one author changed.
type Write struct {
	Author string
	Paths  []string
}

// Note is a message from hikari to an agent (rule TK19).
type Note struct {
	To, Body string
}

func (c *Change) wrote(author string, paths ...string) {
	if len(paths) == 0 {
		return
	}
	for i := range c.Writes {
		if c.Writes[i].Author == author {
			for _, p := range paths {
				if !slices.Contains(c.Writes[i].Paths, p) {
					c.Writes[i].Paths = append(c.Writes[i].Paths, p)
				}
			}
			return
		}
	}
	c.Writes = append(c.Writes, Write{Author: author, Paths: slices.Clone(paths)})
}

func (c *Change) note(to, body string) {
	if to != "" {
		c.Notes = append(c.Notes, Note{To: to, Body: body})
	}
}

// Hikari is hikari's own name as a writer and a sender.
const Hikari = "hikari"

// User is the user's name as a writer.
const User = "user"

func (s *Store) logLine(who, subject, from, to, note string) string {
	line := fmt.Sprintf("%s %s set %s to %s", s.now().UTC().Format(timeLayout), who, subject, to)
	if from != "" {
		line += fmt.Sprintf(" (was %s)", from)
	}
	if note = strings.TrimSpace(note); note != "" {
		line += ": " + strings.ReplaceAll(note, "\n", " ")
	}
	return line
}

// setScope sets t's status, with a line in its log (rule TK18).
func (s *Store) setScope(t *Task, who, status, note string) {
	if t.Status == status {
		return
	}
	t.Body = appendLog(t.Body, s.logLine(who, "the scope", t.Status, status, note))
	t.Status = status
}

func (s *Store) setPart(t *Task, who, partition, status, note string) {
	from := t.PartStatus(partition)
	if from == status {
		return
	}
	t.Body = appendLog(t.Body, s.logLine(who, "the part of "+partition, from, status, note))
	t.SetPart(partition, status)
}

// fromParts is the status of a started scope that its parts give (rule
// TK15).
func fromParts(t *Task) string {
	all, testing := true, false
	for _, p := range t.Partitions {
		switch t.PartStatus(p) {
		case Blocked:
			return Blocked
		case Testing:
			testing = true
		case Done:
		default:
			all = false
		}
	}
	switch {
	case len(t.Partitions) > 0 && all && !testing:
		return Validating
	case all:
		return Testing
	}
	return Started
}

func anyPart(t *Task, status string) bool {
	return slices.ContainsFunc(t.Parts, func(p Part) bool { return p.Status == status })
}

// follow sets a started scope's status from its parts, and tells the
// orchestration agent when every part is done. A block that the scope
// itself has, with no blocked part, stays until it is cleared (rule TK16).
func (s *Store) follow(t *Task, c *Change) {
	next := fromParts(t)
	if t.Block != "" {
		return
	}
	if next == t.Status {
		return
	}
	s.setScope(t, Hikari, next, "")
	if next == Validating {
		c.note(s.check.Orchestration, fmt.Sprintf("Every part of [[%s]] is done. Validate it: follow the Validate a scope procedure of the hikari-scope-work skill.", t.ID))
	}
}

// Schedule starts the next scope of each running queue that can start
// (rules TK13 and TK17).
func (s *Store) Schedule(c *Change) error {
	for _, qid := range sortedKeys(s.Queues) {
		q := s.Queues[qid]
		if q.State != Running {
			continue
		}
		var t *Task
		for _, id := range q.Scopes {
			if task := s.Tasks[id]; task != nil && task.Status != Done {
				t = task
				break
			}
		}
		if t == nil || active(t.Status) {
			continue
		}
		if reason := s.blockers(t); reason != "" {
			if t.Status != Waiting || t.Wait != reason {
				s.setScope(t, Hikari, Waiting, reason)
				t.Wait = reason
				f, err := s.saveTask(t)
				if err != nil {
					return err
				}
				c.wrote(Hikari, f)
			}
			continue
		}
		s.setScope(t, Hikari, Started, "the queue "+q.ID+" started it")
		t.Wait = ""

		for _, p := range t.Partitions {
			t.SetPart(p, Queued)
		}
		f, err := s.saveTask(t)
		if err != nil {
			return err
		}
		c.wrote(Hikari, f)
		if s.check.Orchestration != "" {
			c.note(s.check.Orchestration, fmt.Sprintf("The queue %s started [[%s]]. Follow the Carry out a scope procedure of the hikari-scope-work skill.", q.ID, t.ID))
		} else {
			for _, p := range t.Partitions {
				c.note(p, fmt.Sprintf("The queue %s started [[%s]], with a part for you. Follow the hikari-task-work skill.", q.ID, t.ID))
			}
		}
	}
	return nil
}

// blockers says why t cannot start yet, or "" when it can (D4 and D5).
func (s *Store) blockers(t *Task) string {
	var why []string
	for _, n := range t.Needs {
		if o := s.Tasks[n]; o == nil || o.Status != Done {
			why = append(why, fmt.Sprintf("it needs [[%s]] done", n))
		}
	}
	for _, p := range t.Partitions {
		for _, id := range sortedKeys(s.Tasks) {
			o := s.Tasks[id]
			if id != t.ID && active(o.Status) && slices.Contains(o.Partitions, p) {
				why = append(why, fmt.Sprintf("%s is busy with [[%s]]", p, id))
				break
			}
		}
	}
	return strings.Join(why, "; ")
}

// SetQueueState starts or pauses a queue (rule TK12).
func (s *Store) SetQueueState(id, state string, c *Change) error {
	if state != Running && state != Paused {
		return fmt.Errorf("a queue's state is %s or %s, not %q", Running, Paused, state)
	}
	q, err := s.queue(id)
	if err != nil {
		return err
	}
	if q.State == state {
		return nil
	}
	q.State = state
	f, err := s.saveQueue(q)
	if err != nil {
		return err
	}
	c.wrote(User, f)
	return s.Schedule(c)
}

// Grant says whether the server granted done, and why not.
type Grant struct {
	Status string `json:"status"`
	// Refused names each feature whose health is not passing.
	Refused []string `json:"refused,omitempty"`
}

// notPassing lists each feature of t, in partition, whose health is
// not passing (rule TK14), or each of the orchestration feature map when
// partition is empty (rule TK16).
func (s *Store) notPassing(t *Task, partition string) []string {
	var out []string
	for _, f := range t.Features {
		if s.check.Feature != nil && !s.check.Feature(partition, f) {
			continue
		}
		h, ok := "", false
		if s.check.Health != nil {
			h, ok = s.check.Health(partition, f)
		}
		if !ok {
			h = "unknown"
		}
		if h != "passing" {
			out = append(out, fmt.Sprintf("[[%s]] is %s", f, h))
		}
	}
	return out
}

var errForbidden = errors.New("not allowed")

// SetStatus sets the status of a part, for a partition agent, or of the
// scope, for the orchestration agent or the user (rules TK14 to TK19).
// part names the part for the user; an agent's part is its own.
func (s *Store) SetStatus(caller, id, part, status, note string, c *Change) (Grant, error) {
	t, err := s.task(id)
	if err != nil {
		return Grant{}, err
	}
	if t.Stage != StageScope {
		return Grant{}, fmt.Errorf("[[%s]] is a stub, which has no status", t.ID)
	}
	isPartition := slices.Contains(s.check.Partitions, caller)
	switch {
	case isPartition:
		if !slices.Contains(t.Partitions, caller) {
			return Grant{}, fmt.Errorf("%w: [[%s]] has no part for %s", errForbidden, t.ID, caller)
		}
		part = caller
	case caller == s.check.Orchestration:
		part = ""
	case caller == User:
	default:
		return Grant{}, fmt.Errorf("%w: %s sets no status", errForbidden, caller)
	}
	if status == Blocked && strings.TrimSpace(note) == "" && caller != User {
		return Grant{}, errors.New("a block needs a note that says why")
	}
	var g Grant
	if part != "" {
		g, err = s.setPartStatus(t, caller, part, status, note, c)
	} else {
		g, err = s.setScopeStatus(t, caller, status, note, c)
	}
	if err != nil {
		return g, err
	}
	f, err := s.saveTask(t)
	if err != nil {
		return g, err
	}
	c.wrote(caller, f)
	if t.Status == Done {
		err = s.Schedule(c)
	}
	return g, err
}

func (s *Store) setPartStatus(t *Task, caller, part, status, note string, c *Change) (Grant, error) {
	if !slices.Contains(t.Partitions, part) {
		return Grant{}, fmt.Errorf("[[%s]] has no part for %s", t.ID, part)
	}
	allowed := []string{Started, Testing, Blocked, Done}
	if caller == User {
		allowed = append(allowed, Queued)
	}
	if !slices.Contains(allowed, status) {
		return Grant{}, fmt.Errorf("a part's status is one of %s, not %q", strings.Join(allowed, ", "), status)
	}
	if !active(t.Status) && caller != User {
		state := t.Status
		if state == "" {
			state = "a draft"
		}
		return Grant{}, fmt.Errorf("[[%s]] has not started; it is %s", t.ID, state)
	}
	g := Grant{Status: status}
	if status == Done && caller != User {
		if refused := s.notPassing(t, part); len(refused) > 0 {
			g = Grant{Status: Testing, Refused: refused}
			status = Testing
			note = "done refused: " + strings.Join(refused, ", ")
		}
	}
	from := t.PartStatus(part)
	s.setPart(t, caller, part, status, note)
	if active(t.Status) || t.Status == Done {
		s.follow(t, c)
	}
	orch := s.check.Orchestration
	switch {
	case from == status:
	case status == Done:
		c.note(orch, fmt.Sprintf("%s: the part of [[%s]] is done.", part, t.ID))
	case status == Blocked:
		c.note(orch, fmt.Sprintf("%s: the part of [[%s]] is blocked: %s", part, t.ID, strings.TrimSpace(note)))
	case from == Blocked:
		c.note(orch, fmt.Sprintf("%s: the part of [[%s]] is no longer blocked; it is %s.", part, t.ID, status))
	}
	return g, nil
}

func (s *Store) setScopeStatus(t *Task, caller, status, note string, c *Change) (Grant, error) {
	if caller == User {
		all := []string{Queued, Waiting, Started, Blocked, Testing, Validating, Done}
		if !slices.Contains(all, status) {
			return Grant{}, fmt.Errorf("a scope's status is one of %s, not %q", strings.Join(all, ", "), status)
		}
		if status == Blocked {
			t.Block = strings.TrimSpace(note)
			if t.Block == "" {
				t.Block = "blocked by the user"
			}
		} else {
			t.Block = ""
		}
		s.setScope(t, User, status, note)
		return Grant{Status: status}, nil
	}
	switch status {
	case Blocked:
		if !active(t.Status) {
			return Grant{}, fmt.Errorf("[[%s]] is %s, not started", t.ID, t.Status)
		}
		t.Block = strings.TrimSpace(note)
		s.setScope(t, caller, Blocked, note)
		return Grant{Status: Blocked}, nil
	case Started:
		if t.Block == "" {
			return Grant{}, fmt.Errorf("[[%s]] has no block of its own to clear; a blocked part is cleared by its partition agent", t.ID)
		}
		t.Block = ""
		next := fromParts(t)
		s.setScope(t, caller, next, note)
		return Grant{Status: next}, nil
	case Done:
		if t.Status != Validating {
			return Grant{}, fmt.Errorf("[[%s]] is %s; a scope is done only after every part is done and it is validating", t.ID, t.Status)
		}
		if refused := s.notPassing(t, ""); len(refused) > 0 {
			return Grant{Status: Validating, Refused: refused}, nil
		}
		s.setScope(t, caller, Done, note)
		return Grant{Status: Done}, nil
	}
	return Grant{}, fmt.Errorf("the orchestration agent sets a scope blocked, started (to clear a block), or done, not %q", status)
}
