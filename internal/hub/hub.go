// Package hub routes messages between adjacent nodes and records every
// message in one log. It is the only authority on the topology: a node can
// reach another node only when the hub says the two are connected.
package hub

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/topology"
)

const maxBodyBytes = 64 << 10

type Event struct {
	Type     string       `json:"type"`
	Message  *api.Message `json:"message,omitempty"`
	Node     string       `json:"node,omitempty"`
	Status   *api.Status  `json:"status,omitempty"`
	Snapshot *Snapshot    `json:"snapshot,omitempty"`
}

type NodeView struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Human       bool       `json:"human"`
	Neighbors   []string   `json:"neighbors"`
	Status      api.Status `json:"status"`
}

type Snapshot struct {
	Me       string        `json:"me"`
	Nodes    []NodeView    `json:"nodes"`
	Messages []api.Message `json:"messages"`
}

type Hub struct {
	topo    *topology.Topology
	journal io.Writer

	mu     sync.Mutex
	tokens map[string]string
	nextID int64
	log    []api.Message
	inbox  map[string][]api.Message
	wake   map[string]chan struct{}
	status map[string]api.Status
	subs   map[chan Event]struct{}
}

// New returns a hub for topo. Each message is also written to journal as one
// line of JSON when journal is not nil.
func New(topo *topology.Topology, journal io.Writer) *Hub {
	h := &Hub{
		topo:    topo,
		journal: journal,
		tokens:  map[string]string{},
		inbox:   map[string][]api.Message{},
		wake:    map[string]chan struct{}{},
		status:  map[string]api.Status{},
		subs:    map[chan Event]struct{}{},
	}
	for _, n := range topo.Agents() {
		h.wake[n] = make(chan struct{})
		h.status[n] = api.Status{State: api.StateStarting}
	}
	return h
}

func NewToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// IssueToken returns a new credential that identifies node to the hub.
func (h *Hub) IssueToken(node string) string {
	t := NewToken()
	h.mu.Lock()
	h.tokens[t] = node
	h.mu.Unlock()
	return t
}

func (h *Hub) nodeForToken(token string) (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	n, ok := h.tokens[token]
	return n, ok
}

var errNotNeighbor = errors.New("not a neighbor")

func (h *Hub) Send(from, to, body string) (api.Message, error) {
	if strings.TrimSpace(body) == "" {
		return api.Message{}, errors.New("message body is empty")
	}
	if len(body) > maxBodyBytes {
		return api.Message{}, fmt.Errorf("message body exceeds %d bytes", maxBodyBytes)
	}
	if !h.topo.Connected(from, to) {
		return api.Message{}, fmt.Errorf("%w: %q cannot reach %q; neighbors of %q are %s",
			errNotNeighbor, from, to, from, strings.Join(h.topo.Neighbors(from), ", "))
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextID++
	m := api.Message{ID: h.nextID, From: from, To: to, Body: body, Time: time.Now().UTC()}
	h.log = append(h.log, m)
	if to != topology.User {
		h.inbox[to] = append(h.inbox[to], m)
		close(h.wake[to])
		h.wake[to] = make(chan struct{})
	}
	if h.journal != nil {
		line, _ := json.Marshal(m)
		h.journal.Write(append(line, '\n'))
	}
	h.publish(Event{Type: "message", Message: &m})
	return m, nil
}

// Take removes and returns the pending messages of node. If none are pending,
// it waits until one arrives, wait elapses, or ctx ends.
func (h *Hub) Take(ctx context.Context, node string, wait time.Duration) []api.Message {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		h.mu.Lock()
		if msgs := h.inbox[node]; len(msgs) > 0 {
			h.inbox[node] = nil
			h.mu.Unlock()
			return msgs
		}
		ch := h.wake[node]
		h.mu.Unlock()
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
// result could not be delivered.
func (h *Hub) Requeue(node string, msgs []api.Message) {
	if len(msgs) == 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.inbox[node] = append(msgs, h.inbox[node]...)
	close(h.wake[node])
	h.wake[node] = make(chan struct{})
}

func (h *Hub) SetStatus(node string, s api.Status) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.status[node] = s
	h.publish(Event{Type: "status", Node: node, Status: &s})
}

func (h *Hub) Self(node string) api.Self {
	n := h.topo.Nodes[node]
	return api.Self{
		Name:         node,
		Description:  n.Description,
		Instructions: n.Instructions,
		Model:        n.Model,
		Neighbors:    h.peers(node),
	}
}

func (h *Hub) peers(node string) []api.Peer {
	var out []api.Peer
	for _, name := range h.topo.Neighbors(node) {
		desc := h.topo.Nodes[name].Description
		if name == topology.User {
			desc = "The human operator."
		}
		out = append(out, api.Peer{Name: name, Description: desc})
	}
	return out
}

// Subscribe returns a channel that first receives a snapshot of the hub and
// then every later event. The hub closes the channel if the subscriber falls
// behind; cancel unsubscribes.
func (h *Hub) Subscribe() (events <-chan Event, cancel func()) {
	ch := make(chan Event, 256)
	h.mu.Lock()
	snap := h.snapshot()
	ch <- Event{Type: "snapshot", Snapshot: &snap}
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
	}
}

func (h *Hub) publish(e Event) {
	for ch := range h.subs {
		select {
		case ch <- e:
		default:
			delete(h.subs, ch)
			close(ch)
		}
	}
}

func (h *Hub) snapshot() Snapshot {
	s := Snapshot{Me: topology.User, Messages: append([]api.Message(nil), h.log...)}
	s.Nodes = append(s.Nodes, NodeView{
		Name:        topology.User,
		Description: "You, the human operator.",
		Human:       true,
		Neighbors:   h.topo.Neighbors(topology.User),
		Status:      api.Status{State: api.StateIdle},
	})
	for _, n := range h.topo.Agents() {
		s.Nodes = append(s.Nodes, NodeView{
			Name:        n,
			Description: h.topo.Nodes[n].Description,
			Neighbors:   h.topo.Neighbors(n),
			Status:      h.status[n],
		})
	}
	return s
}
