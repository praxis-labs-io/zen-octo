package app_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/config"
	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/app"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// fakeSearcher answers the one call the root model makes.
type fakeSearcher struct {
	prs []gh.PullRequest
	err error

	gotQuery    string
	gotLimit    int
	calls       int
	gotDeadline time.Time
	hadDeadline bool
}

func (f *fakeSearcher) SearchPullRequests(ctx context.Context, query string, limit int) ([]gh.PullRequest, error) {
	f.calls++
	f.gotQuery, f.gotLimit = query, limit
	f.gotDeadline, f.hadDeadline = ctx.Deadline()
	return f.prs, f.err
}

func testConfig() *config.Config {
	return &config.Config{
		PRSections: []config.Section{{Title: "My PRs", Filters: "is:open author:@me"}},
		Defaults:   config.Defaults{PRsLimit: 20, IssuesLimit: 20},
		Theme:      "rose-pine-moon",
	}
}

func samplePRs() []gh.PullRequest {
	return []gh.PullRequest{
		{
			ID: "PR_412", Number: 412, Title: "Fix auth retry", Repository: "zen-octo/zen-octo",
			Author: gh.Actor{Login: "drucial"}, State: gh.PRStateOpen,
			Checks: gh.CheckStateSuccess, UpdatedAt: time.Now().Add(-2 * time.Hour),
		},
		{
			ID: "PR_408", Number: 408, Title: "Bump deps", Repository: "zen-octo/zen-octo",
			Author: gh.Actor{Login: "drucial"}, State: gh.PRStateOpen, IsDraft: true,
			Checks: gh.CheckStateFailure, UpdatedAt: time.Now().Add(-30 * time.Hour),
		},
	}
}

// drive runs the model's Init command, feeds every resulting message back, then
// applies the given messages in order. It returns the settled model.
func drive(t *testing.T, m tea.Model, msgs ...tea.Msg) tea.Model {
	t.Helper()

	for _, msg := range flatten(m.Init()) {
		m, _ = m.Update(msg)
	}
	for _, msg := range msgs {
		m, _ = m.Update(msg)
	}
	return m
}

// flatten runs a command and returns every message it produces, unpacking
// batches. Init batches the spinner tick alongside the fetch, so taking only
// the first message would drop the one that matters.
func flatten(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, sub := range batch {
			out = append(out, flatten(sub)...)
		}
		return out
	}
	if msg == nil {
		return nil
	}
	return []tea.Msg{msg}
}

func render(t *testing.T, m tea.Model) string {
	t.Helper()
	return m.View().Content
}

func TestRendersFetchedPullRequests(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := drive(t, app.New(testConfig(), client), tea.WindowSizeMsg{Width: 120, Height: 40})

	out := render(t, m)
	for _, want := range []string{"My PRs", "#412", "Fix auth retry", "zen-octo/zen-octo", "drucial", "#408"} {
		if !strings.Contains(out, want) {
			t.Errorf("view is missing %q\n%s", want, out)
		}
	}
}

func TestPassesSectionQueryAndLimitToTheClient(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	drive(t, app.New(testConfig(), client))

	if client.gotQuery != "is:open author:@me" {
		t.Errorf("query = %q, want the section filters unmodified", client.gotQuery)
	}
	if client.gotLimit != 20 {
		t.Errorf("limit = %d, want 20", client.gotLimit)
	}
}

func TestRendersEmptySectionWithoutClaimingAnError(t *testing.T) {
	client := &fakeSearcher{prs: nil}
	m := drive(t, app.New(testConfig(), client), tea.WindowSizeMsg{Width: 120, Height: 40})

	out := render(t, m)
	if !strings.Contains(out, "Nothing matches this section.") {
		t.Errorf("view = %q, want the empty-section message", out)
	}
	if strings.Contains(out, "Failed to load") {
		t.Error("view claims a failure for an empty result")
	}
}

func TestRendersTheFixCommandWhenAScopeIsMissing(t *testing.T) {
	client := &fakeSearcher{err: errors.New("HTTP 403\nYour gh token is missing the workflow scope. Run:\n  gh auth refresh -s workflow")}
	m := drive(t, app.New(testConfig(), client), tea.WindowSizeMsg{Width: 120, Height: 40})

	out := render(t, m)
	if !strings.Contains(out, "Failed to load") {
		t.Errorf("view = %q, want the failure label", out)
	}
	if !strings.Contains(out, "gh auth refresh -s workflow") {
		t.Errorf("view = %q, want the fix command carried through to the screen", out)
	}
}

