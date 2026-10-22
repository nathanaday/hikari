// Package server routes messages between agents and records every message in
// one log. It is the only authority on the topology: a node can post only to
// hubs it is in, and can send direct messages only to nodes it shares a hub
// with.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/systems"
	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/internal/version"
	"github.com/nathanaday/hikari/internal/vocab"
)

const maxBodyBytes = 64 << 10

type Event struct {
	Type       string            `json:"type"`
	Message    *api.Message      `json:"message,omitempty"`
	Node       string            `json:"node,omitempty"`
	Status     *api.Status       `json:"status,omitempty"`
	Activity   *api.Activity     `json:"activity,omitempty"`
	System     *api.SystemView   `json:"system,omitempty"`
	Vocabulary *vocab.Vocabulary `json:"vocabulary,omitempty"`
	Snapshot   *Snapshot         `json:"snapshot,omitempty"`
}

type NodeView struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Hubs        []string   `json:"hubs"`
	Status      api.Status `json:"status"`
}

type Snapshot struct {
	// Version is the version of the running hikari.
	Version      string             `json:"version"`
	Me           string             `json:"me"`
	Agents       []NodeView         `json:"agents"`
	Hubs         []api.HubInfo      `json:"hubs"`
	Messages     []api.Message      `json:"messages"`
	Systems      []api.SystemView   `json:"systems"`
	Vocabularies []vocab.Vocabulary `json:"vocabularies"`
	// Activity holds the recent steps of each agent.
	Activity map[string][]api.Activity `json:"activity"`
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
	sys     *systems.Manager

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
}

// maxActivity is the most entries kept per agent. Older turns are dropped
// whole.
const maxActivity = 400

// New returns a server for topo and its systems. Each message is also
// written to journal as one line of JSON when journal is not nil.
func New(topo *topology.Topology, journal io.Writer, sys *systems.Manager) *Server {
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
	sys.OnChange(s.systemChanged)
	return s
}

// Systems returns the systems manager.
func (s *Server) Systems() *systems.Manager { return s.sys }

