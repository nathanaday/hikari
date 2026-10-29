// Package server routes messages between agents and records every message in
// one log. Every agent and the user are in one room: a post reaches everyone,
// and anyone can send a direct message to anyone else.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/assistant"
	"github.com/nathanaday/hikari/internal/editor"
	"github.com/nathanaday/hikari/internal/linked"
	"github.com/nathanaday/hikari/internal/partitions"
	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/internal/version"
	"github.com/nathanaday/hikari/internal/vocab"
)

const maxBodyBytes = 64 << 10

type Event struct {
	Type       string             `json:"type"`
	Message    *api.Message       `json:"message,omitempty"`
	Agent      string             `json:"agent,omitempty"`
	Status     *api.Status        `json:"status,omitempty"`
	Activity   *api.Activity      `json:"activity,omitempty"`
	Partition  *api.PartitionView `json:"partition,omitempty"`
	Vocabulary *vocab.Vocabulary  `json:"vocabulary,omitempty"`
	Wiki       *api.WikiView      `json:"wiki,omitempty"`
	Snapshot   *Snapshot          `json:"snapshot,omitempty"`
	// Revision is the topology file's new revision, in a topology event
	// (rule TA8 of the topology API spec).
	Revision string `json:"revision,omitempty"`
	// Module and View are a module's name and its new view, in a module
	// event (rule MO4 of the task pipeline spec).
	Module string `json:"module,omitempty"`
	View   any    `json:"view,omitempty"`
	// Entry is a new entry of the feed, in a feed event (rule FD2).
	Entry *api.Entry `json:"entry,omitempty"`
	// Assistant is a change of the hikari agent, in an assistant event
	// (rule HA9 of the hikari agent spec).
	Assistant *assistant.Event `json:"assistant,omitempty"`
}

type AgentView struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Role        string     `json:"role"`
	Status      api.Status `json:"status"`
}

type Snapshot struct {
	// Version is the version of the running hikari.
	Version string `json:"version"`
	// Project is the project's name (rule ID13 of the ids spec).
	Project string `json:"project"`
	// Mode, File, and Revision say what hikari up does and which file it
	// edits (rule UP9 of the topology API spec). StartFindings are why
	// the last start failed.
	Mode          string             `json:"mode"`
	File          string             `json:"file"`
	Revision      string             `json:"revision"`
	StartFindings []topology.Finding `json:"start_findings,omitempty"`
	Me            string             `json:"me"`
	Agents        []AgentView        `json:"agents"`
	// Participants lists everyone in the room: the orchestration agent first and
	// the user last.
	Participants []api.Peer          `json:"participants"`
	Messages     []api.Message       `json:"messages"`
	Partitions   []api.PartitionView `json:"partitions"`
	Vocabulary   vocab.Vocabulary    `json:"vocabulary"`
	// Repos and Deliverables let the UI draw the topology (rules N2, N3).
	Repos        []api.RepoInfo     `json:"repos"`
	Deliverables []deliverableEntry `json:"deliverables"`
	// Wikis holds the public wiki and every feature map.
	Wikis []api.WikiView `json:"wikis"`
	// Activity holds the recent steps of each agent.
	Activity map[string][]api.Activity `json:"activity"`
	// Modules holds the view of each module that the project turns on.
	Modules map[string]any `json:"modules"`
	// Feed holds the recent entries of the feed, and Cost the run's cost
	// so far (rules FD2 and FD6).
	Feed []api.Entry `json:"feed"`
	Cost float64     `json:"cost"`
	// Assistant is the hikari agent's view, or why the run has none
	// (rule HA9).
	Assistant assistant.View `json:"assistant"`
}

type delivery struct {
	msg  api.Message
	wake bool
}

type recipient struct {
	name string
	wake bool
}

type Server struct {
	topo    *topology.Topology
	journal io.Writer
	sys     *partitions.Manager

	mu     sync.Mutex
	tokens map[string]string
	nextID int64
	log    []api.Message
	inbox  map[string][]delivery
	signal map[string]chan struct{}
	status map[string]api.Status
	subs   map[chan Event]struct{}
	// activity keeps the recent steps of each agent; turnStart holds the
	// message ID when each agent's current turn began.
	activity  map[string][]api.Activity
	turnStart map[string]int64
	// modules are the host halves of the project's modules.
	modules []openModule
	// entries are the recent entries of the feed, nextEntry the id of the
	// last one, feedJournal where each is appended, and cost the run's
	// cost so far.
	entries     []api.Entry
	nextEntry   int64
	feedJournal io.Writer
	cost        float64
	// assistant is the hikari agent of the run, or nil with assistantOff
	// saying why.
	assistant    *assistant.Assistant
	assistantOff string
}