func TestCursorMovesAndStopsAtTheEnds(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	base := drive(t, app.New(testConfig(), client), tea.WindowSizeMsg{Width: 120, Height: 40})

	// Two rows, so one "j" lands on the second and a second "j" holds there.
	moved := applyKeys(base, "j", "j")
	if !strings.Contains(selectedLine(t, moved), "#408") {
		t.Errorf("selection = %q, want it clamped to the last row", selectedLine(t, moved))
	}

	back := applyKeys(moved, "k", "k", "k")
	if !strings.Contains(selectedLine(t, back), "#412") {
		t.Errorf("selection = %q, want it clamped to the first row", selectedLine(t, back))
	}
}

func TestRefreshRefetches(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := drive(t, app.New(testConfig(), client), tea.WindowSizeMsg{Width: 120, Height: 40})

	if client.calls != 1 {
		t.Fatalf("calls = %d after load, want 1", client.calls)
	}

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("pressing r produced no command, want a refetch")
	}
	flatten(cmd)

	if client.calls != 2 {
		t.Errorf("calls = %d after refresh, want 2", client.calls)
	}
}

func TestQuitKeys(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{
		{Code: 'q', Text: "q"},
		{Code: 'c', Mod: tea.ModCtrl},
	} {
		client := &fakeSearcher{prs: samplePRs()}
		m := drive(t, app.New(testConfig(), client))

		_, cmd := m.Update(key)
		if cmd == nil {
			t.Fatalf("%q produced no command, want quit", key.String())
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("%q did not produce a QuitMsg", key.String())
		}
	}
}

func TestSelectionPaintsEveryCellNotJustTheFirst(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := drive(t, app.New(testConfig(), client), tea.WindowSizeMsg{Width: 120, Height: 40})

	line := selectedLine(t, m)
	if line == "" {
		t.Fatal("no row carries the selection background")
	}

	// Every cell terminates in a full SGR reset, which drops the background
	// too. So the background has to be re-set per cell: one occurrence means
	// only the first cell is highlighted and the row reads as unselected.
	if got := strings.Count(line, selectionSeq()); got < 7 {
		t.Errorf("selection background appears %d times, want one per cell (>=7)\n%q", got, line)
	}
}

func TestRefreshClearsTheStaleError(t *testing.T) {
	client := &fakeSearcher{err: errors.New("boom, the first attempt failed")}
	m := drive(t, app.New(testConfig(), client), tea.WindowSizeMsg{Width: 120, Height: 40})

	if !strings.Contains(render(t, m), "Failed to load") {
		t.Fatal("setup: expected the first fetch to render a failure")
	}

	// The retry is in flight: the old error must be gone and the spinner up,
	// or the user cannot tell that r did anything.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	out := render(t, next)
	if strings.Contains(out, "boom, the first attempt failed") {
		t.Errorf("view still shows the previous error during the retry\n%s", out)
	}
	if !strings.Contains(out, "Loading pull requests") {
		t.Errorf("view = %q, want the loading state during the retry", out)
	}
	flatten(cmd)
}

func TestRefreshKeepsTheCursorOnTheSamePullRequest(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := drive(t, app.New(testConfig(), client), tea.WindowSizeMsg{Width: 120, Height: 40})
	m = applyKeys(m, "j") // now on #408

	// A new PR lands at the top, pushing #408 down a row.
	shifted := append([]gh.PullRequest{{
		ID: "PR_NEW", Number: 500, Title: "Brand new", Repository: "zen-octo/zen-octo",
		State: gh.PRStateOpen, UpdatedAt: time.Now(),
	}}, samplePRs()...)
	m = refresh(t, m, client, shifted)

	if got := selectedLine(t, m); !strings.Contains(got, "#408") {
		t.Errorf("selection = %q, want it still on #408 after the refresh", got)
	}
}

func TestRefreshClampsTheCursorWhenTheRowIsGone(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := drive(t, app.New(testConfig(), client), tea.WindowSizeMsg{Width: 120, Height: 40})
	m = applyKeys(m, "j") // now on #408

	// #408 merged and dropped out of the section.
	m = refresh(t, m, client, samplePRs()[:1])

	if got := selectedLine(t, m); !strings.Contains(got, "#412") {
		t.Errorf("selection = %q, want it clamped onto the remaining row", got)
	}
}

