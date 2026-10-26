package topoedit

import "strings"

// DiffLine is one line of a line diff (rule TA3): Op is " ", "+", or "-",
// and Old and New are its 1-based line numbers in each text, or 0.
type DiffLine struct {
	Op   string `json:"op"`
	Text string `json:"text"`
	Old  int    `json:"old,omitempty"`
	New  int    `json:"new,omitempty"`
}

// diffLines diffs two texts by their longest common subsequence of lines.
// Compose files hold hundreds of lines, so the quadratic table is small.
func diffLines(before, after string) []DiffLine {
	a, b := splitLines(before), splitLines(after)
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out []DiffLine
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && a[i] == b[j]:
			out = append(out, DiffLine{Op: " ", Text: a[i], Old: i + 1, New: j + 1})
			i, j = i+1, j+1
		case i < n && (j == m || lcs[i+1][j] >= lcs[i][j+1]):
			// A removed line comes before the line that replaces it.
			out = append(out, DiffLine{Op: "-", Text: a[i], Old: i + 1})
			i++
		default:
			out = append(out, DiffLine{Op: "+", Text: b[j], New: j + 1})
			j++
		}
	}
	return out
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// Diff is the line diff of a plan (rule TA3).
func Diff(before, after string) []DiffLine {
	if d := diffLines(before, after); d != nil {
		return d
	}
	return []DiffLine{}
}