// maxActivity is the most entries kept per agent. Older turns are dropped
// whole.
const maxActivity = 400

// New returns a server for topo and its partitions. Each message is also
// written to journal as one line of JSON when journal is not nil.
func New(topo *topology.Topology, journal io.Writer, sys *partitions.Manager) *Server {
	s := &Server{
		topo:      topo,
		journal:   journal,
		sys:       sys,
		tokens:    map[string]string{},
		inbox:     map[string][]delivery{},
		signal:    map[string]chan struct{}{},
		status:    map[string]api.Status{},
		subs:      map[chan Event]struct{}{},
		activity:  map[string][]api.Activity{},
		turnStart: map[string]int64{},
	}
	for _, n := range topo.AgentNames() {
		s.signal[n] = make(chan struct{})
		s.status[n] = api.Status{State: api.StateStarting}
	}
	sys.OnChange(s.partitionChanged)
	s.openModules()
	return s
}

// SetAssistant gives the run its hikari agent, or with nil the reason it
// has none (rule HA9).
func (s *Server) SetAssistant(a *assistant.Assistant, off string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.assistant, s.assistantOff = a, off
}

// PublishAssistant sends a change of the hikari agent to the UI.
func (s *Server) PublishAssistant(e assistant.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publish(Event{Type: "assistant", Assistant: &e})
}

// Partitions returns the partitions manager.
func (s *Server) Partitions() *partitions.Manager { return s.sys }

func (s *Server) partitionChanged(c partitions.Change) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var names []string
	if c.Partition != "" {
		names = append(names, c.Partition)
	}
	for _, name := range append(names, c.Dependents...) {
		v := s.sys.PartitionView(name)
		s.publish(Event{Type: "partition", Partition: &v})
	}
	// A partition's health is the support of the public wiki's documents,
	// and the health of the orchestration feature map's: one computation
	// of the health serves both views.
	refs := []string{}
	if c.Wiki != "" {
		refs = append(refs, c.Wiki)
	}
	if c.Partition != "" {
		for _, ref := range []string{api.PublicWikiRef, api.OrchestrationWikiRef} {
			if ref != c.Wiki {
				refs = append(refs, ref)
			}
		}
	}
	for _, v := range s.sys.WikiViews(refs) {
		s.publish(Event{Type: "wiki", Wiki: &v})
	}
	if c.Vocabulary {
		v := s.sys.Vocabulary()
		s.publish(Event{Type: "vocabulary", Vocabulary: &v})
	}
}

func NewToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// IssueToken returns a new credential that identifies agent to the server.
func (s *Server) IssueToken(agent string) string {
	t := NewToken()
	s.mu.Lock()
	s.tokens[t] = agent
	s.mu.Unlock()
	return t
}

func (s *Server) agentForToken(token string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.tokens[token]
	return n, ok
}

var errForbidden = errors.New("not allowed")

// Send posts to the room, or sends a direct message when req.To is set.
func (s *Server) Send(from string, req api.SendRequest) (api.Message, error) {
	if strings.TrimSpace(req.Body) == "" {
		return api.Message{}, errors.New("message body is empty")
	}
	if len(req.Body) > maxBodyBytes {
		return api.Message{}, fmt.Errorf("message body exceeds %d bytes", maxBodyBytes)
	}
	if req.To != "" {
		return s.direct(from, req.To, req.Body)
	}
	return s.post(from, req.Body), nil
}

// post sends body to the room. It wakes the orchestration agent, every agent it
// mentions, and every agent when the orchestration agent sends it without
// mentions. Without an orchestration agent, a post without mentions wakes every
// agent.
func (s *Server) post(from, body string) api.Message {
	participants := s.topo.Participants()
	mentioned := map[string]bool{}
	for name := range mentions(body) {
		if slices.Contains(participants, name) {
			mentioned[name] = true
		}
	}
	orch := s.topo.Orchestration
	var recipients []recipient
	for _, p := range participants {
		if p == from || p == topology.User {
			continue
		}
		wake := mentioned[p] || (orch != "" && p == orch) ||
			(len(mentioned) == 0 && (orch == "" || from == orch))
		recipients = append(recipients, recipient{name: p, wake: wake})
	}
	return s.deliver(api.Message{From: from, Body: body}, recipients)
}

func (s *Server) direct(from, to, body string) (api.Message, error) {
	others := slices.DeleteFunc(s.topo.Participants(), func(p string) bool { return p == from })
	if !slices.Contains(others, to) {
		return api.Message{}, fmt.Errorf("%w: %q cannot send a direct message to %q; the room is %s",
			errForbidden, from, to, list(others))
	}
	var recipients []recipient
	if to != topology.User {
		recipients = append(recipients, recipient{name: to, wake: true})
	}
	return s.deliver(api.Message{From: from, To: to, Body: body}, recipients), nil
}

