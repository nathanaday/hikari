package testmap

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

// Outcomes of a test.
const (
	Passed  = "passed"
	Failed  = "failed"
	Skipped = "skipped"
)

// Limits of one report (rule J6).
const (
	MaxCases       = 50000
	MaxReportBytes = 20 << 20
	maxMessage     = 200
)

// Case is one test of a report.
type Case struct {
	ID      string
	File    string
	Outcome string
	Message string
}

type junitCase struct {
	Classname string        `xml:"classname,attr"`
	Name      string        `xml:"name,attr"`
	File      string        `xml:"file,attr"`
	Failure   *junitProblem `xml:"failure"`
	Error     *junitProblem `xml:"error"`
	Skipped   *struct{}     `xml:"skipped"`
}

type junitProblem struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

// ParseReport reads the test cases of a JUnit XML report (rules J3 to J5).
// workDir is /workspace, so that an absolute path under it becomes a
// workspace path. disk tells whether a class name names a file of the
// workspace.
func ParseReport(data []byte, workDir string, disk Disk) ([]Case, error) {
	if len(data) > MaxReportBytes {
		return nil, fmt.Errorf("the report has %d bytes; at most %d", len(data), MaxReportBytes)
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	var (
		suiteFiles []string
		out        []Case
		index      = map[string]int{}
	)
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("the report is not valid XML: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "testsuite":
				suiteFiles = append(suiteFiles, attr(t, "file"))
			case "testcase":
				var jc junitCase
				if err := dec.DecodeElement(&jc, &t); err != nil {
					return nil, fmt.Errorf("the report is not valid JUnit XML: %w", err)
				}
				c := newCase(jc, nearest(suiteFiles), workDir, disk)
				if c.ID == "" {
					continue
				}
				i, seen := index[c.ID]
				if !seen {
					if len(out) == MaxCases {
						return nil, fmt.Errorf("the report has more than %d test cases", MaxCases)
					}
					index[c.ID] = len(out)
					out = append(out, c)
					continue
				}
				merge(&out[i], c)
			}
		case xml.EndElement:
			if t.Name.Local == "testsuite" && len(suiteFiles) > 0 {
				suiteFiles = suiteFiles[:len(suiteFiles)-1]
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("the report holds no test case; is it a JUnit XML report?")
	}
	return out, nil
}

// merge folds a second case with the same id into c (rule J5): it failed
// if any case failed, and it was skipped only if every case was skipped.
func merge(c *Case, next Case) {
	switch {
	case c.Outcome == Failed:
	case next.Outcome == Failed:
		c.Outcome, c.Message = Failed, next.Message
	case next.Outcome == Passed:
		c.Outcome = Passed
	}
	if c.File == "" {
		c.File = next.File
	}
}

func newCase(jc junitCase, suiteFile, workDir string, disk Disk) Case {
	name := strings.TrimSpace(jc.Name)
	class := strings.TrimSpace(jc.Classname)
	c := Case{ID: name, Outcome: Passed}
	if class != "" && name != "" {
		c.ID = class + " > " + name
	} else if name == "" {
		c.ID = class
	}
	switch {
	case jc.Failure != nil:
		c.Outcome, c.Message = Failed, message(jc.Failure)
	case jc.Error != nil:
		c.Outcome, c.Message = Failed, message(jc.Error)
	case jc.Skipped != nil:
		c.Outcome = Skipped
	}
	for _, f := range []string{jc.File, suiteFile} {
		if rel := relative(f, workDir); rel != "" {
			c.File = rel
			return c
		}
	}
	if rel := relative(class, workDir); rel != "" && disk.IsFile(rel) {
		c.File = rel
	}
	return c
}

// relative returns p relative to the workspace, or "" when p is empty or
// outside it (rule J4).
func relative(p, workDir string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if path.IsAbs(p) {
		prefix := strings.TrimSuffix(workDir, "/") + "/"
		if workDir == "" || !strings.HasPrefix(p, prefix) {
			return ""
		}
		p = strings.TrimPrefix(p, prefix)
	}
	p = path.Clean(p)
	if !local(p) || p == "." {
		return ""
	}
	return p
}

func message(p *junitProblem) string {
	text := p.Message
	if strings.TrimSpace(text) == "" {
		text = p.Text
	}
	for line := range strings.SplitSeq(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			if r := []rune(line); len(r) > maxMessage {
				return string(r[:maxMessage])
			}
			return line
		}
	}
	return ""
}

func nearest(files []string) string {
	for i := len(files) - 1; i >= 0; i-- {
		if files[i] != "" {
			return files[i]
		}
	}
	return ""
}

func attr(e xml.StartElement, name string) string {
	for _, a := range e.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}
