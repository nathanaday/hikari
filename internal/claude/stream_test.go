package claude

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nathanaday/hikari/internal/api"
)

// collector receives the entries that a tap posts.
type collector struct {
	entries []api.Activity
}

func (c *collector) post(a api.Activity) { c.entries = append(c.entries, a) }

// byID returns the last posted version of each entry, in order of first
// appearance.
func (c *collector) byID() []api.Activity {
	var order []int64
	last := map[int64]api.Activity{}
	for _, a := range c.entries {
		if _, ok := last[a.ID]; !ok {
			order = append(order, a.ID)
		}
		last[a.ID] = a
	}
	out := make([]api.Activity, len(order))
	for i, id := range order {
		out[i] = last[id]
	}
	return out
}

// stream is Claude Code's output for one turn with partial messages: a
// tool call with its result, a thinking block, and streamed text.
const stream = `
{"type":"system","subtype":"init","session_id":"s1"}
{"type":"stream_event","event":{"type":"message_start"}}
{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","name":"Read"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"file_path\": \"/work/a.py\"}"}}}
{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_1","name":"Read","input":{"file_path":"/work/a.py"}}]}}
{"type":"stream_event","event":{"type":"content_block_stop","index":0}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"1\tprint(1)\n","is_error":false}]}}
{"type":"stream_event","event":{"type":"message_stop"}}
{"type":"stream_event","event":{"type":"message_start"}}
{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":""}}}
{"type":"assistant","message":{"content":[{"type":"thinking","thinking":""}]}}
{"type":"stream_event","event":{"type":"content_block_stop","index":0}}
{"type":"stream_event","event":{"type":"content_block_start","index":1,"content_block":{"type":"text"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Should I "}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"add it?"}}}
{"type":"assistant","message":{"content":[{"type":"text","text":"Should I add it?"}]}}
{"type":"stream_event","event":{"type":"content_block_stop","index":1}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1","content":[{"type":"text","text":"late"}],"is_error":true}]}}
{"type":"result","subtype":"success","is_error":false,"result":"Should I add it?","total_cost_usd":0.03}
`

func TestTap(t *testing.T) {
	col := &collector{}
	tp := NewTap(col.post)

	tp.Begin("woken by user in the room")
	for _, line := range strings.Split(strings.TrimSpace(stream), "\n") {
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatal(err)
		}
		tp.Observe(e)
	}
	tp.End("Should I add it?", 0.03, 1200*time.Millisecond, nil)

	got := col.byID()
	kinds := make([]string, len(got))
	for i, a := range got {
		kinds[i] = a.Kind
	}
	if want := "turn tool thinking text done"; strings.Join(kinds, " ") != want {
		t.Fatalf("kinds = %v, want %s", kinds, want)
	}
	if got[0].Text != "woken by user in the room" || got[0].Turn != 1 {
		t.Errorf("turn entry: %+v", got[0])
	}
	tool := got[1]
	if tool.Tool != "Read" || string(tool.Input) != `{"file_path":"/work/a.py"}` || tool.Output != "1\tprint(1)\n" || !tool.Done || tool.Error {
		t.Errorf("tool entry: %+v", tool)
	}
	if !got[2].Done || got[2].Text != "" {
		t.Errorf("thinking entry: %+v", got[2])
	}
	if got[3].Text != "Should I add it?" || !got[3].Done {
		t.Errorf("text entry: %+v", got[3])
	}
	if got[4].Text != "Should I add it?" || got[4].Cost != 0.03 || got[4].Duration != 1200 || !got[4].Done {
		t.Errorf("done entry: %+v", got[4])
	}

	// The late, second result for toolu_1 is ignored: a result updates its
	// tool once.
	for _, a := range col.entries {
		if a.ID == tool.ID && a.Error {
			t.Error("a second tool result must not change the entry")
		}
	}
	// The tool was posted when it started, before its input was known.
	first := col.entries[1]
	if first.Kind != api.ActivityTool || first.Tool != "Read" || first.Input != nil || first.Done {
		t.Errorf("the tool must be posted as it starts: %+v", first)
	}

	tp.Begin("woken by lead directly")
	tp.End("", 0, 0, errString("claude: boom"))
	got = col.byID()
	last := got[len(got)-1]
	if last.Turn != 2 || !last.Error || last.Text != "claude: boom" {
		t.Errorf("failed turn: %+v", last)
	}
}

func TestTapWithoutPartialMessages(t *testing.T) {
	col := &collector{}
	tp := NewTap(col.post)
	tp.Begin("x")
	var e Event
	json.Unmarshal([]byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"Hi"},{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}]}}`), &e)
	tp.Observe(e)
	got := col.byID()
	if len(got) != 3 || got[1].Text != "Hi" || !got[1].Done || got[2].Tool != "Bash" || got[2].Done {
		t.Errorf("entries = %+v", got)
	}
}

func TestClip(t *testing.T) {
	if got := clipText("héllo", 2); got != "h…" {
		t.Errorf("clipText cut inside a character: %q", got)
	}
	long := strings.Repeat("x", 10)
	if got := string(clipJSON(json.RawMessage(`{"a":"`+long+`"}`), 8)); got != `"{\"a\":\"xx…"` {
		t.Errorf("clipJSON = %s", got)
	}
	if got := resultText(json.RawMessage(`[{"type":"text","text":"a"},{"type":"image"}]`)); got != "a\n[image]" {
		t.Errorf("resultText = %q", got)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