// deliver records m and queues it for each recipient. The user has no inbox:
// the UI reads the log.
func (s *Server) deliver(m api.Message, recipients []recipient) api.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	m.ID = s.nextID
	m.Time = time.Now().UTC()
	for _, r := range recipients {
		if r.wake {
			m.Woke = append(m.Woke, r.name)
		}
	}
	for _, r := range recipients {
		s.inbox[r.name] = append(s.inbox[r.name], delivery{msg: m, wake: r.wake})
		if r.wake {
			s.notify(r.name)
		}
	}
	s.log = append(s.log, m)
	if s.journal != nil {
		line, _ := json.Marshal(m)
		s.journal.Write(append(line, '\n'))
	}
	s.publish(Event{Type: "message", Message: &m})
	s.feed(api.Entry{Kind: api.EntryMessage, Who: m.From, To: m.To, Text: m.Body, Message: m.ID, Time: m.Time})
	return m
}

var mentionPattern = regexp.MustCompile(`(?:^|[^\w@])@([a-z][a-z0-9-]*)`)

func mentions(body string) map[string]bool {
	out := map[string]bool{}
	for _, m := range mentionPattern.FindAllStringSubmatch(body, -1) {
		out[m[1]] = true
	}
	return out
}

func (s *Server) notify(agent string) {
	close(s.signal[agent])
	s.signal[agent] = make(chan struct{})
}

// Take removes and returns every pending message of agent. With wait == 0 it
// returns at once. With wait > 0 it first waits until a pending message wakes
// the agent; if wait elapses or ctx ends first, it returns nothing and leaves
// the inbox as it was.
func (s *Server) Take(ctx context.Context, agent string, wait time.Duration) []api.Message {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		s.mu.Lock()
		pending := s.inbox[agent]
		if wait == 0 || slices.ContainsFunc(pending, func(d delivery) bool { return d.wake }) {
			s.inbox[agent] = nil
			s.mu.Unlock()
			msgs := make([]api.Message, len(pending))
			for i, d := range pending {
				msgs[i] = d.msg
			}
			return msgs
		}
		ch := s.signal[agent]
		s.mu.Unlock()
		select {
		case <-ch:
		case <-timer.C:
			return nil
		case <-ctx.Done():
			return nil
		}
	}
}

// Requeue puts msgs back at the front of the inbox of agent, for when a Take
// result could not be delivered. The messages wake the agent again.
func (s *Server) Requeue(agent string, msgs []api.Message) {
	if len(msgs) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	back := make([]delivery, len(msgs))
	for i, m := range msgs {
		back[i] = delivery{msg: m, wake: true}
	}
	s.inbox[agent] = append(back, s.inbox[agent]...)
	s.notify(agent)
}

func (s *Server) SetStatus(agent string, st api.Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	from := s.status[agent]
	s.status[agent] = st
	s.cost += st.Cost
	s.publish(Event{Type: "status", Agent: agent, Status: &st})
	if e := statusEntry(agent, from, st); e != nil {
		s.feed(*e)
	}
}

// AddActivity records a step of agent's turn. An entry with the ID of a kept
// entry replaces it. A closing text is marked unheard when the agent sent
// no message during the turn.
func (s *Server) AddActivity(agent string, a api.Activity) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a.Time.IsZero() {
		a.Time = time.Now().UTC()
	}
	switch a.Kind {
	case api.ActivityTurn:
		if !a.Done {
			s.turnStart[agent] = s.nextID
		}
	case api.ActivityDone:
		a.Unheard = a.Text != "" && !s.sentSince(agent, s.turnStart[agent])
	}
	list := s.activity[agent]
	if i := slices.IndexFunc(list, func(x api.Activity) bool { return x.ID == a.ID }); i >= 0 {
		list[i] = a
	} else {
		list = append(list, a)
	}
	for len(list) > maxActivity {
		turn := list[0].Turn
		n := 0
		for n < len(list) && list[n].Turn == turn {
			n++
		}
		list = slices.Clone(list[n:])
	}
	s.activity[agent] = list
	s.publish(Event{Type: "activity", Agent: agent, Activity: &a})
	if a.Kind == api.ActivityDone {
		var steps []api.Activity
		for _, x := range list {
			if x.Turn == a.Turn && x.Kind != api.ActivityDone {
				steps = append(steps, x)
			}
		}
		text := "turn done"
		if a.Error {
			text = "turn failed"
		}
		if summary := turnSummary(steps, a); summary != "" {
			text += " · " + summary
		}
		s.feed(api.Entry{Kind: api.EntryTurn, Who: agent, Text: text, Error: a.Error, Turn: a.Turn})
	}
}

