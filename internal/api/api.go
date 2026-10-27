// Package api holds the wire types of the server's agent API and a client for
// it.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// Message is a post to the room, or a direct message when To is set.
type Message struct {
	ID   int64     `json:"id"`
	From string    `json:"from"`
	To   string    `json:"to,omitempty"`
	Body string    `json:"body"`
	Time time.Time `json:"time"`
	// Woke lists the recipients that the message started a turn for.
	Woke []string `json:"woke,omitempty"`
}

const (
	RoleOrchestration = "orchestration"
	RolePartition     = "partition"
	RoleOperator      = "operator"
)

type Peer struct {
	Name        string `json:"name"`
	Role        string `json:"role"`
	Description string `json:"description,omitempty"`
}

type Self struct {
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	Instructions string `json:"instructions,omitempty"`
	Model        string `json:"model,omitempty"`
	// Participants lists everyone in the room: the orchestration agent first and
	// the user last.
	Participants []Peer `json:"participants"`
	// Partition is set when the agent has a partition.
	Partition *SelfPartition `json:"partition,omitempty"`
	// Orchestration says that the agent is the project's orchestration agent.
	Orchestration bool `json:"orchestration,omitempty"`
	// VisiblePartitions lists the partitions whose maps the orchestration agent reads.
	VisiblePartitions []string `json:"visible_partitions,omitempty"`
	// Releases lists the partitions whose releases the agent can read.
	Releases []string `json:"releases,omitempty"`
	// Deliverables lists the deliverables that a partition agent publishes.
	Deliverables []DeliverableInfo `json:"deliverables,omitempty"`
	// PartitionAgentGrants maps each partition agent, for the orchestration agent, to the partitions
	// whose releases it is granted.
	PartitionAgentGrants map[string][]string `json:"partition_agent_grants,omitempty"`
	// PartitionAgentDeliverables maps each partition agent, for the orchestration agent, to the
	// deliverables it publishes.
	PartitionAgentDeliverables map[string][]DeliverableInfo `json:"partition_agent_deliverables,omitempty"`
}

// DeliverableInfo names a deliverable for a prompt. Description is the
// deliverable's description, or a note when the deliverable is invalid.
type DeliverableInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// PartitionSkill and OrchestrationSkill are hikari's own skills, which always come from
// the binary: one for partition agents, one for orchestration agents. No shared skill or
// deliverable may use their names.
const (
	PartitionSkill     = "hikari-partition"
	OrchestrationSkill = "hikari-orchestration"
)

// BuiltinSkills are hikari's own skills.
var BuiltinSkills = []string{PartitionSkill, OrchestrationSkill}

// AgentPrefix starts the name of each of hikari's own agents. No shared
// agent may use it.
const AgentPrefix = "hikari-"

// SendRequest posts to the room, or sends a direct message when To is set.
type SendRequest struct {
	To   string `json:"to,omitempty"`
	Body string `json:"body"`
}

const (
	StateStarting = "starting"
	StateIdle     = "idle"
	StateWorking  = "working"
	StateIndexing = "indexing"
	StateError    = "error"
	StateExited   = "exited"
)

// Activity kinds.
const (
	ActivityTurn     = "turn"     // a turn started; Text says what woke the agent
	ActivityThinking = "thinking" // the agent is thinking; Done when it stops
	ActivityText     = "text"     // what the agent writes, streamed until Done
	ActivityTool     = "tool"     // a tool call; Done once its result is in
	ActivityDone     = "done"     // the turn ended; Text is its closing text
)

// Activity is one step of an agent's turn, for the live view. The driver
// sends an entry when a step starts and again when it changes, with the
// same ID, so the server replaces it.
type Activity struct {
	ID   int64     `json:"id"`
	Turn int64     `json:"turn"`
	Time time.Time `json:"time"`
	Kind string    `json:"kind"`
	Text string    `json:"text,omitempty"`
	Done bool      `json:"done"`
	// Tool, Input, and Output describe a tool call.
	Tool   string          `json:"tool,omitempty"`
	Input  json.RawMessage `json:"input,omitempty"`
	Output string          `json:"output,omitempty"`
	Error  bool            `json:"error,omitempty"`
	// Cost and Duration (in milliseconds) describe an ended turn.
	Cost     float64 `json:"cost,omitempty"`
	Duration int64   `json:"duration,omitempty"`
	// Unheard marks closing text that reached no one: the agent sent no
	// message during the turn. The server sets it.
	Unheard bool `json:"unheard,omitempty"`
}

type Status struct {
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
	// Cost is the cost in US dollars of the turn that just ended.
	Cost float64 `json:"cost,omitempty"`
}

type errorBody struct {
	Error string `json:"error"`
}

type Client struct {
	URL   string
	Token string
	HTTP  *http.Client
}

func ClientFromEnv() (*Client, error) {
	c := &Client{
		URL:   os.Getenv("HIKARI_SERVER_URL"),
		Token: os.Getenv("HIKARI_TOKEN"),
		HTTP:  &http.Client{Timeout: 90 * time.Second},
	}
	if c.URL == "" || c.Token == "" {
		return nil, errors.New("HIKARI_SERVER_URL and HIKARI_TOKEN must be set")
	}
	return c, nil
}

func (c *Client) Self(ctx context.Context) (Self, error) {
	var s Self
	err := c.do(ctx, http.MethodGet, "/api/agent/self", nil, &s)
	return s, err
}

func (c *Client) Send(ctx context.Context, req SendRequest) (Message, error) {
	var m Message
	err := c.do(ctx, http.MethodPost, "/api/agent/send", req, &m)
	return m, err
}

// Inbox removes and returns the driver's pending messages. With wait > 0, it
// first waits up to wait for a message that wakes the driver.
func (c *Client) Inbox(ctx context.Context, wait time.Duration) ([]Message, error) {
	var msgs []Message
	path := "/api/agent/inbox?wait=" + url.QueryEscape(wait.String())
	err := c.do(ctx, http.MethodGet, path, nil, &msgs)
	return msgs, err
}

func (c *Client) SetStatus(ctx context.Context, s Status) error {
	return c.do(ctx, http.MethodPut, "/api/agent/status", s, nil)
}

func (c *Client) PostActivity(ctx context.Context, a Activity) error {
	return c.do(ctx, http.MethodPost, "/api/agent/activity", a, nil)
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.URL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e errorBody
		if json.NewDecoder(resp.Body).Decode(&e) == nil && e.Error != "" {
			return errors.New(e.Error)
		}
		return fmt.Errorf("server returned %s", resp.Status)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