func TestRowsAreClippedToTheTerminalHeight(t *testing.T) {
	client := &fakeSearcher{prs: manyPRs(60)}
	// 4 lines of chrome, so 10 rows fit.
	m := drive(t, app.New(testConfig(), client), tea.WindowSizeMsg{Width: 120, Height: 14})

	out := render(t, m)
	if lines := strings.Count(out, "\n") + 1; lines > 14 {
		t.Errorf("frame is %d lines, want no more than the terminal's 14\n%s", lines, out)
	}
	if !strings.Contains(out, "more below") {
		t.Error("footer does not say rows are hidden")
	}
	if !strings.Contains(out, "j/k move") {
		t.Error("footer fell off the frame")
	}
}

func TestCursorStaysVisibleWhenScrollingPastTheFold(t *testing.T) {
	client := &fakeSearcher{prs: manyPRs(60)}
	m := drive(t, app.New(testConfig(), client), tea.WindowSizeMsg{Width: 120, Height: 14})

	// Walk well past the bottom of the window.
	for range 30 {
		m = applyKeys(m, "j")
	}

	if selectedLine(t, m) == "" {
		t.Error("the selected row is not in the rendered frame after scrolling")
	}
	if got := selectedLine(t, m); !strings.Contains(got, "#30") {
		t.Errorf("selection = %q, want the 31st row (#30)", got)
	}
}

func TestFetchCarriesADeadline(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	drive(t, app.New(testConfig(), client))

	if !client.hadDeadline {
		t.Fatal("the fetch context has no deadline, so a hung request spins forever")
	}
	if until := time.Until(client.gotDeadline); until <= 0 || until > time.Minute {
		t.Errorf("deadline is %v away, want a sane positive bound", until)
	}
}

func TestUnknownThemeSaysSoRatherThanFallingBackSilently(t *testing.T) {
	cfg := testConfig()
	cfg.Theme = "rose-pine-dawn"

	client := &fakeSearcher{prs: samplePRs()}
	m := drive(t, app.New(cfg, client), tea.WindowSizeMsg{Width: 120, Height: 40})

	out := render(t, m)
	if !strings.Contains(out, "rose-pine-dawn") {
		t.Errorf("view = %q, want it to name the theme it did not recognise", out)
	}
	if !strings.Contains(out, "rose-pine-moon") {
		t.Errorf("view = %q, want it to name the theme it fell back to", out)
	}
}

func TestKnownThemeShowsNoNotice(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := drive(t, app.New(testConfig(), client), tea.WindowSizeMsg{Width: 120, Height: 40})

	if strings.Contains(render(t, m), "Unknown theme") {
		t.Error("a valid theme produced a notice")
	}
}

func manyPRs(n int) []gh.PullRequest {
	prs := make([]gh.PullRequest, n)
	for i := range prs {
		prs[i] = gh.PullRequest{
			ID: fmt.Sprintf("PR_%d", i), Number: i, Title: fmt.Sprintf("Change %d", i),
			Repository: "zen-octo/zen-octo", State: gh.PRStateOpen, UpdatedAt: time.Now(),
		}
	}
	return prs
}

func applyKeys(m tea.Model, keys ...string) tea.Model {
	for _, k := range keys {
		m, _ = m.Update(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
	}
	return m
}

// refresh presses r and delivers the resulting messages, with the fake now
// answering with next. This goes through the real refresh path rather than
// synthesising an internal message.
func refresh(t *testing.T, m tea.Model, client *fakeSearcher, next []gh.PullRequest) tea.Model {
	t.Helper()

	client.prs = next
	m, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	for _, msg := range flatten(cmd) {
		m, _ = m.Update(msg)
	}
	return m
}

// selectionSeq is the SGR sequence that sets the selection background.
func selectionSeq() string {
	r, g, b, _ := theme.RosePineMoon.SelectedBackground.RGBA()
	return fmt.Sprintf("48;2;%d;%d;%d", r>>8, g>>8, b>>8)
}

// selectedLine returns the rendered row painted with the selection background.
// Matching the exact color keeps this from picking up the header, which has a
// background of its own.
func selectedLine(t *testing.T, m tea.Model) string {
	t.Helper()

	for _, line := range strings.Split(render(t, m), "\n") {
		if strings.Contains(line, selectionSeq()) {
			return line
		}
	}
	return ""
}
