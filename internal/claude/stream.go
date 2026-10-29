// Package claude reads the stream of a headless Claude Code session: the
// types of its stream-json output, and a tap that turns the stream into
// activity entries for the live view. The driver uses it in a container,
// and the assistant on the host (rule HA1 of the hikari agent spec).
package claude

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/nathanaday/hikari/internal/api"
)

// Limits on what one activity entry carries. The log keeps the rest.
const (
	maxActivityText   = 20000
	maxActivityInput  = 4000
	maxActivityOutput = 2000
	textFlush         = 250 * time.Millisecond
)

// Tap turns Claude Code's stream into activity entries and posts each one
// as it starts and as it changes, so the UI can show the turn live.
type Tap struct {
	post   func(api.Activity)
	turn   int64
	nextID int64

	// started holds the content blocks of the current message in order;
	// byIndex finds one by its stream index, and tools by tool_use id.
	started []*block
	byIndex map[int]*block
	tools   map[string]*block
}

type block struct {
	entry    api.Activity
	final    bool      // the whole block arrived in an assistant event
	sent     time.Time // when the entry was last posted
	begun    time.Time
	thinking strings.Builder
	text     strings.Builder
}

// Event holds the fields of Claude Code's stream-json output that hikari
// uses.
type Event struct {
	Type      string  `json:"type"`
	Subtype   string  `json:"subtype"`
	SessionID string  `json:"session_id"`
	IsError   bool    `json:"is_error"`
	Result    string  `json:"result"`
	Cost      float64 `json:"total_cost_usd"`
	// SlashCommands are the session's, in the init event.
	SlashCommands []string `json:"slash_commands"`
	Message       struct {
		Content []ContentBlock `json:"content"`
	} `json:"message"`
	// RequestID and Request are set for control_request, which a session
	// with stream-json input writes to ask the host something, such as a
	// permission (subtype can_use_tool).
	RequestID string `json:"request_id"`
	Request   struct {
		Subtype     string          `json:"subtype"`
		ToolName    string          `json:"tool_name"`
		Input       json.RawMessage `json:"input"`
		Description string          `json:"description"`
	} `json:"request"`
	// Event is set for stream_event, the partial messages.
	Event struct {
		Type         string `json:"type"`
		Index        int    `json:"index"`
		ContentBlock struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"content_block"`
		Delta struct {
			Type        string `json:"type"`
			Text        string `json:"text"`
			Thinking    string `json:"thinking"`
			PartialJSON string `json:"partial_json"`
		} `json:"delta"`
	} `json:"event"`
}

// ContentBlock is one block of a message of the stream.
type ContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

// NewTap returns a tap that gives each entry to post.
func NewTap(post func(api.Activity)) *Tap {
	return &Tap{post: post}
}

// Begin opens a turn. trigger says what woke the agent.
func (t *Tap) Begin(trigger string) {
	t.turn++
	t.reset()
	t.send(&block{entry: api.Activity{ID: t.id(), Turn: t.turn, Kind: api.ActivityTurn, Text: trigger}})
}

// End closes the turn with its closing text, cost, and duration, or with
// the error that ended it.
func (t *Tap) End(closing string, cost float64, duration time.Duration, err error) {
	t.reset()
	a := api.Activity{ID: t.id(), Turn: t.turn, Kind: api.ActivityDone, Done: true,
		Text: clipText(closing, maxActivityText), Cost: cost, Duration: duration.Milliseconds()}
	if err != nil {
		a.Error = true
		a.Text = err.Error()
	}
	t.send(&block{entry: a})
}

// Turn is the number of the current turn.
func (t *Tap) Turn() int64 { return t.turn }

// Resume continues after a kept transcript: the next turn is turn+1, and
// the next entry id is id+1.
func (t *Tap) Resume(turn, id int64) { t.turn, t.nextID = turn, id }

func (t *Tap) id() int64 {
	t.nextID++
	return t.nextID
}

func (t *Tap) reset() {
	t.started, t.byIndex, t.tools = nil, map[int]*block{}, map[string]*block{}
}

// Observe handles one event of the stream.
func (t *Tap) Observe(e Event) {
	switch e.Type {
	case "stream_event":
		t.partial(e)
	case "assistant":
		for _, c := range e.Message.Content {
			t.complete(c)
		}
	case "user":
		for _, c := range e.Message.Content {
			if c.Type == "tool_result" {
				t.result(c)
			}
		}
	}
}

// partial handles the start, the deltas, and the end of a content block.
func (t *Tap) partial(e Event) {
	ev := e.Event
	switch ev.Type {
	case "message_start":
		t.started, t.byIndex = nil, map[int]*block{}
	case "content_block_start":
		b := t.open(ev.ContentBlock.Type, ev.ContentBlock.Name)
		if b == nil {
			return
		}
		t.byIndex[ev.Index] = b
		t.send(b)
	case "content_block_delta":
		b := t.byIndex[ev.Index]
		if b == nil || b.final {
			return
		}
		switch ev.Delta.Type {
		case "text_delta":
			b.text.WriteString(ev.Delta.Text)
			b.entry.Text = clipText(b.text.String(), maxActivityText)
		case "thinking_delta":
			b.thinking.WriteString(ev.Delta.Thinking)
			b.entry.Text = clipText(b.thinking.String(), maxActivityInput)
		default:
			return
		}
		if time.Since(b.sent) >= textFlush {
			t.send(b)
		}
	case "content_block_stop":
		b := t.byIndex[ev.Index]
		if b == nil || b.final || b.entry.Kind == api.ActivityTool {
			return
		}
		t.finish(b)
	}
}

// open starts a block of a kind, or returns nil for a kind the view does
// not show.
func (t *Tap) open(kind, name string) *block {
	var entry api.Activity
	switch kind {
	case "text":
		entry = api.Activity{Kind: api.ActivityText}
	case "thinking":
		entry = api.Activity{Kind: api.ActivityThinking}
	case "tool_use":
		entry = api.Activity{Kind: api.ActivityTool, Tool: name}
	default:
		return nil
	}
	entry.ID, entry.Turn = t.id(), t.turn
	b := &block{entry: entry, begun: time.Now()}
	t.started = append(t.started, b)
	return b
}

// complete handles a whole content block from an assistant event: it fills
// in the block that the partial events started, or opens one when the
// stream has no partial messages.
func (t *Tap) complete(c ContentBlock) {
	if c.Type != "text" && c.Type != "thinking" && c.Type != "tool_use" {
		return
	}
	var b *block
	for _, s := range t.started {
		if !s.final && kindOf(c.Type) == s.entry.Kind && (c.Type != "tool_use" || s.entry.Tool == c.Name) {
			b = s
			break
		}
	}
	if b == nil {
		if b = t.open(c.Type, c.Name); b == nil {
			return
		}
	}
	b.final = true
	switch c.Type {
	case "text":
		b.entry.Text = clipText(c.Text, maxActivityText)
		t.finish(b)
	case "thinking":
		if c.Thinking != "" {
			b.entry.Text = clipText(c.Thinking, maxActivityInput)
		}
		t.finish(b)
	case "tool_use":
		b.entry.Input = clipJSON(c.Input, maxActivityInput)
		t.tools[c.ID] = b
		t.send(b)
	}
}

// result fills in a tool call's result and marks it done.
func (t *Tap) result(c ContentBlock) {
	b := t.tools[c.ToolUseID]
	if b == nil {
		return
	}
	delete(t.tools, c.ToolUseID)
	b.entry.Output = clipText(resultText(c.Content), maxActivityOutput)
	b.entry.Error = c.IsError
	b.entry.Duration = time.Since(b.begun).Milliseconds()
	b.entry.Done = true
	t.send(b)
}

func (t *Tap) finish(b *block) {
	b.entry.Done = true
	b.entry.Duration = time.Since(b.begun).Milliseconds()
	t.send(b)
}

func (t *Tap) send(b *block) {
	b.sent = time.Now()
	a := b.entry
	a.Time = time.Now().UTC()
	t.post(a)
}

func kindOf(blockType string) string {
	switch blockType {
	case "text":
		return api.ActivityText
	case "thinking":
		return api.ActivityThinking
	default:
		return api.ActivityTool
	}
}

// resultText returns the text of a tool result, which is a string or a list
// of blocks.
func resultText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		} else {
			parts = append(parts, "["+b.Type+"]")
		}
	}
	return strings.Join(parts, "\n")
}

// clipText keeps the first n bytes of s, cut at a character boundary.
func clipText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !isStart(s[n]) {
		n--
	}
	return s[:n] + "…"
}

func isStart(b byte) bool { return b&0xC0 != 0x80 }

// clipJSON keeps raw when it fits, and else a JSON string of its start, so
// the entry stays valid JSON.
func clipJSON(raw json.RawMessage, n int) json.RawMessage {
	if len(raw) <= n {
		return raw
	}
	out, _ := json.Marshal(clipText(string(raw), n))
	return out
}
