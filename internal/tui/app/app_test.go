package app_test

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/config"
	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/app"
	"github.com/zen-octo/zen-octo/internal/tui/list"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// fakeSearcher answers every section with the same rows. Sections fetch
// concurrently, so it locks: the -race detector is the point of the suite.
type fakeSearcher struct {
	rate gh.RateLimit
	err  error

	mu          sync.Mutex
	prs         []gh.PullRequest
	queries     []string
	opens       []string
	details     map[string]gh.PullRequestDetail
	detailErr   error
	gotLimit    int
	gotDeadline time.Time
	hadDeadline bool
}

func (f *fakeSearcher) SearchPullRequests(ctx context.Context, query string, limit int) (gh.SearchResult, error) {
	f.mu.Lock()
	f.queries = append(f.queries, query)
	f.gotLimit = limit
	f.gotDeadline, f.hadDeadline = ctx.Deadline()
	prs := f.prs
	f.mu.Unlock()

	if f.err != nil {
		return gh.SearchResult{}, f.err
	}
	return gh.SearchResult{PullRequests: prs, RateLimit: f.rate}, nil
}

// serve replaces what the next fetch returns.
func (f *fakeSearcher) serve(prs []gh.PullRequest) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prs = prs
}

func (f *fakeSearcher) asked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.queries)
}

func (f *fakeSearcher) calls() int { return len(f.asked()) }

// PullRequest answers with the row it was asked for, wrapped in whatever detail
// the test staged. Echoing the row matters: a detail response replaces what the
// list had, so a fake that returned a bare id would blank the header.
func (f *fakeSearcher) PullRequest(_ context.Context, id, _ string) (gh.DetailResult, error) {
	f.mu.Lock()
	f.opens = append(f.opens, id)
	detail, err, rows := f.details[id], f.detailErr, f.prs
	f.mu.Unlock()

	if err != nil {
		return gh.DetailResult{}, err
	}
	for _, pr := range rows {
		if pr.ID == id {
			detail.PullRequest = pr
		}
	}
	return gh.DetailResult{Detail: detail, RateLimit: f.rate}, nil
}

// serveDetail stages one pull request's conversation. It is per id because a
// response that lands after the reader moved on has to be told apart from the
// one they are looking at.
func (f *fakeSearcher) serveDetail(id, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.details == nil {
		f.details = make(map[string]gh.PullRequestDetail)
	}
	f.details[id] = gh.PullRequestDetail{Body: body}
}

// failDetails makes every open fail from here on.
func (f *fakeSearcher) failDetails(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.detailErr = err
}

// opened is the pull request ids the model asked for, in order.
func (f *fakeSearcher) opened() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.opens)
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