func (s *Server) systemChanged(c systems.Change) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.System != "" {
		v := s.sys.SystemView(c.System)
		s.publish(Event{Type: "system", System: &v})
	}
	if c.Hub != "" {
		v := s.sys.Vocabulary(c.Hub)
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

// IssueToken returns a new credential that identifies node to the server.
func (s *Server) IssueToken(node string) string {
	t := NewToken()
	s.mu.Lock()
	s.tokens[t] = node
	s.mu.Unlock()
	return t
}

func (s *Server) nodeForToken(token string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.tokens[token]
	return n, ok
}

var errForbidden = errors.New("not allowed")

// Send posts to a hub or sends a direct message, as req says.
func (s *Server) Send(from string, req api.SendRequest) (api.Message, error) {
	if strings.TrimSpace(req.Body) == "" {
		return api.Message{}, errors.New("message body is empty")
	}
	if len(req.Body) > maxBodyBytes {
		return api.Message{}, fmt.Errorf("message body exceeds %d bytes", maxBodyBytes)
	}
	switch {
	case req.Hub != "" && req.To != "":
		return api.Message{}, errors.New("set either hub or to, not both")
	case req.Hub != "":
		return s.post(from, req.Hub, req.Body)
	case req.To != "":
		return s.direct(from, req.To, req.Body)
	default:
		return api.Message{}, errors.New("set hub to post to a hub, or to to send a direct message")
	}
}

func (s *Server) post(from, hub, body string) (api.Message, error) {
	if !s.topo.InHub(from, hub) {
		return api.Message{}, fmt.Errorf("%w: %q is not in hub %q; hubs of %q are %s",
			errForbidden, from, hub, from, list(s.topo.HubsOf(from)))
	}
	h := s.topo.Hubs[hub]
	participants := s.topo.Participants(hub)
	mentioned := map[string]bool{}
	for name := range mentions(body) {
		if slices.Contains(participants, name) {
			mentioned[name] = true
		}
	}
	var recipients []recipient
	for _, p := range participants {
		if p == from || p == topology.User {
			continue
		}
		wake := p == h.Orchestrator ||
			mentioned[p] ||
			(from == h.Orchestrator && len(mentioned) == 0)
		recipients = append(recipients, recipient{name: p, wake: wake})
	}
	return s.deliver(api.Message{From: from, Hub: hub, Body: body}, recipients), nil
}

func (s *Server) direct(from, to, body string) (api.Message, error) {
	if !s.topo.CanDirect(from, to) {
		return api.Message{}, fmt.Errorf("%w: %q shares no hub with %q; %q can send direct messages to %s",
			errForbidden, from, to, from, list(s.topo.Contacts(from)))
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

func (s *Server) notify(node string) {
	close(s.signal[node])
	s.signal[node] = make(chan struct{})
}

// Take removes and returns every pending message of node. With wait == 0 it
// returns at once. With wait > 0 it first waits until a pending message wakes
// the node; if wait elapses or ctx ends first, it returns nothing and leaves
// the inbox as it was.
func (s *Server) Take(ctx context.Context, node string, wait time.Duration) []api.Message {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		s.mu.Lock()
		pending := s.inbox[node]
		if wait == 0 || slices.ContainsFunc(pending, func(d delivery) bool { return d.wake }) {
			s.inbox[node] = nil
			s.mu.Unlock()
			msgs := make([]api.Message, len(pending))
			for i, d := range pending {
				msgs[i] = d.msg
			}
			return msgs
		}
		ch := s.signal[node]
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

// Requeue puts msgs back at the front of the inbox of node, for when a Take
// result could not be delivered. The messages wake the node again.
func (s *Server) Requeue(node string, msgs []api.Message) {
	if len(msgs) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	back := make([]delivery, len(msgs))
	for i, m := range msgs {
		back[i] = delivery{msg: m, wake: true}
	}
	s.inbox[node] = append(back, s.inbox[node]...)
	s.notify(node)
}

func (s *Server) SetStatus(node string, st api.Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status[node] = st
	s.publish(Event{Type: "status", Node: node, Status: &st})
}

// AddActivity records a step of node's turn. An entry with the ID of a kept
// entry replaces it. A closing text is marked unheard when the agent sent
// no message during the turn.
func (s *Server) AddActivity(node string, a api.Activity) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a.Time.IsZero() {
		a.Time = time.Now().UTC()
	}
	switch a.Kind {
	case api.ActivityTurn:
		if !a.Done {
			s.turnStart[node] = s.nextID
		}
	case api.ActivityDone:
		a.Unheard = a.Text != "" && !s.sentSince(node, s.turnStart[node])
	}
	list := s.activity[node]
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
	s.activity[node] = list
	s.publish(Event{Type: "activity", Node: node, Activity: &a})
}

func (s *Server) sentSince(node string, id int64) bool {
	for i := len(s.log) - 1; i >= 0 && s.log[i].ID > id; i-- {
		if s.log[i].From == node {
			return true
		}
	}
	return false
}

func (s *Server) Self(node string) api.Self {
	a := s.topo.Agents[node]
	self := api.Self{
		Name:         node,
		Description:  a.Description,
		Instructions: a.Instructions,
		Model:        a.Model,
	}
	for _, h := range s.topo.HubsOf(node) {
		self.Hubs = append(self.Hubs, s.hubInfo(h))
	}
	self.System, self.Orchestrates = s.sys.Self(node)
	self.VisibleSystems = s.topo.VisibleSystems(node)
	self.Releases = s.topo.ReadableReleases(node)
	self.Deliverables = s.sys.Deliverables(node)
	for _, hub := range s.topo.OrchestratedHubs(node) {
		for _, m := range s.topo.Hubs[hub].Members {
			if g := s.topo.Agents[m].Grants.Releases; len(g) > 0 {
				if self.MemberGrants == nil {
					self.MemberGrants = map[string][]string{}
				}
				self.MemberGrants[m] = g
			}
			if d := s.sys.Deliverables(m); len(d) > 0 {
				if self.MemberDeliverables == nil {
					self.MemberDeliverables = map[string][]api.DeliverableInfo{}
				}
				self.MemberDeliverables[m] = d
			}
		}
	}
	return self
}

func (s *Server) hubInfo(hub string) api.HubInfo {
	h := s.topo.Hubs[hub]
	info := api.HubInfo{Name: hub, Description: h.Description}
	for _, p := range s.topo.Participants(hub) {
		peer := api.Peer{Name: p, Role: api.RoleMember, Description: s.topo.Agents[p].Description}
		switch p {
		case h.Orchestrator:
			peer.Role = api.RoleOrchestrator
		case topology.User:
			peer.Role = api.RoleOperator
			peer.Description = "The human operator."
		}
		info.Participants = append(info.Participants, peer)
	}
	return info
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
	snap := Snapshot{Version: version.Version, Me: topology.User, Messages: slices.Clone(s.log)}
	if snap.Messages == nil {
		snap.Messages = []api.Message{}
	}
	for _, n := range s.topo.AgentNames() {
		snap.Agents = append(snap.Agents, NodeView{
			Name:        n,
			Description: s.topo.Agents[n].Description,
			Hubs:        s.topo.HubsOf(n),
			Status:      s.status[n],
		})
	}
	for _, h := range s.topo.HubNames() {
		snap.Hubs = append(snap.Hubs, s.hubInfo(h))
	}
	snap.Systems = s.sys.Systems()
	snap.Vocabularies = s.sys.Vocabularies()
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
