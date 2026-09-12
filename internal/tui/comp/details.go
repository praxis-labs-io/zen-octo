package comp

import (
	"regexp"
	"strings"
)

// Segment is one piece of a markdown body: prose, or a <details> block folded to its summary.
type Segment struct {
	// Text is prose, empty on a fold.
	Text string

	// Summary is set only on a fold, with Lines counting what it hides.
	Summary string
	Lines   int
}

// Non-greedy, so nested <details> mispair; GitHub does not write them.
var detailsBlock = regexp.MustCompile(`(?is)<details>\s*(?:<summary>(.*?)</summary>)?(.*?)</details>`)

var tagRun = regexp.MustCompile(`<[^>]*>`)

// SplitDetails breaks body into prose and <details> folds, in order.
func SplitDetails(body string) []Segment {
	var out []Segment
	rest := body

	for {
		loc := detailsBlock.FindStringSubmatchIndex(rest)
		if loc == nil {
			break
		}

		out = appendText(out, rest[:loc[0]])
		out = append(out, Segment{
			Summary: summaryOf(group(rest, loc, 1)),
			Lines:   countLines(group(rest, loc, 2)),
		})
		rest = rest[loc[1]:]
	}

	return appendText(out, rest)
}

// Folded reports whether segments hold any fold.
func Folded(segments []Segment) bool {
	for _, s := range segments {
		if s.Summary != "" {
			return true
		}
	}
	return false
}

func appendText(out []Segment, text string) []Segment {
	if strings.TrimSpace(text) == "" {
		return out
	}
	return append(out, Segment{Text: text})
}

func group(s string, loc []int, n int) string {
	if loc[2*n] < 0 {
		return ""
	}
	return s[loc[2*n]:loc[2*n+1]]
}

func summaryOf(raw string) string {
	s := strings.TrimSpace(tagRun.ReplaceAllString(raw, ""))
	if s == "" {
		return "Details"
	}
	return s
}

func countLines(body string) int {
	n := 0
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}