func (f *querySearcher) PullRequest(_ context.Context, id, _ string) (gh.DetailResult, error) {
	return gh.DetailResult{Detail: gh.PullRequestDetail{PullRequest: gh.PullRequest{ID: id}}}, nil
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

// responses runs a command and keeps the fetch results, dropping the spinner
// tick that rides in the same batch. It is what lets a test hold one section's
// answer back and let another land first.
func responses(cmd tea.Cmd) []tea.Msg {
	var out []tea.Msg
	for _, msg := range immediate(cmd) {
		if _, ok := msg.(spinner.TickMsg); !ok {
			out = append(out, msg)
		}
	}
	return out
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

// Every section fetches at startup, not just the one on screen. That is what
// lets a tab the user has not opened carry a count.
func TestEverySectionFetchesOnceWithItsOwnFilters(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	drive(t, app.New(testConfig(), client))

	want := []string{"is:open is:pr author:@me", "is:open is:pr review-requested:@me"}
	got := client.asked()
	slices.Sort(got)
	slices.Sort(want)

	if !slices.Equal(got, want) {
		t.Errorf("queries = %q, want each section's filters exactly once and unmodified", got)
	}
	if client.gotLimit != 20 {
		t.Errorf("limit = %d, want 20", client.gotLimit)
	}
}

func TestEveryTabCarriesItsOwnCount(t *testing.T) {
	client := &querySearcher{results: map[string]gh.SearchResult{
		"is:open is:pr author:@me":           {PullRequests: manyPRs(5)},
		"is:open is:pr review-requested:@me": {PullRequests: manyPRs(2)},
	}}

	top := strings.Split(stripANSI(render(t, drive(t, app.New(testConfig(), client), tea.WindowSizeMsg{Width: 160, Height: 40}))), "\n")[0]

	for _, want := range []string{"My PRs 5", "Needs My Review 2"} {
		if !strings.Contains(top, want) {
			t.Errorf("tab strip = %q, want %q in it", top, want)
		}
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
	if !strings.Contains(selectedText(t, moved), "#408") {
		t.Errorf("selection = %q, want it clamped to the last row", selectedText(t, moved))
	}

	back := press(moved, "k", "k", "k")
	if !strings.Contains(selectedText(t, back), "#412") {
		t.Errorf("selection = %q, want it clamped to the first row", selectedText(t, back))
	}
}

// The tab counts are on screen alongside the rows, so a refresh that left them
// as they were would be making only part of the frame true.
func TestRefreshRefetchesEverySection(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := loaded(t, client, 120, 40)

	if client.calls() != 2 {
		t.Fatalf("calls = %d after load, want one per section", client.calls())
	}

	settle(m, keyMsg("r"))
	if client.calls() != 4 {
		t.Errorf("calls = %d after refresh, want another per section", client.calls())
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

	// The retry is in flight: the fetch commands are held rather than run, so
	// the old error has to be gone and the spinner up, or the user cannot tell
	// that r did anything.
	next, _ := m.Update(list.RefreshMsg{})
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
	client.serve(append([]gh.PullRequest{{
		ID: "PR_NEW", Number: 500, Title: "Brand new", Repository: "zen-octo/zen-octo",
		State: gh.PRStateOpen, UpdatedAt: time.Now(),
	}}, samplePRs()...))
	m = settle(m, keyMsg("r"))

	if got := selectedText(t, m); !strings.Contains(got, "#408") {
		t.Errorf("selection = %q, want it still on #408 after the refresh", got)
	}
}

func TestRefreshClampsTheCursorWhenTheRowIsGone(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := press(loaded(t, client, 120, 40), "j") // now on #408

	client.serve(samplePRs()[:1]) // #408 merged and dropped out of the section
	m = settle(m, keyMsg("r"))

	if got := selectedText(t, m); !strings.Contains(got, "#412") {
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

	if selectedText(t, m) == "" {
		t.Fatal("the selected row is not in the rendered frame after scrolling")
	}
	if got := selectedText(t, m); !strings.Contains(got, "#30") {
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
	if !strings.Contains(stripANSI(out), "#408 Bump deps") {
		t.Errorf("detail = %q, want the selected pull request", out)
	}

	back := press(detail, "esc")
	if !strings.Contains(render(t, back), "Fix auth retry") {
		t.Error("escape did not return to the list")
	}
	if got := selectedText(t, back); !strings.Contains(got, "#408") {
		t.Errorf("selection = %q, want the same row still selected", got)
	}
}

func TestTheRailCollapsesOnANarrowTerminal(t *testing.T) {
	// "Author" is a rail section heading. The pane title reads "Details", which
	// also appears in the status bar hints, and the header spells the login with
	// an @ and no heading, so this tells the two columns apart.
	const railOnly = "Author"

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

// Every section is already held, so a tab switch is a move through state rather
// than a round trip. Refetching here is what made switching tabs feel slow.
func TestTabSwitchesSectionWithoutRefetching(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := loaded(t, client, 120, 40)
	before := client.calls()

	m = settle(m, keyMsg("tab"))

	if got := client.calls(); got != before {
		t.Errorf("calls went from %d to %d, want the switch to fetch nothing", before, got)
	}
	if !strings.Contains(render(t, m), "Fix auth retry") {
		t.Error("the second section's rows are not on screen")
	}
}

func TestTheStatusBarCarriesTheRateLimit(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs(), rate: gh.RateLimit{Limit: 5000, Cost: 1, Remaining: 4821}}

	if out := render(t, loaded(t, client, 120, 40)); !strings.Contains(out, "4821") {
		t.Errorf("view = %q, want the remaining budget in the status bar", out)
	}
}

// A refresh that returns identical rows moves nothing on screen. The toast is
// the only signal that anything happened, and one press earns one of them
// however many sections it fired at.
func TestRefreshAnnouncesItselfOnceButTheFirstLoadDoesNot(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := loaded(t, client, 120, 40)

	if strings.Contains(render(t, m), "Refreshed") {
		t.Error("the first load raised a toast, want the rows to speak for themselves")
	}

	out := render(t, settle(m, keyMsg("r")))
	if !strings.Contains(out, "Refreshed 2 sections") {
		t.Errorf("view = %q, want the refresh to report what came back", out)
	}
}

// The toast waits for the last section. Firing on the first arrival claims a
// refresh that two of the tabs on screen have not finished.
func TestTheRefreshToastWaitsForTheLastSection(t *testing.T) {
	m := loaded(t, &fakeSearcher{prs: samplePRs()}, 120, 40)

	// One section's response delivered, the rest held.
	next, cmd := m.Update(list.RefreshMsg{})
	landed := responses(cmd)
	if len(landed) < 2 {
		t.Fatalf("setup: the refresh produced %d responses, want one per section", len(landed))
	}

	next = settle(next, landed[0])
	if strings.Contains(render(t, next), "Refreshed") {
		t.Error("the toast fired while a section was still in flight")
	}

	if out := render(t, settle(next, landed[1:]...)); !strings.Contains(out, "Refreshed 2 sections") {
		t.Errorf("view = %q, want the toast once the last section landed", out)
	}
}

// store.Begin refuses a section already in flight, so a refresh does not always
// reach every tab. The toast counts what it fetched, not what it asked for, or
// it claims work it never did and failures it never caused.
func TestTheRefreshToastCountsOnlyTheSectionsItStarted(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	var m tea.Model = app.New(testConfig(), client)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Both startup fetches, held rather than delivered.
	initial := responses(m.Init())
	if len(initial) != 2 {
		t.Fatalf("setup: startup produced %d responses, want one per section", len(initial))
	}

	// One section home, the other still out, and then r: only the settled one
	// can be refetched.
	m, _ = m.Update(initial[0])
	m, cmd := m.Update(list.RefreshMsg{})

	m = settle(m, initial[1])
	m = settle(m, responses(cmd)...)

	if out := render(t, m); !strings.Contains(out, "Refreshed 1 section") {
		t.Errorf("view = %q, want the toast to count the one section the refresh started", out)
	}
}

func TestARefreshThatFailsSaysSo(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := loaded(t, client, 120, 40)

	client.err = errors.New("context deadline exceeded")
	out := render(t, settle(m, keyMsg("r")))

	if !strings.Contains(out, "Refresh failed") {
		t.Errorf("view = %q, want the toast to report the failure", out)
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

	prev := topRow(t, m)
	for i := range 30 {
		m = press(m, "j")

		top := topRow(t, m)
		if top < prev || top > prev+1 {
			t.Fatalf("press %d moved the window from row %d to row %d, want at most one row", i, prev, top)
		}
		prev = top
	}

	if prev == 0 {
		t.Error("the window never moved, so nothing here was tested")
	}
}

// topRow is the number of the first pull request with a title line on screen.
func topRow(t *testing.T, m tea.Model) int {
	t.Helper()

	for _, l := range strings.Split(stripANSI(render(t, m)), "\n") {
		i := strings.Index(l, "Change ")
		if i < 0 {
			continue
		}
		n, err := strconv.Atoi(strings.Fields(l[i+len("Change "):])[0])
		if err != nil {
			t.Fatalf("cannot read a row number out of %q: %v", l, err)
		}
		return n
	}
	t.Fatal("no pull request on screen")
	return 0
}

// The old root model clamped the scroll on every resize. Losing that put the
// selection below the fold, where the next enter opens a row nobody can see.
func TestShrinkingTheTerminalKeepsTheSelectionOnScreen(t *testing.T) {
	m := loaded(t, &fakeSearcher{prs: manyPRs(60)}, 120, 40)
	for range 30 {
		m = press(m, "j")
	}

	m = settle(m, tea.WindowSizeMsg{Width: 120, Height: 14})

	if got := selectedText(t, m); !strings.Contains(got, "#30") {
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
		{name: "page down", keys: []tea.KeyPressMsg{ctrl('f')}, want: "#3"},
		{name: "page down then back up", keys: []tea.KeyPressMsg{ctrl('f'), ctrl('b')}, want: "#0"},
		{name: "half page down", keys: []tea.KeyPressMsg{ctrl('d')}, want: "#1"},
		{name: "half page down twice, half back", keys: []tea.KeyPressMsg{ctrl('d'), ctrl('d'), ctrl('u')}, want: "#1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := loaded(t, &fakeSearcher{prs: manyPRs(60)}, 120, 14)
			for _, k := range tt.keys {
				m = settle(m, k)
			}

			if got := selectedText(t, m); !strings.Contains(got, tt.want+" ") {
				t.Errorf("selection = %q, want %s", got, tt.want)
			}
		})
	}
}

// A failure belongs to the section that had it. One section timing out used to
// take over whatever was on screen, leaving an error with no fetch in flight
// and no spinner to explain it.
func TestAFailedSectionIsTheOnlyOneShowingAnError(t *testing.T) {
	client := &querySearcher{
		errs:    map[string]error{"is:open is:pr author:@me": errors.New("context deadline exceeded")},
		results: map[string]gh.SearchResult{"is:open is:pr review-requested:@me": {PullRequests: samplePRs()}},
	}

	m := drive(t, app.New(testConfig(), client), tea.WindowSizeMsg{Width: 120, Height: 40})

	first := render(t, m)
	if !strings.Contains(first, "context deadline exceeded") {
		t.Fatalf("the failed section does not show its own error\n%s", first)
	}

	second := render(t, settle(m, keyMsg("tab")))
	if strings.Contains(second, "Failed to load") {
		t.Errorf("the failure followed the user to a section that loaded fine\n%s", second)
	}
	if !strings.Contains(second, "Fix auth retry") {
		t.Error("the loaded rows are not on screen")
	}
}

// Responses arrive in whatever order they finish, so the newest is not the
// truest. A budget that ticked back up mid-burst would be reading the wrong one.
func TestTheStatusBarCarriesTheLowestBudgetSeen(t *testing.T) {
	window := time.Now().Add(time.Hour)
	client := &querySearcher{results: map[string]gh.SearchResult{
		// The lower number lands first, so a status bar reading the newest
		// response rather than the lowest shows 4820 and reads as a budget that
		// went back up.
		"is:open is:pr author:@me": {
			PullRequests: samplePRs(),
			RateLimit:    gh.RateLimit{Limit: 5000, Remaining: 4819, ResetAt: window},
		},
		"is:open is:pr review-requested:@me": {
			RateLimit: gh.RateLimit{Limit: 5000, Remaining: 4820, ResetAt: window},
		},
	}}

	out := render(t, drive(t, app.New(testConfig(), client), tea.WindowSizeMsg{Width: 120, Height: 40}))
	if !strings.Contains(out, "4819") {
		t.Errorf("view = %q, want the lowest remaining across the responses", out)
	}
}

// The tick chain re-arms from the list's own Update. Delegating by focus killed
// it the moment the detail opened over a fetch in flight, and coming back
// showed a spinner frozen on one frame.
func TestTheSpinnerKeepsTickingBehindTheDetailScreen(t *testing.T) {
	m := loaded(t, &fakeSearcher{prs: samplePRs()}, 120, 40)

	// Holding the fetch commands back leaves the sections loading, which is the
	// state the chain has to survive.
	m, _ = m.Update(list.RefreshMsg{})

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

// selectedText is the selected row with its styling dropped, for assertions
// about what it says rather than how it is painted.
func selectedText(t *testing.T, m tea.Model) string {
	t.Helper()
	return stripANSI(selectedLine(t, m))
}

// stripANSI drops SGR sequences so an assertion can reason about the text. A
// cell ends in a reset, so "#5 " is not a substring of the styled frame even
// when the number is followed by its padding.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// opening presses enter and stops before the detail response lands, so a test
// can see the frame the reader gets first. The pending fetch comes back with
// it, to be delivered when the test is ready.
func opening(m tea.Model) (tea.Model, tea.Cmd) {
	m, cmd := m.Update(keyMsg("enter"))
	for _, msg := range immediate(cmd) {
		m, cmd = m.Update(msg)
	}
	return m, cmd
}

func TestOpeningAPullRequestFetchesItOnce(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(loaded(t, client, 160, 40), "enter")

	if got := client.opened(); len(got) != 1 || got[0] != "PR_412" {
		t.Errorf("opened %v, want one fetch for PR_412", got)
	}
	if !strings.Contains(stripANSI(render(t, m)), "Caps the backoff at 30s.") {
		t.Error("the conversation never reached the screen")
	}
}

// The point of holding a detail is that the second open costs no wait. The
// refetch still goes out; it swaps in behind whatever is already being read.
func TestReopeningPaintsFromWhatIsHeldAndRefetchesBehindIt(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(loaded(t, client, 160, 40), "enter")
	if !strings.Contains(stripANSI(render(t, m)), "Caps the backoff at 30s.") {
		t.Fatal("setup: the first open never loaded")
	}

	again, pending := opening(press(m, "esc"))
	if !strings.Contains(stripANSI(render(t, again)), "Caps the backoff at 30s.") {
		t.Error("the second open waited on the network rather than painting what was held")
	}

	settle(again, immediate(pending)...)
	if got := client.opened(); len(got) != 2 {
		t.Errorf("opened %v, want the reopen to have refetched", got)
	}
}

// Open one, escape, open another, and the first response still arrives. It
// must not land on the screen that replaced it.
func TestAResponseForAPullRequestYouLeftDoesNotReachTheScreen(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "the auth retry description")
	client.serveDetail("PR_408", "the dependency bump description")

	first, stale := opening(loaded(t, client, 160, 40))
	elsewhere := press(press(first, "esc"), "j", "enter")

	if !strings.Contains(stripANSI(render(t, elsewhere)), "the dependency bump description") {
		t.Fatal("setup: the second pull request never loaded")
	}

	settled := settle(elsewhere, immediate(stale)...)
	out := stripANSI(render(t, settled))
	if strings.Contains(out, "the auth retry description") {
		t.Error("a response from the pull request that was left landed on the one on screen")
	}
	if !strings.Contains(out, "the dependency bump description") {
		t.Error("the screen lost what it was showing")
	}
}

// The screen keeps reading through a failed refetch, so the toast is the only
// thing saying it happened.
func TestAFailedRefetchKeepsTheConversationAndSaysSo(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(loaded(t, client, 160, 40), "enter")
	client.failDetails(errors.New("no such host"))

	again := press(press(m, "esc"), "enter")
	out := stripANSI(render(t, again))

	if !strings.Contains(out, "Caps the backoff at 30s.") {
		t.Error("the failed refetch emptied a screen that was reading fine")
	}
	if !strings.Contains(out, "Could not refresh #412") {
		t.Errorf("frame = %q, want a toast naming the pull request that failed", out)
	}
}

// Nothing held and nothing back yet is its own state, and it is the one the
// reader sees most often.
func TestAFirstOpenSaysItIsLoading(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	waiting, _ := opening(loaded(t, client, 160, 40))
	out := stripANSI(render(t, waiting))

	if !strings.Contains(out, "Loading the conversation") {
		t.Error("the first open renders nothing while it waits")
	}
	// The glyph is the only thing saying the wait is going somewhere. Its first
	// frame is what a screen that never armed its spinner would also render, so
	// the moving part is asserted in the prview suite.
	if !strings.ContainsAny(out, "⣾⣽⣻⢿⡿⣟⣯⣷") {
		t.Errorf("frame = %q, want a spinner beside the label", out)
	}
}

// The detail query is the most expensive call in the app, so the budget on
// screen has to move with it rather than only with the sections.
func TestOpeningMovesTheBudget(t *testing.T) {
	client := &fakeSearcher{
		prs:  samplePRs(),
		rate: gh.RateLimit{Limit: 5000, Remaining: 4700, ResetAt: time.Now().Add(time.Hour)},
	}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := loaded(t, client, 160, 40)
	client.rate = gh.RateLimit{Limit: 5000, Remaining: 4697, ResetAt: client.rate.ResetAt}

	if out := render(t, press(m, "enter")); !strings.Contains(out, "4697") {
		t.Errorf("frame = %q, want the budget the detail response carried", out)
	}
}

// The glyph appearing is not the same as the glyph moving. This is the wiring
// between the two: the screen arms its own chain, and the root routes the ticks
// back to it.
func TestOpeningArmsTheDetailSpinner(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	waiting, pending := opening(loaded(t, client, 160, 40))

	var started spinner.TickMsg
	var armed bool
	for _, msg := range immediate(pending) {
		if got, ok := msg.(spinner.TickMsg); ok {
			started, armed = got, true
		}
	}
	if !armed {
		t.Fatal("opening armed no spinner, so the glyph would never move")
	}

	before := stripANSI(render(t, waiting))
	moved, _ := waiting.Update(started)
	if stripANSI(render(t, moved)) == before {
		t.Error("the tick did not reach the detail screen")
	}
}

// The screen is new on every open, and so is its spinner. Arming the chain with
// the fetch leaves it frozen here, because the request is already out and the
// old chain's ticks carry a tag the new spinner drops.
func TestReopeningWhileTheFetchIsStillOutKeepsTheSpinnerRunning(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	// Open and leave without letting the response land, so the store still has
	// the request out when the second open happens.
	opened, pending := opening(loaded(t, client, 160, 40))
	back := settle(opened, keyMsg("esc"))

	again, reopened := opening(back)

	var tick spinner.TickMsg
	var armed bool
	for _, msg := range immediate(reopened) {
		if got, ok := msg.(spinner.TickMsg); ok {
			tick, armed = got, true
		}
	}
	if !armed {
		t.Fatal("the reopen armed no spinner, so the glyph would sit frozen")
	}

	before := stripANSI(render(t, again))
	moved, _ := again.Update(tick)
	if stripANSI(render(t, moved)) == before {
		t.Error("the tick did not reach the reopened screen")
	}

	// The fetch was not started twice: the first one is still out.
	settle(again, immediate(pending)...)
	if got := client.opened(); len(got) != 1 {
		t.Errorf("opened %v, want the one request that was already in flight", got)
	}
}
