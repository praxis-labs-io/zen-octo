package app_test

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/config"
	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/app"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// fakeSearcher answers the one call the root model makes.
type fakeSearcher struct {
	prs  []gh.PullRequest
	rate gh.RateLimit
	err  error

	gotQuery    string
	gotLimit    int
	calls       int
	gotDeadline time.Time
	hadDeadline bool
}

func (f *fakeSearcher) SearchPullRequests(ctx context.Context, query string, limit int) (gh.SearchResult, error) {
	f.calls++
	f.gotQuery, f.gotLimit = query, limit
	f.gotDeadline, f.hadDeadline = ctx.Deadline()

	if f.err != nil {
		return gh.SearchResult{}, f.err
	}
	return gh.SearchResult{PullRequests: f.prs, RateLimit: f.rate}, nil
}

// querySearcher answers per query, so a test can hold one section's fetch back
// and let another land first.
type querySearcher struct {
	results map[string]gh.SearchResult
	errs    map[string]error
}

func (f *querySearcher) SearchPullRequests(_ context.Context, query string, _ int) (gh.SearchResult, error) {
	if err, ok := f.errs[query]; ok {
		return gh.SearchResult{}, err
	}
	return f.results[query], nil
}

func testConfig() *config.Config {
	return &config.Config{
		PRSections: []config.Section{
			{Title: "My PRs", Filters: "is:open is:pr author:@me"},
			{Title: "Needs My Review", Filters: "is:open is:pr review-requested:@me"},
		},
		Defaults: config.Defaults{PRsLimit: 20, IssuesLimit: 20},
		Theme:    "rose-pine-moon",
	}
}

func samplePRs() []gh.PullRequest {
	return []gh.PullRequest{
		{
			ID: "PR_412", Number: 412, Title: "Fix auth retry", Repository: "zen-octo/zen-octo",
			Author: gh.Actor{Login: "drucial"}, State: gh.PRStateOpen, BaseRefName: "main",
			HeadRefName: "fix-auth", Additions: 42, Deletions: 7, ChangedFiles: 3,
			Checks: gh.CheckStateSuccess, UpdatedAt: time.Now().Add(-2 * time.Hour),
		},
		{
			ID: "PR_408", Number: 408, Title: "Bump deps", Repository: "zen-octo/zen-octo",
			Author: gh.Actor{Login: "drucial"}, State: gh.PRStateOpen, IsDraft: true,
			Checks: gh.CheckStateFailure, UpdatedAt: time.Now().Add(-30 * time.Hour),
		},
	}
}

// drive runs the model's Init command and then applies the given messages,
// following every command each one produces.
func drive(t *testing.T, m tea.Model, msgs ...tea.Msg) tea.Model {
	t.Helper()

	m = settle(m, immediate(m.Init())...)
	return settle(m, msgs...)
}

// loaded is the common setup: a sized terminal with the first fetch settled.
func loaded(t *testing.T, client *fakeSearcher, width, height int) tea.Model {
	t.Helper()
	return drive(t, app.New(testConfig(), client), tea.WindowSizeMsg{Width: width, Height: height})
}

// settle applies messages and keeps going until the model stops producing any.
// A key press can be three hops from its effect: r yields a RefreshMsg, which
// yields a fetch, which yields the rows.
func settle(m tea.Model, msgs ...tea.Msg) tea.Model {
	queue := append([]tea.Msg(nil), msgs...)
	for range 64 {
		if len(queue) == 0 {
			break
		}

		var cmd tea.Cmd
		m, cmd = m.Update(queue[0])
		queue = append(queue[1:], immediate(cmd)...)
	}
	return m
}

// immediate runs a command and returns its messages, unpacking batches. A
// command that does not answer at once is a timer, and following the spinner or
// a toast expiry would just make the suite sleep, so those get dropped.
func immediate(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}

	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()

	select {
	case msg := <-done:
		if batch, ok := msg.(tea.BatchMsg); ok {
			var out []tea.Msg
			for _, sub := range batch {
				out = append(out, immediate(sub)...)
			}
			return out
		}
		if msg == nil {
			return nil
		}
		return []tea.Msg{msg}
	case <-time.After(20 * time.Millisecond):
		return nil
	}
}

func render(t *testing.T, m tea.Model) string {
	t.Helper()
	return m.View().Content
}

