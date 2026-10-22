package console

import (
	"strings"
	"testing"
)

func TestTruncate(t *testing.T) {
	red := "\x1b[31mhello world\x1b[0m"
	cases := []struct {
		in    string
		width int
		want  string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"hello world", 8, "hello w…"},
		{red, 20, red},
		{red, 8, "\x1b[31mhello w…\x1b[0m"},
		{"héllo wörld", 6, "héllo…"},
	}
	for _, c := range cases {
		if got := Truncate(c.in, c.width); got != c.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", c.in, c.width, got, c.want)
		}
	}
}

func TestPlainOutput(t *testing.T) {
	var b strings.Builder
	c := &Console{out: &b, width: func() int { return 80 }}
	c.Println("hello")
	s := c.Step("Building images")
	s.Done("Built images")
	c.Step("Starting").Fail(errString("boom"))
	c.SetFooter(func(int) []string { return []string{"Web UI  http://x"} })
	want := "hello\n  … Building images\n  ✓ Built images\n  … Starting\n  ✗ Starting: boom\nWeb UI  http://x\n"
	if b.String() != want {
		t.Errorf("plain output:\n%q\nwant\n%q", b.String(), want)
	}
}

func TestLiveRedraw(t *testing.T) {
	var b strings.Builder
	c := &Console{out: &b, live: true, width: func() int { return 80 }}
	c.SetFooter(func(int) []string { return []string{"footer 1", "footer 2"} })
	c.Println("event")
	got := b.String()
	want := "footer 1\nfooter 2\n" + "\x1b[2A\r\x1b[J" + "event\nfooter 1\nfooter 2\n"
	if got != want {
		t.Errorf("live output:\n%q\nwant\n%q", got, want)
	}
	b.Reset()
	c.Refresh()
	if b.Len() != 0 {
		t.Errorf("an unchanged live area must not be redrawn: %q", b.String())
	}
}

type errString string

func (e errString) Error() string { return string(e) }
