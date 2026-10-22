// Package console draws hikari's terminal output for people: a banner,
// start-up steps, a feed of lines, and a footer pinned to the bottom of the
// terminal. On a terminal it redraws the live part in place; elsewhere, such
// as in a pipe, it prints plain lines once.
package console

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/term"
)

var spinner = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

// Console writes to one terminal. Its methods are safe for concurrent use.
type Console struct {
	mu    sync.Mutex
	out   io.Writer
	live  bool
	color bool
	width func() int

	step   *Step
	footer func(frame int) []string
	frame  int
	shown  string // the live area on screen
	drawn  int    // its number of lines
}

// New returns a console on f. It redraws in place only when f is a terminal,
// and it uses color unless NO_COLOR is set.
func New(f *os.File) *Console {
	fd := int(f.Fd())
	tty := term.IsTerminal(fd) && os.Getenv("TERM") != "dumb"
	c := &Console{out: f, live: tty, color: tty && os.Getenv("NO_COLOR") == ""}
	c.width = func() int {
		if w, _, err := term.GetSize(fd); err == nil && w > 0 {
			return w
		}
		return 80
	}
	if c.live {
		go c.tick()
	}
	return c
}

// Styles. Each returns s unchanged when the console has no color.
func (c *Console) Bold(s string) string   { return c.paint("1", s) }
func (c *Console) Dim(s string) string    { return c.paint("2", s) }
func (c *Console) Green(s string) string  { return c.paint("32", s) }
func (c *Console) Yellow(s string) string { return c.paint("33", s) }
func (c *Console) Blue(s string) string   { return c.paint("34", s) }
func (c *Console) Red(s string) string    { return c.paint("31", s) }
func (c *Console) Link(s string) string   { return c.paint("1;4;36", s) }

func (c *Console) paint(code, s string) string {
	if !c.color {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// Color256 paints s with a color of the 256-color palette.
func (c *Console) Color256(n int, s string) string { return c.paint(fmt.Sprintf("1;38;5;%d", n), s) }

// Spinner returns the spinner glyph of a frame.
func Spinner(frame int) string { return string(spinner[frame%len(spinner)]) }

// Width returns the width of the terminal, or 80 without one.
func (c *Console) Width() int {
	if !c.live {
		return 80
	}
	return c.width()
}

// Println prints a line above the live area.
func (c *Console) Println(line string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.write(line + "\n")
}

// Warn prints a warning.
func (c *Console) Warn(msg string) { c.Println("  " + c.Yellow("!") + " " + msg) }

// Error prints an error.
func (c *Console) Error(msg string) { c.Println("  " + c.Red("✗") + " " + msg) }

// Step is a start-up step that shows a spinner until it ends.
type Step struct {
	c     *Console
	title string
	start time.Time
}

// Step starts a step. Only one step runs at a time.
func (c *Console) Step(title string) *Step {
	s := &Step{c: c, title: title, start: time.Now()}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.step = s
	if c.live {
		c.redraw()
	} else {
		c.write("  … " + title + "\n")
	}
	return s
}

// Done ends the step with a line that says what it did.
func (s *Step) Done(result string) {
	if d := time.Since(s.start); d >= time.Second {
		result += " " + s.c.Dim(fmt.Sprintf("%.1fs", d.Seconds()))
	}
	s.end("  " + s.c.Green("✓") + " " + result)
}

// Fail ends the step with an error.
func (s *Step) Fail(err error) {
	s.end("  " + s.c.Red("✗") + " " + s.title + ": " + err.Error())
}

func (s *Step) end(line string) {
	c := s.c
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.step == s {
		c.step = nil
	}
	c.write(line + "\n")
}

// SetFooter pins lines to the bottom of the terminal. render is called on
// every redraw with an animation frame. Without a terminal, the footer is
// printed once.
func (c *Console) SetFooter(render func(frame int) []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.live {
		c.write(strings.Join(render(0), "\n") + "\n")
		return
	}
	c.footer = render
	c.redraw()
}

// ClearFooter removes the footer.
func (c *Console) ClearFooter() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.footer = nil
	c.redraw()
}

// Refresh redraws the live area, for example after a footer's data changed.
func (c *Console) Refresh() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.redraw()
}

func (c *Console) tick() {
	for range time.Tick(100 * time.Millisecond) {
		c.mu.Lock()
		c.frame++
		c.redraw()
		c.mu.Unlock()
	}
}

// liveLines returns the live area: the running step, then the footer.
func (c *Console) liveLines() []string {
	var lines []string
	if s := c.step; s != nil {
		line := "  " + c.Blue(Spinner(c.frame)) + " " + s.title
		if d := time.Since(s.start); d >= time.Second {
			line += " " + c.Dim(fmt.Sprintf("%ds", int(d.Seconds())))
		}
		lines = append(lines, line)
	}
	if c.footer != nil {
		lines = append(lines, c.footer(c.frame)...)
	}
	width := c.width()
	for i, l := range lines {
		lines[i] = Truncate(l, width-1)
	}
	return lines
}

// redraw replaces the live area when it changed. The caller holds mu.
func (c *Console) redraw() {
	if !c.live {
		return
	}
	lines := c.liveLines()
	area := strings.Join(lines, "\n")
	if area == c.shown && len(lines) == c.drawn {
		return
	}
	var b strings.Builder
	c.clear(&b)
	c.draw(&b, lines, area)
	io.WriteString(c.out, b.String())
}

// write prints text above the live area. The caller holds mu.
func (c *Console) write(text string) {
	if !c.live {
		io.WriteString(c.out, text)
		return
	}
	var b strings.Builder
	c.clear(&b)
	b.WriteString(text)
	lines := c.liveLines()
	c.draw(&b, lines, strings.Join(lines, "\n"))
	io.WriteString(c.out, b.String())
}

func (c *Console) clear(b *strings.Builder) {
	if c.drawn > 0 {
		fmt.Fprintf(b, "\x1b[%dA\r\x1b[J", c.drawn)
	}
}

func (c *Console) draw(b *strings.Builder, lines []string, area string) {
	for _, l := range lines {
		b.WriteString(l + "\n")
	}
	c.shown, c.drawn = area, len(lines)
}

var colorCode = regexp.MustCompile(`^\x1b\[[0-9;]*[A-Za-z]`)

// Truncate cuts s to width visible characters, keeping its color codes. A
// live line must fit the terminal, or wrapping would break the redraw.
func Truncate(s string, width int) string {
	if utf8.RuneCountInString(Plain(s)) <= width {
		return s
	}
	var b strings.Builder
	visible := 0
	for i := 0; i < len(s) && visible < width-1; {
		if code := colorCode.FindString(s[i:]); code != "" {
			b.WriteString(code)
			i += len(code)
			continue
		}
		r, n := utf8.DecodeRuneInString(s[i:])
		b.WriteRune(r)
		visible++
		i += n
	}
	b.WriteString("…")
	if strings.Contains(s, "\x1b[") {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

// Plain removes the color codes from s.
func Plain(s string) string { return allColorCodes.ReplaceAllString(s, "") }

var allColorCodes = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
