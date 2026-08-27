package app_test

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/app"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

// gapRun is the run of spaces the bar holds between the hints and whatever the
// right side is saying. A hint never contains two spaces, so the first run of
// them is where the line stops being hints.
var gapRun = regexp.MustCompile(`\s{2,}`)

// hintTokens is the hints the bar is carrying, one per entry. Reading them off
// the rendered frame rather than off the keymap is what lets these tests hold
// the shed to what a reader sees: a hint cut in half comes back as a token that
// matches nothing, where a length check would call the line fine.
func hintTokens(t *testing.T, m tea.Model) []string {
	t.Helper()
	bar := strings.TrimSpace(stripANSI(lastLine(render(t, m))))
	if bar == "" {
		return nil
	}
	return strings.Split(gapRun.Split(bar, 2)[0], " • ")
}

// The bar clipped its hints with a hard cut and no mark, so at eighty columns
// the line looked complete and was not. It sheds whole hints instead, from the
// right, and help is the one it never lets go of: it is the way to every key
// the room could not hold.
//
// The sweep runs in bands rather than across every width at once. The rail
// comes up unasked at 120 and the file column has a floor of its own, and a
// line that changes because the screen did is not the shed dropping anything.
func TestTheHintLineShedsWholeHintsAndKeepsHelp(t *testing.T) {
	tests := []struct {
		name     string
		open     []string
		from, to int
	}{
		{name: "list", from: 200, to: app.MinWidth},
		{name: "detail, rail up", open: []string{"enter"}, from: 200, to: 120},
		{name: "detail, no rail", open: []string{"enter"}, from: 119, to: app.MinWidth},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeSearcher{prs: samplePRs()}
			client.serveDetail("PR_412", "Caps the backoff at 30s.")
			m := press(loaded(t, client, tt.from, 40), tt.open...)

			// The widest frame in the band is the reference: nothing is shed
			// there, so what it names is the whole line in the order the shed
			// will eat it.
			full := hintTokens(t, m)
			if len(full) < 3 {
				t.Fatalf("the reference line is too short to shed: %v", full)
			}
			help := full[len(full)-1]
			if help != "? help" {
				t.Fatalf("the line ends in %q, want the help hint last so the shed can pin it", help)
			}
			rest := full[:len(full)-1]

			shed := len(rest)
			for width := tt.from; width >= tt.to; width-- {
				m = settle(m, tea.WindowSizeMsg{Width: width, Height: 40})
				got := hintTokens(t, m)

				if w := lipgloss.Width(stripANSI(lastLine(render(t, m)))); w > width {
					t.Fatalf("width %d: the bar is %d cells wide", width, w)
				}
				if len(got) == 0 || got[len(got)-1] != help {
					t.Fatalf("width %d: line = %v, want it to end in %q", width, got, help)
				}

				// A prefix and nothing else: every hint on the line is one the
				// reference named, whole, and they are shed from the right.
				kept := got[:len(got)-1]
				if len(kept) > len(rest) {
					t.Fatalf("width %d: line = %v, longer than the reference %v", width, got, full)
				}
				for i, tok := range kept {
					if tok != rest[i] {
						t.Fatalf("width %d: hint %d = %q, want %q: the shed is not dropping from the right", width, i, tok, rest[i])
					}
				}
				if len(kept) > shed {
					t.Fatalf("width %d: the line grew from %d hints to %d as the frame narrowed", width, shed, len(kept))
				}
				shed = len(kept)
			}

			// Only where the band reaches the floor. A band that stops at the
			// rail's breakpoint is wide enough throughout to name everything,
			// and that is the line doing its job rather than the shed idling.
			if tt.to == app.MinWidth && shed == len(rest) {
				t.Errorf("nothing was ever shed between %d columns and %d", tt.to, tt.from)
			}
		})
	}
}

// A section that has never answered is drawing a spinner and one that failed is
// drawing its error, and every key that acts on a row is refused over both.
// That is the screen where the line is the only thing left explaining the
// keyboard, and five of its nine hints were keys that did nothing.
func TestTheListNamesOnlyWhatABlockedSectionCanDo(t *testing.T) {
	client := &fakeSearcher{err: errors.New("502 Bad Gateway")}
	bar := strings.Join(hintTokens(t, loaded(t, client, 160, 40)), " • ")

	for _, want := range []string{"[/] tab", "s sync", "? help"} {
		if !strings.Contains(bar, want) {
			t.Errorf("status bar = %q, want %q on it", bar, want)
		}
	}
	for _, gone := range []string{"j/k move", "⏎ open", "/ search", "y copy link", "O browser"} {
		if strings.Contains(bar, gone) {
			t.Errorf("status bar = %q, want %q off it: the section is not showing its rows", bar, gone)
		}
	}
}

