package comp

import (
	"strconv"
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

// Clip truncates to width, marking the cut. It always marks: a caller that
// wants the content left alone when it fits checks the width first.
//
// A single column has room for the mark and nothing else, and MaxWidth(0)
// means no limit rather than no room.
//
// The mark carries its own style, because the content is already rendered by
// the time it is cut: a caller that restyles the result afterwards passes a
// plain one, and a caller clipping a finished line passes the row's, or a
// selection background stops one cell short of the edge.
func Clip(content string, width int, mark lipgloss.Style) string {
	switch {
	case width <= 0:
		return ""
	case width == 1:
		return mark.Render("…")
	}
	return lipgloss.NewStyle().MaxWidth(width-1).Render(content) + mark.Render("…")
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
