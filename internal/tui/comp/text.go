package comp

import (
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// RelativeTime renders a compact age such as 34m, 5h, 12d, or 3y. A future time reads "now".
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

// Plural is n and noun, with an s unless n is one.
func Plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

// LongAgo renders an age in words, for a sentence.
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

// Handle is login prefixed with @, keeping GitHub's case, or empty for an empty login.
func Handle(login string) string {
	if login == "" {
		return ""
	}
	return "@" + login
}

// Centered pads block to the middle of a width by height region as one unit, padding above but never below.
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