// The rows come back and so do the keys that act on them.
func TestTheListNamesTheRowKeysOnceTheRowsAreThere(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	bar := strings.Join(hintTokens(t, loaded(t, client, 160, 40)), " • ")

	for _, want := range []string{"j/k move", "⏎ open", "/ search", "y copy link", "O browser"} {
		if !strings.Contains(bar, want) {
			t.Errorf("status bar = %q, want %q on it", bar, want)
		}
	}
}

// The job log's search bar preempts every binding on the detail screen the way
// a compose box does, but it was not named as capturing, so the root went on
// eating q and ? out of a query the reader was typing them into. The bar named
// the whole Checks line while none of it answered.
func TestTheJobLogSearchTakesTheBarAndTheRootKeys(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "body")
	client.mu.Lock()
	d := client.details["PR_412"]
	d.Rollup = gh.CheckRollup{Checks: []gh.Check{{Name: "test", Workflow: "CI", State: gh.CheckStateSuccess, JobID: 9001}}}
	client.details["PR_412"] = d
	client.mu.Unlock()
	client.servedJob(9001, gh.Job{
		ID: 9001, Name: "test", State: gh.CheckStateSuccess,
		Steps: []gh.JobStep{{Number: 1, Name: "Run tests", State: gh.CheckStateSuccess}},
	}, "2026-08-19T14:00:00Z queued\n2026-08-19T14:00:01Z ok\n")

	m := settleJob(press(loaded(t, client, 160, 40), "enter", "]", "]"), d.Rollup.Checks[0], false)
	m = press(m, "/")

	bar := strings.Join(hintTokens(t, m), " • ")
	if bar != "⏎ apply • esc clear" {
		t.Errorf("status bar = %q, want the search bar's own two keys", bar)
	}

	// q and ? are letters in a query. The root used to quit on one and drop the
	// overlay over the screen on the other.
	m = settle(press(m, "q", "?"), prview.SearchSettleMsg{Query: "q?"})
	out := stripANSI(render(t, m))
	if strings.Contains(out, "Keys") && strings.Contains(out, "next tab") {
		t.Errorf("? opened the help overlay from inside the search bar:\n%s", out)
	}
	if !strings.Contains(out, "q?") {
		t.Errorf("the query did not take the letters:\n%s", out)
	}
}

// Esc clears a settled query before it will leave the screen, so a line saying
// "back" was naming the second press rather than the one about to be made.
func TestEscReadsAsClearingASettledJobLogSearch(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "body")
	client.mu.Lock()
	d := client.details["PR_412"]
	d.Rollup = gh.CheckRollup{Checks: []gh.Check{{Name: "test", Workflow: "CI", State: gh.CheckStateSuccess, JobID: 9001}}}
	client.details["PR_412"] = d
	client.mu.Unlock()
	client.servedJob(9001, gh.Job{
		ID: 9001, Name: "test", State: gh.CheckStateSuccess,
		Steps: []gh.JobStep{{Number: 1, Name: "Run tests", State: gh.CheckStateSuccess}},
	}, "2026-08-19T14:00:00Z ok\n")

	m := settleJob(press(loaded(t, client, 160, 40), "enter", "]", "]"), d.Rollup.Checks[0], false)
	m = settle(press(m, "/", "o", "k"), prview.SearchSettleMsg{Query: "ok"})
	m = press(m, "enter")

	bar := strings.Join(hintTokens(t, m), " • ")
	if !strings.Contains(bar, "esc clear search") {
		t.Errorf("status bar = %q, want esc named as clearing the search", bar)
	}
	if strings.Contains(bar, "esc back") {
		t.Errorf("status bar = %q, want esc off the line as a way out while a query stands", bar)
	}
}