func (s *Server) sentSince(agent string, id int64) bool {
	for i := len(s.log) - 1; i >= 0 && s.log[i].ID > id; i-- {
		if s.log[i].From == agent {
			return true
		}
	}
	return false
}

func (s *Server) Self(agent string) api.Self {
	a := s.topo.Agents[agent]
	self := api.Self{
		Name:          agent,
		Description:   a.Description,
		Instructions:  a.Instructions,
		Model:         a.Model,
		Participants:  s.participants(),
		Partition:     s.sys.Self(agent),
		Orchestration: agent == s.topo.Orchestration,
	}
	self.VisiblePartitions = s.topo.VisiblePartitions(agent)
	self.Releases = s.topo.ReadableReleases(agent)
	self.Deliverables = s.sys.Deliverables(agent)
	self.Skills = a.Grants.Skills
	self.Modules = s.topo.Modules
	for _, m := range self.VisiblePartitions {
		if g := s.topo.Agents[m].Grants.Releases; len(g) > 0 {
			if self.PartitionAgentGrants == nil {
				self.PartitionAgentGrants = map[string][]string{}
			}
			self.PartitionAgentGrants[m] = g
		}
		if d := s.sys.Deliverables(m); len(d) > 0 {
			if self.PartitionAgentDeliverables == nil {
				self.PartitionAgentDeliverables = map[string][]api.DeliverableInfo{}
			}
			self.PartitionAgentDeliverables[m] = d
		}
	}
	return self
}

// participants lists everyone in the room with a role.
func (s *Server) participants() []api.Peer {
	var out []api.Peer
	for _, p := range s.topo.Participants() {
		peer := api.Peer{Name: p, Role: s.topo.Role(p), Description: s.topo.Agents[p].Description}
		if p == topology.User {
			peer.Description = "The human operator."
		}
		out = append(out, peer)
	}
	if len(s.modules) > 0 {
		// hikari speaks for its modules, before the user (decision 3 of
		// the task pipeline spec).
		sys := api.Peer{Name: topology.Hikari, Role: s.topo.Role(topology.Hikari), Description: "hikari itself, which writes the messages of its modules."}
		out = append(out[:len(out)-1], sys, out[len(out)-1])
	}
	return out
}

// Subscribe returns a channel that first receives a snapshot of the server
// and then every later event. The server closes the channel if the
// subscriber falls behind; cancel unsubscribes.
func (s *Server) Subscribe() (events <-chan Event, cancel func()) {
	ch := make(chan Event, 256)
	s.mu.Lock()
	snap := s.snapshot()
	ch <- Event{Type: "snapshot", Snapshot: &snap}
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, ok := s.subs[ch]; ok {
			delete(s.subs, ch)
			close(ch)
		}
	}
}

// PublishTopology tells the UI that the topology file changed on disk.
func (s *Server) PublishTopology(revision string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publish(Event{Type: "topology", Revision: revision})
}

func (s *Server) publish(e Event) {
	for ch := range s.subs {
		select {
		case ch <- e:
		default:
			delete(s.subs, ch)
			close(ch)
		}
	}
}

func (s *Server) snapshot() Snapshot {
	snap := Snapshot{Version: version.Version, Project: s.topo.Name, Mode: ModeRunning, Me: topology.User, Messages: slices.Clone(s.log)}
	if s.topo.File != "" {
		text, err := os.ReadFile(s.topo.File)
		snap.File, snap.Revision = linked.Shown(s.topo.File), editor.Revision(text, err == nil)
	}
	if snap.Messages == nil {
		snap.Messages = []api.Message{}
	}
	for _, n := range s.topo.AgentNames() {
		snap.Agents = append(snap.Agents, AgentView{
			Name:        n,
			Description: s.topo.Agents[n].Description,
			Role:        s.topo.Role(n),
			Status:      s.status[n],
		})
	}
	snap.Participants = s.participants()
	snap.Partitions = s.sys.Partitions()
	snap.Vocabulary = s.sys.Vocabulary()
	snap.Repos = s.sys.Repos()
	snap.Deliverables = s.deliverables(s.sys.Vault())
	snap.Wikis = s.sys.Wikis()
	snap.Modules = s.moduleViews()
	snap.Feed = slices.Clone(s.entries)
	if snap.Feed == nil {
		snap.Feed = []api.Entry{}
	}
	snap.Cost = s.cost
	if s.assistant != nil {
		snap.Assistant = s.assistant.View()
	} else {
		snap.Assistant = assistant.Off(s.assistantOff)
	}
	snap.Activity = map[string][]api.Activity{}
	for n, list := range s.activity {
		snap.Activity[n] = slices.Clone(list)
	}
	return snap
}

func list(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}