func press(m tea.Model, keys ...string) tea.Model {
	for _, k := range keys {
		m = settle(m, keyMsg(k))
	}
	return m
}

func keyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	default:
		return tea.KeyPressMsg{Code: rune(k[0]), Text: k}
	}
}

func TestRendersFetchedPullRequests(t *testing.T) {
	m := loaded(t, &fakeSearcher{prs: samplePRs()}, 120, 40)

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

	if client.gotQuery != "is:open is:pr author:@me" {
		t.Errorf("query = %q, want the section filters unmodified", client.gotQuery)
	}
	if client.gotLimit != 20 {
		t.Errorf("limit = %d, want 20", client.gotLimit)
	}
}

func TestRendersEmptySectionWithoutClaimingAnError(t *testing.T) {
	out := render(t, loaded(t, &fakeSearcher{prs: nil}, 120, 40))

	if !strings.Contains(out, "Nothing matches this section.") {
		t.Errorf("view = %q, want the empty-section message", out)
	}
	if strings.Contains(out, "Failed to load") {
		t.Error("view claims a failure for an empty result")
	}
}

func TestRendersTheFixCommandWhenAScopeIsMissing(t *testing.T) {
	client := &fakeSearcher{err: errors.New("HTTP 403\nYour gh token is missing the workflow scope. Run:\n  gh auth refresh -s workflow")}
	out := render(t, loaded(t, client, 120, 40))

	if !strings.Contains(out, "Failed to load") {
		t.Errorf("view = %q, want the failure label", out)
	}
	if !strings.Contains(out, "gh auth refresh -s workflow") {
		t.Errorf("view = %q, want the fix command carried through to the screen", out)
	}
}

func TestCursorMovesAndStopsAtTheEnds(t *testing.T) {
	base := loaded(t, &fakeSearcher{prs: samplePRs()}, 120, 40)

	// Two rows, so one "j" lands on the second and a second "j" holds there.
	moved := press(base, "j", "j")
	if !strings.Contains(selectedLine(t, moved), "#408") {
		t.Errorf("selection = %q, want it clamped to the last row", selectedLine(t, moved))
	}

	back := press(moved, "k", "k", "k")
	if !strings.Contains(selectedLine(t, back), "#412") {
		t.Errorf("selection = %q, want it clamped to the first row", selectedLine(t, back))
	}
}

func TestRefreshRefetches(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := loaded(t, client, 120, 40)

	if client.calls != 1 {
		t.Fatalf("calls = %d after load, want 1", client.calls)
	}

	settle(m, keyMsg("r"))
	if client.calls != 2 {
		t.Errorf("calls = %d after refresh, want 2", client.calls)
	}
}

func TestQuitKeys(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{name: "q", key: tea.KeyPressMsg{Code: 'q', Text: "q"}},
		{name: "ctrl+c", key: tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := drive(t, app.New(testConfig(), &fakeSearcher{prs: samplePRs()}))

			_, cmd := m.Update(tt.key)
			if cmd == nil {
				t.Fatal("produced no command, want quit")
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Error("did not produce a QuitMsg")
			}
		})
	}
}

func TestSelectionPaintsEveryCellNotJustTheFirst(t *testing.T) {
	m := loaded(t, &fakeSearcher{prs: samplePRs()}, 120, 40)

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
	m := loaded(t, client, 120, 40)

	if !strings.Contains(render(t, m), "Failed to load") {
		t.Fatal("setup: expected the first fetch to render a failure")
	}

	// The retry is in flight: the old error must be gone and the spinner up,
	// or the user cannot tell that r did anything.
	next, _ := m.Update(keyMsg("r"))
	out := render(t, next)
	if strings.Contains(out, "boom, the first attempt failed") {
		t.Errorf("view still shows the previous error during the retry\n%s", out)
	}
	if !strings.Contains(out, "Loading pull requests") {
		t.Errorf("view = %q, want the loading state during the retry", out)
	}
}