// Walking the Checks column empties the selected job and refills it a debounce
// and a round trip later. A line derived from that fetch dropped the log keys
// on every step and put them back when the reader stopped, which on a held j is
// the line gone for the length of the walk.
func TestWalkingTheChecksColumnDoesNotBlinkTheLogKeys(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "body")
	client.mu.Lock()
	d := client.details["PR_412"]
	d.Rollup = gh.CheckRollup{Checks: []gh.Check{
		{Name: "test", Workflow: "CI", State: gh.CheckStateFailure, JobID: 9001},
		{Name: "lint", Workflow: "CI", State: gh.CheckStateFailure, JobID: 9002},
	}}
	client.details["PR_412"] = d
	client.mu.Unlock()
	for _, id := range []int64{9001, 9002} {
		client.servedJob(id, gh.Job{
			ID: id, Name: "test", State: gh.CheckStateFailure,
			Steps: []gh.JobStep{{Number: 1, Name: "Run", State: gh.CheckStateFailure}},
		}, "2026-08-19T14:00:00Z boom\n")
	}

	m := settleJob(press(loaded(t, client, 160, 40), "enter", "]", "]"), d.Rollup.Checks[0], false)
	landed := hintTokens(t, m)
	for _, want := range []string{"{/} block", "/ search log", "f first failure"} {
		if !slices.Contains(landed, want) {
			t.Fatalf("setup: line = %v, want %q on it once a job has landed", landed, want)
		}
	}

	// One step down the column, with the new job's fetch still out.
	if moved := hintTokens(t, press(m, "j")); !slices.Equal(moved, landed) {
		t.Errorf("the line changed while the next job was still on its way:\n before %v\n after  %v", landed, moved)
	}
}

// A toast wins the line and the hints shed around it, where the readout loses
// to them. The room the shed is given differs per path, and this is the one
// where getting the arithmetic wrong still renders: the line would come back
// too long and the bar would cut it the old way, through a word.
func TestTheHintsShedAroundAToastRatherThanBeingCut(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	full := hintTokens(t, loaded(t, client, 300, 40))
	help := full[len(full)-1]
	rest := full[:len(full)-1]

	m := settle(loaded(t, client, 70, 40), keyMsg("s"))
	bar := stripANSI(lastLine(render(t, m)))
	if !strings.Contains(bar, "Refreshed") {
		t.Fatalf("status bar = %q, want the toast whole on it", strings.TrimSpace(bar))
	}
	if w := lipgloss.Width(bar); w > 70 {
		t.Errorf("the bar is %d cells wide in a 70 column frame", w)
	}

	got := hintTokens(t, m)
	if len(got) == 0 || got[len(got)-1] != help {
		t.Fatalf("hints = %v, want them to end in %q beside the toast", got, help)
	}
	kept := got[:len(got)-1]
	if len(kept) >= len(rest) {
		t.Errorf("hints = %v, want fewer than the %d the frame holds without a toast", got, len(rest))
	}
	for i, tok := range kept {
		if tok != rest[i] {
			t.Fatalf("hint %d = %q, want %q: a hint was cut rather than shed", i, tok, rest[i])
		}
	}
}

// A picker, the merge form and a compose box each carry a hint line inside
// their own frame, so the bar spends nothing on keys that stopped answering
// when the modal opened. That decision moved into the screen, next to the
// widgets drawing what replaces it, so all three have to be held.
func TestTheBarGoesQuietForEveryBoxThatDrawsItsOwnHints(t *testing.T) {
	tests := []struct {
		name string
		open func(*testing.T, *fakeSearcher) tea.Model
	}{
		{name: "merge form", open: openMergeForm},
		{
			name: "compose box",
			open: func(t *testing.T, client *fakeSearcher) tea.Model {
				t.Helper()
				m := press(loaded(t, client, 160, 40), "enter", "c")
				if out := stripANSI(render(t, m)); !strings.Contains(out, "ctrl+e") {
					t.Fatalf("setup: c opened no compose box:\n%s", out)
				}
				return m
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeSearcher{prs: samplePRs()}
			client.serveDetail("PR_412", "Caps the backoff at 30s.")

			bar := stripANSI(lastLine(render(t, tt.open(t, client))))
			for _, gone := range []string{"j/k move", "esc back", "? help"} {
				if strings.Contains(bar, gone) {
					t.Errorf("status bar = %q, want %q off it: the box has that key", strings.TrimSpace(bar), gone)
				}
			}
		})
	}
}
