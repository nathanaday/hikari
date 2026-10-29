package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/console"
)

// TestEntryLine checks rule FD7 of the feed spec: the terminal's line of
// each kind of entry, on a console without color.
func TestEntryLine(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	d := &dashboard{con: console.New(f)}
	at := time.Date(2026, 10, 28, 14, 2, 0, 0, time.UTC).In(time.Local).Format("15:04")
	cases := []struct {
		entry api.Entry
		want  string
	}{
		{api.Entry{Kind: api.EntryMessage, Who: "orders", Text: "Done:\n the field"}, "  " + at + "  orders → room  Done: the field"},
		{api.Entry{Kind: api.EntryMessage, Who: "user", To: "lead", Text: "plan it"}, "  " + at + "  user → lead  plan it"},
		{api.Entry{Kind: api.EntryTurn, Who: "orders", Text: "turn done · 3m"}, "  " + at + "  orders  turn done · 3m"},
		{api.Entry{Kind: api.EntryStatus, Who: "orders", Text: "error: boom", Error: true}, "  " + at + "  orders  error: boom"},
		{api.Entry{Kind: api.EntryModule, Who: "hikari", Text: "[[cancel]] orders set the part of orders to done"}, "  " + at + "  hikari  [[cancel]] orders set the part of orders to done"},
	}
	for _, c := range cases {
		c.entry.Time = time.Date(2026, 10, 28, 14, 2, 0, 0, time.UTC)
		if got := d.entryLine(c.entry); got != c.want {
			t.Errorf("entryLine(%+v)\n got %q\nwant %q", c.entry, got, c.want)
		}
	}
}
