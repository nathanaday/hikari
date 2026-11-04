package tasks

import (
	"regexp"
	"strings"
	"time"
)

// Scorecard is what a task's log says of how the work went: when it
// started and finished, how often it was blocked, how often done was
// refused, how many reviews it carried, the window of each part, and the
// log's lines with their times, for the UI's timeline. The UI adds the
// turns and the cost of each partition agent inside its part's window.
type Scorecard struct {
	Started  time.Time `json:"started,omitzero"`
	Finished time.Time `json:"finished,omitzero"`
	Blocks   int       `json:"blocks"`
	Refusals int       `json:"refusals"`
	Reviews  int       `json:"reviews"`
	Parts    []Window  `json:"parts"`
	Log      []LogLine `json:"log"`
}

// Window is when one partition's part was started and when it was done;
// Finished is zero while the part is open.
type Window struct {
	Partition string    `json:"partition"`
	Started   time.Time `json:"started,omitzero"`
	Finished  time.Time `json:"finished,omitzero"`
}

// LogLine is one line of the task's log.
type LogLine struct {
	Time time.Time `json:"time,omitzero"`
	Text string    `json:"text"`
}

var (
	logLine    = regexp.MustCompile(`^- (\S+) (.*)$`)
	partChange = regexp.MustCompile(`^\S+ set the part of (\S+) to (\w+)`)
	scopeSet   = regexp.MustCompile(`^(\S+) set the scope to (\w+)`)
)

// scorecard reads the Log section of a task's body.
func scorecard(t *Task) *Scorecard {
	log, ok := Section(t.Body, SectionLog)
	if !ok {
		return nil
	}
	sc := &Scorecard{Parts: []Window{}, Log: []LogLine{}}
	windows := map[string]*Window{}
	for _, p := range t.Partitions {
		w := &Window{Partition: p}
		windows[p] = w
	}
	for _, raw := range strings.Split(log, "\n") {
		m := logLine.FindStringSubmatch(strings.TrimSpace(raw))
		if m == nil {
			continue
		}
		at, err := time.Parse(timeLayout, m[1])
		text := m[2]
		if err != nil {
			text = strings.TrimSpace(m[1] + " " + m[2])
		}
		sc.Log = append(sc.Log, LogLine{Time: at, Text: text})
		switch {
		case strings.Contains(text, "asked for done of ") && strings.Contains(text, ": refused"):
			sc.Refusals++
		}
		if strings.Contains(text, "reviewed: ") {
			sc.Reviews++
		}
		if pm := partChange.FindStringSubmatch(text); pm != nil {
			w, ok := windows[pm[1]]
			if !ok {
				w = &Window{Partition: pm[1]}
				windows[pm[1]] = w
				t.Partitions = append(t.Partitions, pm[1])
			}
			switch pm[2] {
			case Started:
				if w.Started.IsZero() {
					w.Started = at
				}
				w.Finished = time.Time{}
			case Done:
				w.Finished = at
			case Blocked:
				sc.Blocks++
			case Testing:
				w.Finished = time.Time{}
			}
			if sc.Started.IsZero() && pm[2] == Started {
				sc.Started = at
			}
			continue
		}
		if sm := scopeSet.FindStringSubmatch(text); sm != nil {
			switch sm[2] {
			case Started:
				if sc.Started.IsZero() {
					sc.Started = at
				}
			case Blocked:
				// hikari sets the scope blocked when a part is, which the
				// part's line already counted.
				if sm[1] != Hikari {
					sc.Blocks++
				}
			case Done:
				sc.Finished = at
			}
		}
	}
	for _, p := range t.Partitions {
		if w, ok := windows[p]; ok {
			sc.Parts = append(sc.Parts, *w)
		}
	}
	return sc
}