func TestRefreshKeepsTheCursorOnTheSamePullRequest(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := press(loaded(t, client, 120, 40), "j") // now on #408

	// A new PR lands at the top, pushing #408 down a row.
	client.prs = append([]gh.PullRequest{{
		ID: "PR_NEW", Number: 500, Title: "Brand new", Repository: "zen-octo/zen-octo",
		State: gh.PRStateOpen, UpdatedAt: time.Now(),
	}}, samplePRs()...)
	m = settle(m, keyMsg("r"))

	if got := selectedLine(t, m); !strings.Contains(got, "#408") {
		t.Errorf("selection = %q, want it still on #408 after the refresh", got)
	}
}

func TestRefreshClampsTheCursorWhenTheRowIsGone(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := press(loaded(t, client, 120, 40), "j") // now on #408

	client.prs = samplePRs()[:1] // #408 merged and dropped out of the section
	m = settle(m, keyMsg("r"))

	if got := selectedLine(t, m); !strings.Contains(got, "#412") {
		t.Errorf("selection = %q, want it clamped onto the remaining row", got)
	}
}

// This is the property the old chromeLines constant was holding by hand, and
// getting wrong. Nothing derives a height from a count of chrome lines now, so
// it should hold at any size and on either screen.
func TestTheFrameNeverExceedsTheTerminal(t *testing.T) {
	sizes := []struct{ width, height int }{
		{width: 200, height: 60},
		{width: 120, height: 40},
		{width: 120, height: 14},
		{width: 90, height: 24},
		{width: 60, height: 8},
		{width: 40, height: 5},
		// Below this a pane is borders and nothing else, and one line has to be
		// the status bar rather than a config notice.
		{width: 60, height: 3},
		{width: 40, height: 2},
	}

	for _, size := range sizes {
		name := fmt.Sprintf("%dx%d", size.width, size.height)
		t.Run(name, func(t *testing.T) {
			m := loaded(t, &fakeSearcher{prs: manyPRs(60)}, size.width, size.height)

			for _, stage := range []struct {
				what string
				m    tea.Model
			}{
				{what: "list", m: m},
				{what: "detail", m: press(m, "enter")},
				{what: "help", m: press(m, "?")},
			} {
				out := render(t, stage.m)
				lines := strings.Split(out, "\n")
				if len(lines) > size.height {
					t.Errorf("%s frame is %d lines, want no more than %d", stage.what, len(lines), size.height)
				}
				for i, line := range lines {
					if w := lipgloss.Width(line); w > size.width {
						t.Errorf("%s frame line %d is %d cells wide, want no more than %d", stage.what, i, w, size.width)
					}
				}
			}
		})
	}
}

func TestCursorStaysVisibleWhenScrollingPastTheFold(t *testing.T) {
	m := loaded(t, &fakeSearcher{prs: manyPRs(60)}, 120, 14)

	for range 30 {
		m = press(m, "j")
	}

	if selectedLine(t, m) == "" {
		t.Fatal("the selected row is not in the rendered frame after scrolling")
	}
	if got := selectedLine(t, m); !strings.Contains(got, "#30") {
		t.Errorf("selection = %q, want the 31st row (#30)", got)
	}
}

func TestEnterOpensTheDetailAndEscapeComesBack(t *testing.T) {
	m := press(loaded(t, &fakeSearcher{prs: samplePRs()}, 160, 40), "j") // on #408

	detail := press(m, "enter")
	out := render(t, detail)
	if !strings.Contains(out, "Conversation") {
		t.Errorf("detail = %q, want the conversation tab strip", out)
	}
	if !strings.Contains(out, "Bump deps #408") {
		t.Errorf("detail = %q, want the selected pull request", out)
	}

	back := press(detail, "esc")
	if !strings.Contains(render(t, back), "Fix auth retry") {
		t.Error("escape did not return to the list")
	}
	if got := selectedLine(t, back); !strings.Contains(got, "#408") {
		t.Errorf("selection = %q, want the same row still selected", got)
	}
}

func TestTheRailCollapsesOnANarrowTerminal(t *testing.T) {
	// "Branch" is a rail section heading. The pane title reads "Details", which
	// also appears in the status bar hints, so it cannot tell the two apart.
	const railOnly = "Branch"

	wide := press(loaded(t, &fakeSearcher{prs: samplePRs()}, 160, 40), "enter")
	if !strings.Contains(render(t, wide), railOnly) {
		t.Error("the rail is missing on a wide terminal")
	}

	narrow := press(loaded(t, &fakeSearcher{prs: samplePRs()}, 100, 40), "enter")
	if strings.Contains(render(t, narrow), railOnly) {
		t.Error("the rail is still on screen at 100 columns, want it collapsed")
	}

	// The toggle overrides the automatic decision in either direction.
	if !strings.Contains(render(t, press(narrow, "d")), railOnly) {
		t.Error("the toggle did not bring the rail back on a narrow terminal")
	}
	if strings.Contains(render(t, press(wide, "d")), railOnly) {
		t.Error("the toggle did not hide the rail on a wide terminal")
	}
}

