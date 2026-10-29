// Package assistant runs the hikari agent for the UI: your own Claude Code,
// on the host, in the project root, one headless turn per message, with
// its permission prompts answered from the panel (the hikari agent spec).
package assistant

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/claude"
)

// The permission modes of the menu (decision 6).
const (
	ModeAsk         = "default"
	ModeAcceptEdits = "acceptEdits"
	ModePlan        = "plan"
	ModeBypass      = "bypassPermissions"
)

var modes = []string{ModeAsk, ModeAcceptEdits, ModePlan, ModeBypass}
var efforts = []string{"", "low", "medium", "high", "xhigh", "max"}

// The states of the assistant.
const (
	StateIdle    = "idle"
	StateWorking = "working"
)

// maxActivity is the most entries kept, as the server keeps an agent's.
const maxActivity = 400

// Message is one of your messages to the hikari agent.
type Message struct {
	ID   int64     `json:"id"`
	Time time.Time `json:"time"`
	Text string    `json:"text"`
}

// Prompt is a permission that a turn asked for (rule HA5). Answer is
// "allow" or "deny" once given, with Message as the reason of a deny.
type Prompt struct {
	ID          string          `json:"id"`
	Turn        int64           `json:"turn"`
	Time        time.Time       `json:"time"`
	Tool        string          `json:"tool"`
	Input       json.RawMessage `json:"input,omitempty"`
	Description string          `json:"description,omitempty"`
	Answer      string          `json:"answer,omitempty"`
	Message     string          `json:"message,omitempty"`
}

// Status is the assistant's state and options (rule HA7).
type Status struct {
	Session        string   `json:"session"`
	Model          string   `json:"model"`
	Effort         string   `json:"effort"`
	PermissionMode string   `json:"permission_mode"`
	State          string   `json:"state"`
	Cost           float64  `json:"cost"`
	Commands       []string `json:"commands"`
}

// View is the assistant's part of the snapshot.
type View struct {
	Enabled bool   `json:"enabled"`
	Reason  string `json:"reason,omitempty"`
	Status
	Messages []Message      `json:"messages"`
	Activity []api.Activity `json:"activity"`
	Prompts  []Prompt       `json:"prompts"`
}

// Off is the view of a run without the assistant.
func Off(reason string) View {
	return View{Reason: reason, Messages: []Message{}, Activity: []api.Activity{}, Prompts: []Prompt{}}
}

// Event is one change of the assistant, for the UI.
type Event struct {
	Type     string        `json:"type"` // message, activity, prompt, or status
	Message  *Message      `json:"message,omitempty"`
	Activity *api.Activity `json:"activity,omitempty"`
	Prompt   *Prompt       `json:"prompt,omitempty"`
	Status   *Status       `json:"status,omitempty"`
}

// Options are what the next turn runs with.
type Options struct {
	Model          string `json:"model"`
	Effort         string `json:"effort"`
	PermissionMode string `json:"permission_mode"`
}

// session is session.json.
type session struct {
	Session        string   `json:"session"`
	Model          string   `json:"model,omitempty"`
	Effort         string   `json:"effort,omitempty"`
	PermissionMode string   `json:"permission_mode"`
	Cost           float64  `json:"cost"`
	Commands       []string `json:"commands,omitempty"`
}

// line is one line of a transcript.
type line struct {
	Kind     string        `json:"kind"`
	Message  *Message      `json:"message,omitempty"`
	Activity *api.Activity `json:"activity,omitempty"`
	Prompt   *Prompt       `json:"prompt,omitempty"`
}

// Assistant is the hikari agent of one project.
type Assistant struct {
	dir, root, context string
	listen             func(Event)
	// Command runs Claude Code; tests point it at a fake.
	Command string

	mu         sync.Mutex
	s          session
	messages   []Message
	activity   []api.Activity
	prompts    []Prompt
	nextMsg    int64
	state      string
	cancel     context.CancelFunc
	stdin      io.Writer
	stdinMu    sync.Mutex
	pending    map[string]chan Prompt
	tap        *claude.Tap
	transcript *os.File
	done       chan struct{}
	// early holds the lines of a turn before its session id is known.
	early []line
	// events carries each change to the listener, outside the lock, in
	// order.
	events chan Event
}

