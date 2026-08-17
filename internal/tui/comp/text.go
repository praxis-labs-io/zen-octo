package comp

import (
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// RelativeTime renders a compact age: 34m, 5h, 12d, 3y. Anything in the future
// reads as "now" rather than a negative number.
func RelativeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}

	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h"
	case d < 365*24*time.Hour:
		return strconv.Itoa(int(d.Hours()/24)) + "d"
	default:
		return strconv.Itoa(int(d.Hours()/24/365)) + "y"
	}
}

// Plural is a count and its noun, with the s only when it is earned.
func Plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

// LongAgo renders an age in words, for a sentence. RelativeTime is the compact
// one, for a column that has a handful of cells to say it in.
func LongAgo(t time.Time) string {
	if t.IsZero() {
		return ""
	}

	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return Plural(int(d.Minutes()), "minute") + " ago"
	case d < 24*time.Hour:
		return Plural(int(d.Hours()), "hour") + " ago"
	case d < 365*24*time.Hour:
		return Plural(int(d.Hours()/24), "day") + " ago"
	default:
		return Plural(int(d.Hours()/24/365), "year") + " ago"
	}
}

// Handle is how a login is written wherever it names a person as a value: on
// the rail, and in the header's opened-by clause. GitHub's own case is kept.
func Handle(login string) string {
	if login == "" {
		return ""
	}
	return "@" + login
}

// Centered puts a block in the middle of a region. It moves the block as a
// unit rather than centring line by line, so a second line stays under the
// words it belongs to instead of floating to its own column.
//
// It pads above and never below, because every caller draws into something
// that pads its own content out to the rows it has, and trailing blanks here
// would be counted twice.
func Centered(block string, width, height int) string {
	lines := strings.Split(block, "\n")

	widest := 0
	for _, line := range lines {
		widest = max(widest, lipgloss.Width(line))
	}
	left := strings.Repeat(" ", max(0, (width-widest)/2))
	for i, line := range lines {
		lines[i] = left + line
	}

	above := make([]string, max(0, (height-len(lines))/2))
	return strings.Join(append(above, lines...), "\n")
}
