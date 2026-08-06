package comp

import (
	"regexp"
	"strings"
)

// Segment is one piece of a markdown body: either prose to render, or a
// <details> block folded to the line that stands for it.
type Segment struct {
	// Text is prose. Empty on a fold.
	Text string

	// Summary is the <summary> line, and Lines what is behind it. Both are zero
	// on prose, so Summary is what tells the two apart.
	Summary string
	Lines   int
}

// detailsBlock matches one <details> element and pulls out its summary. It is
// deliberately non-greedy, so the first </details> closes it: a nested pair
// would be mispaired, and GitHub does not write them.
var detailsBlock = regexp.MustCompile(`(?is)<details>\s*(?:<summary>(.*?)</summary>)?(.*?)</details>`)

// tagRun strips any HTML left inside a summary. GitHub wraps some of them in
// <b>, and the markers would otherwise read as text.
var tagRun = regexp.MustCompile(`<[^>]*>`)

// SplitDetails breaks a body into prose and folds. GitHub collapses <details>
// in the browser, and a bot review that pastes a sixty-row table of every file
// it looked at is the reason it does.
//
// The caller decides what a fold looks like, and whether to render the body
// whole instead. Nothing here styles anything.
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

// Folded reports whether a body has anything in it worth an expand key.
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

// group is one submatch, empty when the group did not participate.
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