// New opens the assistant of a project (rule HA2): dir is .hikari/assistant,
// root the project root, context the lines that each turn's system prompt
// gets, and listen receives each change.
func New(dir, root, context string, listen func(Event)) (*Assistant, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	a := &Assistant{dir: dir, root: root, context: context, listen: listen, Command: "claude",
		state: StateIdle, pending: map[string]chan Prompt{}, s: session{PermissionMode: ModeAsk}}
	a.tap = claude.NewTap(a.postActivity)
	a.events = make(chan Event, 1024)
	events := a.events
	go func() {
		for e := range events {
			if listen != nil {
				listen(e)
			}
		}
	}()
	if data, err := os.ReadFile(filepath.Join(dir, "session.json")); err == nil {
		if err := json.Unmarshal(data, &a.s); err != nil {
			return nil, fmt.Errorf("session.json: %w", err)
		}
		if a.s.PermissionMode == "" {
			a.s.PermissionMode = ModeAsk
		}
	}
	if a.s.Session != "" {
		if err := a.load(); err != nil {
			return nil, err
		}
	}
	return a, nil
}

// load reads the session's transcript.
func (a *Assistant) load() error {
	f, err := os.Open(a.transcriptPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	var turn, id int64
	for sc.Scan() {
		var l line
		if json.Unmarshal(sc.Bytes(), &l) != nil {
			continue
		}
		switch {
		case l.Kind == "message" && l.Message != nil:
			a.messages = append(a.messages, *l.Message)
			a.nextMsg = max(a.nextMsg, l.Message.ID)
		case l.Kind == "activity" && l.Activity != nil:
			a.keep(*l.Activity)
			turn, id = max(turn, l.Activity.Turn), max(id, l.Activity.ID)
		case l.Kind == "prompt" && l.Prompt != nil:
			a.keepPrompt(*l.Prompt)
		}
	}
	a.tap.Resume(turn, id)
	// A turn that was running when hikari stopped never ended.
	if n := len(a.activity); n > 0 && a.activity[n-1].Kind != api.ActivityDone {
		a.keep(api.Activity{ID: id + 1, Turn: turn, Time: time.Now().UTC(), Kind: api.ActivityDone, Done: true, Error: true, Text: "hikari stopped during the turn"})
		a.tap.Resume(turn, id+1)
	}
	return nil
}

func (a *Assistant) transcriptPath() string {
	return filepath.Join(a.dir, a.s.Session+".jsonl")
}

// keep adds an entry, replacing one with its id, and drops old turns
// whole beyond maxActivity.
func (a *Assistant) keep(e api.Activity) {
	if i := slices.IndexFunc(a.activity, func(x api.Activity) bool { return x.ID == e.ID }); i >= 0 {
		a.activity[i] = e
	} else {
		a.activity = append(a.activity, e)
	}
	for len(a.activity) > maxActivity {
		turn := a.activity[0].Turn
		n := 0
		for n < len(a.activity) && a.activity[n].Turn == turn {
			n++
		}
		a.activity = slices.Clone(a.activity[n:])
	}
}

func (a *Assistant) keepPrompt(p Prompt) {
	if i := slices.IndexFunc(a.prompts, func(x Prompt) bool { return x.ID == p.ID }); i >= 0 {
		a.prompts[i] = p
	} else {
		a.prompts = append(a.prompts, p)
	}
}

// record appends l to the transcript of the session, opening it when the
// session is known. The caller holds mu.
func (a *Assistant) record(l line) {
	if a.s.Session == "" {
		return
	}
	if a.transcript == nil || a.transcript.Name() != a.transcriptPath() {
		if a.transcript != nil {
			a.transcript.Close()
		}
		f, err := os.OpenFile(a.transcriptPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			log.Printf("assistant: %v", err)
			return
		}
		a.transcript = f
	}
	data, _ := json.Marshal(l)
	a.transcript.Write(append(data, '\n'))
}

// flush records the lines kept before the session id was known.
func (a *Assistant) flush() {
	for _, l := range a.early {
		a.record(l)
	}
	a.early = nil
}

// writeSession writes session.json. The caller holds mu.
func (a *Assistant) writeSession() {
	data, _ := json.MarshalIndent(a.s, "", "  ")
	if err := os.WriteFile(filepath.Join(a.dir, "session.json"), append(data, '\n'), 0o644); err != nil {
		log.Printf("assistant: %v", err)
	}
}

func (a *Assistant) status() Status {
	cmds := a.s.Commands
	if cmds == nil {
		cmds = []string{}
	}
	return Status{Session: a.s.Session, Model: a.s.Model, Effort: a.s.Effort, PermissionMode: a.s.PermissionMode,
		State: a.state, Cost: a.s.Cost, Commands: cmds}
}

// View returns the assistant's part of the snapshot.
func (a *Assistant) View() View {
	a.mu.Lock()
	defer a.mu.Unlock()
	v := View{Enabled: true, Status: a.status(), Messages: slices.Clone(a.messages), Activity: slices.Clone(a.activity), Prompts: slices.Clone(a.prompts)}
	if v.Messages == nil {
		v.Messages = []Message{}
	}
	if v.Activity == nil {
		v.Activity = []api.Activity{}
	}
	if v.Prompts == nil {
		v.Prompts = []Prompt{}
	}
	return v
}

// emit queues e for the listener, so that no lock is held when it runs.
func (a *Assistant) emit(e Event) {
	if a.events == nil {
		return
	}
	select {
	case a.events <- e:
	default:
		log.Print("assistant: the listener fell behind; an event was dropped")
	}
}

// emitStatus sends the status. The caller holds mu.
func (a *Assistant) emitStatus() {
	st := a.status()
	a.emit(Event{Type: "status", Status: &st})
}

// SetOptions keeps the model, the effort, and the permission mode for the
// next turn (rule HA6).
func (a *Assistant) SetOptions(o Options) error {
	if !slices.Contains(modes, o.PermissionMode) {
		return fmt.Errorf("the permission mode must be one of %s", strings.Join(modes, ", "))
	}
	if !slices.Contains(efforts, o.Effort) {
		return fmt.Errorf("the effort must be one of %s", strings.Join(efforts[1:], ", "))
	}
	if strings.ContainsAny(o.Model, " \t\n") {
		return errors.New("the model must be one word")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.s.Model, a.s.Effort, a.s.PermissionMode = o.Model, o.Effort, o.PermissionMode
	a.writeSession()
	a.emitStatus()
	return nil
}

// NewChat forgets the session, so that the next turn starts one (rule HA6).
func (a *Assistant) NewChat() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state == StateWorking {
		return errors.New("the hikari agent is working; stop it first")
	}
	a.s.Session, a.s.Cost, a.s.Commands = "", 0, nil
	a.messages, a.activity, a.prompts = nil, nil, nil
	a.tap.Resume(0, 0)
	if a.transcript != nil {
		a.transcript.Close()
		a.transcript = nil
	}
	a.writeSession()
	a.emitStatus()
	return nil
}

// Send runs one turn with text as your message (rule HA3).
func (a *Assistant) Send(text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("the message is empty")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state == StateWorking {
		return errors.New("the hikari agent is working; stop it or wait")
	}
	a.nextMsg++
	m := Message{ID: a.nextMsg, Time: time.Now().UTC(), Text: text}
	a.messages = append(a.messages, m)
	a.put(line{Kind: "message", Message: &m})
	a.emit(Event{Type: "message", Message: &m})
	a.state = StateWorking
	a.emitStatus()
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.done = make(chan struct{})
	go a.run(ctx, text, a.done)
	return nil
}

// put records a line, or keeps it until the session is known.
func (a *Assistant) put(l line) {
	if a.s.Session == "" {
		a.early = append(a.early, l)
		return
	}
	a.record(l)
}

// args are the flags of a turn (rule HA3).
func (a *Assistant) args() []string {
	args := []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--include-partial-messages",
		"--permission-prompt-tool", "stdio", "--permission-mode", a.s.PermissionMode}
	if a.s.PermissionMode == ModeBypass {
		args = append(args, "--dangerously-skip-permissions")
	}
	if a.s.Model != "" {
		args = append(args, "--model", a.s.Model)
	}
	if a.s.Effort != "" {
		args = append(args, "--effort", a.s.Effort)
	}
	if a.s.Session != "" {
		args = append(args, "--resume", a.s.Session)
	}
	if a.context != "" {
		args = append(args, "--append-system-prompt", a.context)
	}
	return args
}