func TestHelpOverlaysTheScreenAndDismisses(t *testing.T) {
	m := loaded(t, &fakeSearcher{prs: samplePRs()}, 120, 40)

	open := press(m, "?")
	out := render(t, open)
	if !strings.Contains(out, "Keys") {
		t.Errorf("help = %q, want the overlay title", out)
	}
	// The help renders from the binding declarations, so a description only
	// reaches the screen if it was declared alongside its key.
	if !strings.Contains(out, "half page down") {
		t.Errorf("help = %q, want a binding's declared description", out)
	}

	if strings.Contains(render(t, press(open, "?")), "half page down") {
		t.Error("pressing ? again did not dismiss the help")
	}
}

// The help bubble sizes columns from their contents and never wraps, so a set
// one column too wide used to get sheared by the overlay: the modal lost its
// right border and its rows ran to the frame edge.
func TestHelpReflowsRatherThanLosingItsBorder(t *testing.T) {
	for _, width := range []int{160, 100, 80, 60} {
		t.Run(fmt.Sprintf("%d", width), func(t *testing.T) {
			out := render(t, press(loaded(t, &fakeSearcher{prs: samplePRs()}, width, 24), "?"))

			var top string
			for _, line := range strings.Split(out, "\n") {
				if strings.Contains(line, "Keys") {
					top = line
					break
				}
			}
			if top == "" {
				t.Fatal("the help overlay is not on screen")
			}
			if !strings.Contains(top, "╮") {
				t.Errorf("the modal lost its top-right corner, so it was sheared rather than reflowed\n%s", top)
			}
			if lipgloss.Width(top) != width {
				t.Errorf("the composited line is %d cells, want %d", lipgloss.Width(top), width)
			}
		})
	}
}

// Help owns the keyboard while it is up. Otherwise a stray j scrolls the screen
// under the thing covering it.
func TestHelpSwallowsScreenKeys(t *testing.T) {
	m := press(loaded(t, &fakeSearcher{prs: samplePRs()}, 120, 40), "?")

	if _, cmd := m.Update(keyMsg("enter")); cmd != nil {
		t.Error("enter reached the list while help was up")
	}
	if !strings.Contains(render(t, press(m, "esc")), "Fix auth retry") {
		t.Error("escape did not dismiss the help")
	}
}

func TestTabSwitchesSectionAndRefetches(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := loaded(t, client, 120, 40)

	m = settle(m, keyMsg("tab"))

	if client.gotQuery != "is:open is:pr review-requested:@me" {
		t.Errorf("query = %q, want the second section's filters", client.gotQuery)
	}
	if !strings.Contains(render(t, m), "Needs My Review") {
		t.Error("the second section is not on screen")
	}
}

func TestTheStatusBarCarriesTheRateLimit(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs(), rate: gh.RateLimit{Limit: 5000, Cost: 1, Remaining: 4821}}

	if out := render(t, loaded(t, client, 120, 40)); !strings.Contains(out, "4821") {
		t.Errorf("view = %q, want the remaining budget in the status bar", out)
	}
}

// A refresh that returns identical rows moves nothing on screen. The toast is
// the only signal that anything happened.
func TestRefreshAnnouncesItselfButTheFirstLoadDoesNot(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := loaded(t, client, 120, 40)

	if strings.Contains(render(t, m), "Loaded 2 pull requests") {
		t.Error("the first load raised a toast, want the rows to speak for themselves")
	}

	if out := render(t, settle(m, keyMsg("r"))); !strings.Contains(out, "Loaded 2 pull requests") {
		t.Errorf("view = %q, want the refresh to report what came back", out)
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

	m := drive(t, app.New(cfg, &fakeSearcher{prs: samplePRs()}), tea.WindowSizeMsg{Width: 120, Height: 40})

	out := render(t, m)
	if !strings.Contains(out, "rose-pine-dawn") {
		t.Errorf("view = %q, want it to name the theme it did not recognise", out)
	}
	if !strings.Contains(out, "rose-pine-moon") {
		t.Errorf("view = %q, want it to name the theme it fell back to", out)
	}
}

func TestKnownThemeShowsNoNotice(t *testing.T) {
	if strings.Contains(render(t, loaded(t, &fakeSearcher{prs: samplePRs()}, 120, 40)), "Unknown theme") {
		t.Error("a valid theme produced a notice")
	}
}

// Scrolling has to follow the cursor by a row. viewport.EnsureVisible acts only
// once the cursor is already outside the window and then puts it on the top
// line, which turned one press into a page jump and the next ten into nothing.
func TestScrollingFollowsTheCursorARowAtATime(t *testing.T) {
	m := loaded(t, &fakeSearcher{prs: manyPRs(60)}, 120, 14)

	// Eleven lines of content: a group header and then two lines a row, so rows
	// 0 through 4 fit and the fifth press is the first that has to move.
	for range 4 {
		m = press(m, "j")
	}
	if !strings.Contains(render(t, m), "#0 ") {
		t.Fatal("the window moved before the cursor reached the fold")
	}

	out := render(t, press(m, "j"))
	if strings.Contains(out, "#0 ") {
		t.Error("the window did not move once the cursor passed the fold")
	}
	if !strings.Contains(out, "#1 ") {
		t.Error("the window moved by more than one row")
	}
}

// The old root model clamped the scroll on every resize. Losing that put the
// selection below the fold, where the next enter opens a row nobody can see.
func TestShrinkingTheTerminalKeepsTheSelectionOnScreen(t *testing.T) {
	m := loaded(t, &fakeSearcher{prs: manyPRs(60)}, 120, 40)
	for range 30 {
		m = press(m, "j")
	}

	m = settle(m, tea.WindowSizeMsg{Width: 120, Height: 14})

	if got := selectedLine(t, m); !strings.Contains(got, "#30") {
		t.Errorf("selection = %q, want row 30 still on screen after the shrink", got)
	}
}

// These are declared and advertised in the help, so they need driving through
// the path a user takes rather than trusted because the binding exists.
func TestPageKeysMoveTheCursor(t *testing.T) {
	tests := []struct {
		name string
		keys []tea.KeyPressMsg
		want string
	}{
		{name: "page down", keys: []tea.KeyPressMsg{ctrl('f')}, want: "#5"},
		{name: "page down then back up", keys: []tea.KeyPressMsg{ctrl('f'), ctrl('b')}, want: "#0"},
		{name: "half page down", keys: []tea.KeyPressMsg{ctrl('d')}, want: "#2"},
		{name: "half page down twice, half back", keys: []tea.KeyPressMsg{ctrl('d'), ctrl('d'), ctrl('u')}, want: "#2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := loaded(t, &fakeSearcher{prs: manyPRs(60)}, 120, 14)
			for _, k := range tt.keys {
				m = settle(m, k)
			}

			if got := selectedLine(t, m); !strings.Contains(got, tt.want+" ") {
				t.Errorf("selection = %q, want %s", got, tt.want)
			}
		})
	}
}

// The success path drops a fetch the user has moved on from. The failure path
// has to do the same, or a timeout from an abandoned section replaces rows that
// loaded fine, with no fetch in flight and no spinner to explain it.
func TestAStaleFailureDoesNotReplaceTheSectionOnScreen(t *testing.T) {
	client := &querySearcher{
		errs:    map[string]error{"is:open is:pr author:@me": errors.New("context deadline exceeded")},
		results: map[string]gh.SearchResult{"is:open is:pr review-requested:@me": {PullRequests: samplePRs()}},
	}

	var m tea.Model = app.New(testConfig(), client)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// The first section's fetch, held rather than delivered.
	stale := m.Init()

	m = settle(m, keyMsg("tab"))
	if !strings.Contains(render(t, m), "Fix auth retry") {
		t.Fatal("setup: the second section did not load")
	}

	m = settle(m, immediate(stale)...)

	if strings.Contains(render(t, m), "Failed to load") {
		t.Error("a failure from the section the user left took over the one on screen")
	}
	if !strings.Contains(render(t, m), "Fix auth retry") {
		t.Error("the loaded rows are gone")
	}
}