func (a *Assistant) run(ctx context.Context, text string, done chan struct{}) {
	defer close(done)
	a.mu.Lock()
	cmd := exec.CommandContext(ctx, a.Command, a.args()...)
	a.mu.Unlock()
	cmd.Dir = a.root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		a.end(ctx, "", 0, 0, err)
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		a.end(ctx, "", 0, 0, err)
		return
	}
	start := time.Now()
	if err := cmd.Start(); err != nil {
		a.end(ctx, "", 0, 0, err)
		return
	}
	a.mu.Lock()
	a.stdin = stdin
	a.tap.Begin("your message")
	a.mu.Unlock()
	user, _ := json.Marshal(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": text}})
	a.write(append(user, '\n'))

	var final *claude.Event
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var e claude.Event
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			continue
		}
		a.mu.Lock()
		if e.SessionID != "" && a.s.Session != e.SessionID {
			a.s.Session = e.SessionID
			a.flush()
		}
		if e.Type == "system" && e.Subtype == "init" && len(e.SlashCommands) > 0 {
			a.s.Commands = slices.Sorted(slices.Values(e.SlashCommands))
			a.emitStatus()
		}
		a.tap.Observe(e)
		a.mu.Unlock()
		switch {
		case e.Type == "control_request" && e.Request.Subtype == "can_use_tool":
			a.ask(e)
		case e.Type == "result":
			final = &e
			stdin.Close()
		}
	}
	waitErr := cmd.Wait()
	cost, closing := 0.0, ""
	switch {
	case ctx.Err() != nil:
		err = errors.New("stopped")
	case final != nil && final.IsError:
		err = fmt.Errorf("claude: %s", clip(final.Result, 500))
	case final != nil:
		cost, closing = final.Cost, final.Result
	case waitErr != nil:
		err = fmt.Errorf("claude: %v: %s", waitErr, clip(stderr.String(), 500))
	default:
		err = errors.New("claude exited without a result")
	}
	a.end(ctx, closing, cost, time.Since(start), err)
}

// write sends a line to the turn's stdin.
func (a *Assistant) write(data []byte) {
	a.stdinMu.Lock()
	defer a.stdinMu.Unlock()
	if a.stdin != nil {
		a.stdin.Write(data)
	}
}

// ask turns a can_use_tool request into a prompt, waits for its answer,
// and writes the control response (rule HA5).
func (a *Assistant) ask(e claude.Event) {
	ch := make(chan Prompt, 1)
	a.mu.Lock()
	p := Prompt{ID: e.RequestID, Turn: a.tap.Turn(), Time: time.Now().UTC(), Tool: e.Request.ToolName, Input: e.Request.Input, Description: e.Request.Description}
	a.keepPrompt(p)
	a.pending[p.ID] = ch
	a.put(line{Kind: "prompt", Prompt: &p})
	a.emit(Event{Type: "prompt", Prompt: &p})
	a.mu.Unlock()
	go func() {
		answered := <-ch
		var response map[string]any
		if answered.Answer == "allow" {
			response = map[string]any{"behavior": "allow", "updatedInput": json.RawMessage(orEmpty(answered.Input))}
		} else {
			response = map[string]any{"behavior": "deny", "message": answered.Message}
		}
		data, _ := json.Marshal(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": answered.ID, "response": response}})
		a.write(append(data, '\n'))
	}()
}

func orEmpty(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("{}")
	}
	return raw
}