// The tick chain re-arms from the list's own Update. Delegating by focus killed
// it the moment the detail opened over a fetch in flight, and coming back
// showed a spinner frozen on one frame.
func TestTheSpinnerKeepsTickingBehindTheDetailScreen(t *testing.T) {
	m := loaded(t, &fakeSearcher{prs: samplePRs()}, 120, 40)

	// Holding the refresh command back leaves the list loading, which is the
	// state the chain has to survive.
	m, _ = m.Update(keyMsg("r"))

	var open tea.Cmd
	m, open = m.Update(keyMsg("enter"))
	m = settle(m, immediate(open)...)

	if _, cmd := m.Update(spinner.TickMsg{}); cmd == nil {
		t.Error("the tick produced no follow-up, so the spinner freezes mid-fetch")
	}
}

func TestHidingTheRailSticksAcrossPullRequests(t *testing.T) {
	m := loaded(t, &fakeSearcher{prs: samplePRs()}, 160, 40)

	hidden := press(m, "enter", "d")
	if strings.Contains(render(t, hidden), "Branch") {
		t.Fatal("setup: d did not hide the rail")
	}

	if strings.Contains(render(t, press(hidden, "esc", "enter")), "Branch") {
		t.Error("reopening the pull request brought the rail back")
	}
}

// In the chrome grey the notice reads as decoration, which is the outcome it
// exists to prevent.
func TestTheConfigNoticeReadsAsAWarning(t *testing.T) {
	cfg := testConfig()
	cfg.Theme = "rose-pine-dawn"

	m := drive(t, app.New(cfg, &fakeSearcher{prs: samplePRs()}), tea.WindowSizeMsg{Width: 160, Height: 40})

	for _, line := range strings.Split(render(t, m), "\n") {
		if !strings.Contains(line, "Unknown theme") {
			continue
		}
		if !strings.Contains(line, fgSeq(theme.RosePineMoon.Warning)) {
			t.Error("the notice renders in the same grey as the key hints")
		}
		return
	}
	t.Fatal("the notice is not on screen")
}

func TestTheBudgetShowsAtZeroAndNotBeforeItIsKnown(t *testing.T) {
	spent := &fakeSearcher{prs: samplePRs(), rate: gh.RateLimit{Limit: 5000, Cost: 1, Remaining: 0}}
	if out := render(t, loaded(t, spent, 120, 40)); !strings.Contains(out, "◆ 0") {
		t.Errorf("view = %q, want the budget still readable once it is gone", out)
	}

	failed := &fakeSearcher{err: errors.New("boom")}
	if strings.Contains(render(t, loaded(t, failed, 120, 40)), "◆") {
		t.Error("the status bar shows a budget it has never been told")
	}
}

func ctrl(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl} }

// fgSeq is the SGR sequence that sets a foreground to the given color.
func fgSeq(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("38;2;%d;%d;%d", r>>8, g>>8, b>>8)
}

// manyPRs builds a run in a known order: one repo and one clock reading, so the
// sort's newest-first tiebreak cannot reorder rows by how long the loop took.
func manyPRs(n int) []gh.PullRequest {
	at := time.Now()

	prs := make([]gh.PullRequest, n)
	for i := range prs {
		prs[i] = gh.PullRequest{
			ID: fmt.Sprintf("PR_%d", i), Number: i, Title: fmt.Sprintf("Change %d", i),
			Repository: "zen-octo/zen-octo", State: gh.PRStateOpen, UpdatedAt: at,
		}
	}
	return prs
}

// selectionSeq is the SGR sequence that sets the selection background.
func selectionSeq() string {
	r, g, b, _ := theme.RosePineMoon.SelectedBackground.RGBA()
	return fmt.Sprintf("48;2;%d;%d;%d", r>>8, g>>8, b>>8)
}

// selectedLine returns every rendered line painted with the selection
// background, joined. Matching the exact color keeps this from picking up other
// styled chrome. A row is two lines, and its number is on the second, so
// returning only the first would answer half the question.
func selectedLine(t *testing.T, m tea.Model) string {
	t.Helper()

	var out []string
	for _, line := range strings.Split(render(t, m), "\n") {
		if strings.Contains(line, selectionSeq()) {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}