// Answer answers a pending prompt (rule HA5).
func (a *Assistant) Answer(id string, allow bool, message string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.answer(id, allow, message)
}

// answer answers a prompt. The caller holds mu.
func (a *Assistant) answer(id string, allow bool, message string) error {
	ch, ok := a.pending[id]
	if !ok {
		return fmt.Errorf("no prompt %s waits for an answer", id)
	}
	delete(a.pending, id)
	i := slices.IndexFunc(a.prompts, func(x Prompt) bool { return x.ID == id })
	p := a.prompts[i]
	if allow {
		p.Answer = "allow"
	} else {
		p.Answer = "deny"
		p.Message = message
		if p.Message == "" {
			p.Message = "The person denied it in hikari's panel."
		}
	}
	a.prompts[i] = p
	a.record(line{Kind: "prompt", Prompt: &p})
	a.emit(Event{Type: "prompt", Prompt: &p})
	ch <- p
	return nil
}

// Stop ends the running turn (rule HA6).
func (a *Assistant) Stop() {
	a.mu.Lock()
	cancel, done := a.cancel, a.done
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

// end closes the turn: the pending prompts are denied, the closing text or
// the error ends the turn in the tap, and the cost is added.
func (a *Assistant) end(ctx context.Context, closing string, cost float64, duration time.Duration, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for id := range a.pending {
		a.answer(id, false, "The turn ended.")
	}
	a.stdin = nil
	a.tap.End(closing, cost, duration, err)
	a.s.Cost += cost
	a.state = StateIdle
	a.cancel = nil
	a.writeSession()
	a.emitStatus()
}

// postActivity keeps an entry of the tap. The caller holds mu.
func (a *Assistant) postActivity(e api.Activity) {
	a.keep(e)
	a.put(line{Kind: "activity", Activity: &e})
	a.emit(Event{Type: "activity", Activity: &e})
}

// Close stops a running turn and closes the transcript.
func (a *Assistant) Close() {
	a.Stop()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.transcript != nil {
		a.transcript.Close()
		a.transcript = nil
	}
	close(a.events)
	a.events = nil
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// Context writes the lines of a turn's system prompt (rule HA4).
func Context(project, root, runDir, url string) string {
	return strings.Join([]string{
		fmt.Sprintf("You are the hikari agent, running inside hikari's web UI for the project %s, whose root is %s.", project, root),
		fmt.Sprintf("hikari runs for it now, with its UI at %s. The run's folder is %s: its feed, one line of JSON per event, is feed.jsonl there, and its messages are messages.jsonl.", url, runDir),
		"The person reads your closing text as your reply in the panel, so end each turn with what they should know. Questions go in the closing text too.",
		"The Hikari plugin's skills route requests about the project; post to the room as the person with hikari send, as the room skill says.",
	}, "\n")
}

// Enabled says whether a run gets the assistant (rule HA10): mode is on,
// off, or auto, and host the listener's host.
func Enabled(mode, host string) (bool, string) {
	switch mode {
	case "off":
		return false, "the hikari agent is off; pass -agent auto or on to run it"
	case "on":
	default:
		if !loopback(host) {
			return false, "the hikari agent is off for a listener outside this machine; pass -agent on to run it"
		}
	}
	if _, err := exec.LookPath("claude"); err != nil {
		return false, "claude is not on the PATH, and the hikari agent needs it"
	}
	return true, ""
}

func loopback(host string) bool {
	return host == "" || host == "localhost" || strings.HasPrefix(host, "127.") || host == "::1"
}
