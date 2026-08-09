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
	"github.com/zen-octo/zen-octo/internal/tui/prview"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// fakeSearcher answers every section with the same rows. Sections fetch
// concurrently, so it locks: the -race detector is the point of the suite.
type fakeSearcher struct {
	rate   gh.RateLimit
	viewer gh.ViewerResult
	err    error

	viewerErr error

	mu          sync.Mutex
	prs         []gh.PullRequest
	queries     []string
	opens       []string
	diffs       []string
	commitDiffs []string
	details     map[string]gh.PullRequestDetail
	files       map[int][]gh.ChangedFile
	commitFiles map[string][]gh.ChangedFile
	posted      []string
	detailErr   error
	filesErr    error
	commitErr   error
	postErr     error
	commitHold  time.Duration
	postHold    time.Duration
	gotLimit    int
	gotDeadline time.Time
	hadDeadline bool
}

func (f *fakeSearcher) Viewer(context.Context) (gh.ViewerResult, error) {
	if f.viewerErr != nil {
		return gh.ViewerResult{}, f.viewerErr
	}
	return f.viewer, nil
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

// serveCommits stages the commits behind one pull request.
func (f *fakeSearcher) serveCommits(id string, commits []gh.Commit) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.details == nil {
		f.details = make(map[string]gh.PullRequestDetail)
	}
	held := f.details[id]
	held.Commits = commits
	f.details[id] = held
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

// PullRequestFiles answers with whatever diff the test staged for that number.
// It is keyed by number rather than id because the diff comes over REST, which
// addresses a pull request by repository and number.
func (f *fakeSearcher) PullRequestFiles(_ context.Context, repo string, number, changed int) (gh.FilesResult, error) {
	f.mu.Lock()
	f.diffs = append(f.diffs, repo+"#"+strconv.Itoa(number))
	files, err := f.files[number], f.filesErr
	f.mu.Unlock()

	if err != nil {
		return gh.FilesResult{}, err
	}
	return gh.FilesResult{Files: files, MoreFiles: max(0, changed-len(files))}, nil
}

// serveFiles stages one pull request's diff.
func (f *fakeSearcher) serveFiles(number int, files []gh.ChangedFile) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.files == nil {
		f.files = make(map[int][]gh.ChangedFile)
	}
	f.files[number] = files
}

// failFiles makes every diff fetch fail from here on.
func (f *fakeSearcher) failFiles(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.filesErr = err
}

// fetched is the pull requests the model asked a diff for, in order.
func (f *fakeSearcher) fetched() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.diffs)
}

// CommitFiles answers with whatever diff the test staged for that sha.
func (f *fakeSearcher) CommitFiles(_ context.Context, repo, sha string) (gh.FilesResult, error) {
	f.mu.Lock()
	f.commitDiffs = append(f.commitDiffs, repo+"@"+sha)
	files, err, hold := f.commitFiles[sha], f.commitErr, f.commitHold
	f.mu.Unlock()

	time.Sleep(hold)

	if err != nil {
		return gh.FilesResult{}, err
	}
	return gh.FilesResult{Files: files}, nil
}

func (f *fakeSearcher) AddComment(_ context.Context, subjectID, body string) (gh.CommentResult, error) {
	f.mu.Lock()
	f.posted = append(f.posted, subjectID+": "+body)
	err, hold := f.postErr, f.postHold
	f.mu.Unlock()

	time.Sleep(hold)

	if err != nil {
		return gh.CommentResult{}, err
	}
	return gh.CommentResult{
		Comment: gh.Comment{
			Kind: gh.CommentIssue, ID: "IC_POSTED",
			Author: gh.Actor{Login: "drucial"}, CreatedAt: time.Now(), Body: body,
			ViewerDidAuthor: true, CanEdit: true, CanDelete: true, CanReact: true,
		},
	}, nil
}

// written is the comments the model sent, in order.
func (f *fakeSearcher) written() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.posted)
}

// holdPosts makes every write answer later than the pump waits, which is how a
// test gets its hands on a comment that is still in flight.
func (f *fakeSearcher) holdPosts() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.postHold = 50 * time.Millisecond
}

// holdCommits makes every commit diff answer later than the pump above waits,
// which is how a test gets its hands on a request that is still in flight.
func (f *fakeSearcher) holdCommits() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commitHold = 50 * time.Millisecond
}

// serveCommit stages one commit's diff.
func (f *fakeSearcher) serveCommit(sha string, files []gh.ChangedFile) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.commitFiles == nil {
		f.commitFiles = make(map[string][]gh.ChangedFile)
	}
	f.commitFiles[sha] = files
}

// failCommits makes every commit diff fetch fail from here on.
func (f *fakeSearcher) failCommits(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commitErr = err
}

// fetchedCommits is the commits the model asked a diff for, in order.
func (f *fakeSearcher) fetchedCommits() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.commitDiffs)
}

// querySearcher answers per query, so a test can hold one section's fetch back
// and let another land first.
type querySearcher struct {
	results map[string]gh.SearchResult
	errs    map[string]error
}

func (f *querySearcher) Viewer(context.Context) (gh.ViewerResult, error) {
	return gh.ViewerResult{Viewer: gh.Actor{Login: "drucial"}}, nil
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

func (f *querySearcher) PullRequestFiles(_ context.Context, _ string, _, _ int) (gh.FilesResult, error) {
	return gh.FilesResult{}, nil
}

func (f *querySearcher) CommitFiles(_ context.Context, _, _ string) (gh.FilesResult, error) {
	return gh.FilesResult{}, nil
}

func (f *querySearcher) AddComment(_ context.Context, _, _ string) (gh.CommentResult, error) {
	return gh.CommentResult{}, nil
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

// settleOn stands in for a cursor that has stopped on a commit. The screen arms
// a wait longer than immediate gives any command, so the message it would have
// carried is delivered by hand.
func settleOn(m tea.Model, sha string) tea.Model {
	return settle(m, prview.CommitSettleMsg{SHA: sha})
}

func keyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "ctrl+enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	default:
		return tea.KeyPressMsg{Code: rune(k[0]), Text: k}
	}
}

// write sends a string a character at a time, the way it reaches a text pane.
func write(m tea.Model, text string) tea.Model {
	for _, r := range text {
		m = settle(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
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

	// Every startup fetch, held rather than delivered: the viewer first, then
	// one per section, in the order Init batches them.
	initial := responses(m.Init())
	if len(initial) != 3 {
		t.Fatalf("setup: startup produced %d responses, want the viewer and one per section", len(initial))
	}
	sections := initial[1:]

	// One section home, the other still out, and then r: only the settled one
	// can be refetched.
	m, _ = m.Update(sections[0])
	m, cmd := m.Update(list.RefreshMsg{})

	m = settle(m, sections[1])
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

// Falling back silently reads as "my config is ignored", and a syntax theme
// nobody notices is one the diff was never going to be styled by.
func TestAnUnknownSyntaxThemeIsReported(t *testing.T) {
	cfg := testConfig()
	cfg.SyntaxTheme = "not-a-chroma-style"

	m := drive(t, app.New(cfg, &fakeSearcher{prs: samplePRs()}), tea.WindowSizeMsg{Width: 200, Height: 40})

	if !strings.Contains(stripANSI(render(t, m)), `Unknown syntax theme "not-a-chroma-style"`) {
		t.Error("an unknown syntax theme falls back with nothing said")
	}
}

// The theme names its own, so a config that says nothing gets no warning.
func TestAThemesOwnSyntaxStyleRaisesNoNotice(t *testing.T) {
	m := loaded(t, &fakeSearcher{prs: samplePRs()}, 200, 40)

	if strings.Contains(stripANSI(render(t, m)), "Unknown syntax theme") {
		t.Error("the default theme names a style Chroma does not ship")
	}
}

// The login is asked for at startup, alongside the sections rather than after
// them. Nothing renders it yet, and the budget its response carries is the one
// place it reaches the frame: every section here fails, so the number on screen
// can only have come from the viewer.
func TestTheViewerIsAskedForAtStartup(t *testing.T) {
	client := &fakeSearcher{
		err: errors.New("every section is down"),
		viewer: gh.ViewerResult{
			Viewer:    gh.Actor{Login: "drucial"},
			RateLimit: gh.RateLimit{Limit: 5000, Cost: 1, Remaining: 4999},
		},
	}

	if out := render(t, loaded(t, client, 120, 40)); !strings.Contains(out, "4999") {
		t.Errorf("view = %q, want the budget the viewer response carried", out)
	}
}

// A login that cannot be read degrades rather than fails. Nothing on the screen
// depends on it yet, and a toast here would be the only one at startup, for the
// one failure with no visible effect.
func TestAViewerThatCannotBeReadChangesNothingOnScreen(t *testing.T) {
	rate := gh.RateLimit{Limit: 5000, Cost: 1, Remaining: 4820}

	broken := &fakeSearcher{prs: samplePRs(), rate: rate, viewerErr: errors.New("boom")}
	fine := &fakeSearcher{
		prs: samplePRs(), rate: rate,
		viewer: gh.ViewerResult{Viewer: gh.Actor{Login: "drucial"}},
	}

	if got, want := render(t, loaded(t, broken, 120, 40)), render(t, loaded(t, fine, 120, 40)); got != want {
		t.Errorf("a failed viewer lookup changed the frame:\n%q\nwant\n%q", got, want)
	}
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

// The diff is a second request and often a large one. A pull request opened to
// read the conversation must not pay for it.
func TestTheDiffIsNotFetchedUntilTheFilesTabIsOpened(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveFiles(412, sampleFiles())

	m := press(loaded(t, client, 160, 40), "enter")
	if got := client.fetched(); len(got) != 0 {
		t.Fatalf("fetched %v before the tab was opened", got)
	}

	m = press(m, "]", "]", "]")
	if got := client.fetched(); len(got) != 1 || got[0] != "zen-octo/zen-octo#412" {
		t.Errorf("fetched %v, want one diff for zen-octo/zen-octo#412", got)
	}
	if !strings.Contains(stripANSI(render(t, m)), "delay = min(delay*2, fetchTimeout)") {
		t.Error("the diff never reached the screen")
	}
}

// Tabbing in and out has to cost one request. The store refuses a second while
// the first is out, and holds the answer for the rest of the session.
func TestTabbingBackToFilesDoesNotFetchAgain(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveFiles(412, sampleFiles())

	m := press(loaded(t, client, 160, 40), "enter")
	m = press(m, "]", "]", "]") // to Files
	m = press(m, "]")           // round to the conversation
	press(m, "]", "]", "]")     // and back

	if got := client.fetched(); len(got) != 1 {
		t.Errorf("fetched %v, want one request", got)
	}
}

// Reopening a pull request refetches its conversation. The diff has to follow,
// or a push lands and the Files tab reads the change from before it for the
// rest of the session, under a header carrying the new counts.
func TestReopeningAPullRequestRefetchesItsDiff(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveFiles(412, sampleFiles())

	m := press(loaded(t, client, 160, 40), "enter", "]", "]", "]")
	if got := client.fetched(); len(got) != 1 {
		t.Fatalf("setup: fetched %v, want one request", got)
	}

	// Back to the list and in again, which is what a reader does after a push.
	m = press(m, "esc", "enter", "]", "]", "]")

	if got := client.fetched(); len(got) != 2 {
		t.Errorf("fetched %v, want the diff asked for again on the reopen", got)
	}
	if !strings.Contains(stripANSI(render(t, m)), "delay = min(delay*2, fetchTimeout)") {
		t.Error("the diff is not on screen after the reopen")
	}
}

// A commit's diff is its own request, so the cursor passing over one costs
// nothing and stopping on it is what pays. Walking a long branch a keystroke at
// a time would otherwise spend a request per commit gone by.
func TestOnlyTheCommitTheCursorStopsOnIsFetched(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveCommits("PR_412", []gh.Commit{
		{SHA: "a3f91c2d5e", Short: "a3f91c2", Headline: "Cap the backoff"},
		{SHA: "7b20ef4a11", Short: "7b20ef4", Headline: "Drop the count"},
	})
	client.serveCommit("7b20ef4a11", sampleFiles())

	// Open, on to Commits, into the column, down a row.
	m := press(loaded(t, client, 160, 40), "enter", "]", "1", "j")
	if got := client.fetchedCommits(); len(got) != 0 {
		t.Fatalf("fetched %v before the cursor stopped anywhere", got)
	}

	// Landing on the tab armed a wait naming the first commit, and j left it.
	m = settleOn(m, "a3f91c2d5e")
	if got := client.fetchedCommits(); len(got) != 0 {
		t.Fatalf("fetched %v for a commit the cursor walked past", got)
	}

	m = settleOn(m, "7b20ef4a11")
	want := "zen-octo/zen-octo@7b20ef4a11"
	if got := client.fetchedCommits(); len(got) != 1 || got[0] != want {
		t.Errorf("fetched %v, want one request for %q", got, want)
	}
	if !strings.Contains(stripANSI(render(t, m)), "delay = min(delay*2, fetchTimeout)") {
		t.Error("the commit's diff never reached the screen")
	}

	// Settling again on the commit already showing is answered by the screen.
	if settleOn(m, "7b20ef4a11"); len(client.fetchedCommits()) != 1 {
		t.Errorf("fetched %v, want the second settle to cost nothing",
			client.fetchedCommits())
	}
}

// Landing on the tab is a cursor stopping like any other, so the commit it
// opens on loads without a keypress. Files opens on content and this is the
// same idea.
func TestTheCommitsTabFetchesWhatItOpensOn(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveCommits("PR_412", []gh.Commit{
		{SHA: "a3f91c2d5e", Short: "a3f91c2", Headline: "Cap the backoff"},
	})
	client.serveCommit("a3f91c2d5e", sampleFiles())

	m := settleOn(press(loaded(t, client, 160, 40), "enter", "]"), "a3f91c2d5e")

	if got := client.fetchedCommits(); len(got) != 1 {
		t.Fatalf("fetched %v, want the commit the tab opened on", got)
	}
	if !strings.Contains(stripANSI(render(t, m)), "delay = min(delay*2, fetchTimeout)") {
		t.Error("the tab opened without its diff")
	}
}

// The cache is keyed by sha because a commit's diff is the same wherever it is
// opened from. Walking back up a branch to a commit already read costs nothing.
func TestACommitAlreadyReadIsNotFetchedAgain(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveCommits("PR_412", []gh.Commit{
		{SHA: "a3f91c2d5e", Short: "a3f91c2", Headline: "Cap the backoff"},
		{SHA: "7b20ef4a11", Short: "7b20ef4", Headline: "Drop the count"},
	})
	client.serveCommit("a3f91c2d5e", sampleFiles())
	client.serveCommit("7b20ef4a11", sampleFiles())

	// The first, the second, then back to the first.
	m := settleOn(press(loaded(t, client, 160, 40), "enter", "]", "1"), "a3f91c2d5e")
	m = settleOn(press(m, "j"), "7b20ef4a11")
	m = settleOn(press(m, "k"), "a3f91c2d5e")

	want := []string{"zen-octo/zen-octo@a3f91c2d5e", "zen-octo/zen-octo@7b20ef4a11"}
	if got := client.fetchedCommits(); !slices.Equal(got, want) {
		t.Errorf("fetched %v, want %v: the second read of a commit is cached", got, want)
	}
	if !strings.Contains(stripANSI(render(t, m)), "delay = min(delay*2, fetchTimeout)") {
		t.Error("the cached diff never reached the screen")
	}
}

// A commit already read comes back from the store in the hop it takes the root
// to answer. The pane used to clear itself on the way there, and the frame in
// between is the whole of the complaint: a spinner over a diff nobody waited for.
func TestWalkingBackToACachedCommitNeverSpins(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveCommits("PR_412", []gh.Commit{
		{SHA: "a3f91c2d5e", Short: "a3f91c2", Headline: "Cap the backoff"},
		{SHA: "7b20ef4a11", Short: "7b20ef4", Headline: "Drop the count"},
	})
	client.serveCommit("a3f91c2d5e", sampleFiles())
	client.serveCommit("7b20ef4a11", sampleFiles())

	m := settleOn(press(loaded(t, client, 160, 40), "enter", "]", "1"), "a3f91c2d5e")
	m = settleOn(press(m, "j"), "7b20ef4a11")

	// One hop at a time on the way back. settleOn runs the queue to the end and
	// would render straight past the frame this is about.
	m = press(m, "k")
	m, _ = m.Update(prview.CommitSettleMsg{SHA: "a3f91c2d5e"})

	if strings.Contains(stripANSI(render(t, m)), "Loading the diff") {
		t.Error("the pane spun on the way to a diff the store already held")
	}
	if got := client.fetchedCommits(); len(got) != 2 {
		t.Errorf("fetched %v, want the cached commit answered without a request", got)
	}
}

// Settling on a commit resets the pane to idle, and a spinner over an idle pane
// stops ticking. Coming back to one whose request is still out has to put the
// pane back into its loading state or the glyph sits there frozen.
func TestReturningToACommitStillInFlightKeepsTheSpinnerAlive(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveCommits("PR_412", []gh.Commit{
		{SHA: "a3f91c2d5e", Short: "a3f91c2", Headline: "Cap the backoff"},
		{SHA: "7b20ef4a11", Short: "7b20ef4", Headline: "Drop the count"},
	})
	client.holdCommits()

	// The first, on to the second, then back before either answers.
	m := settleOn(press(loaded(t, client, 160, 40), "enter", "]", "1"), "a3f91c2d5e")
	m = settleOn(press(m, "j"), "7b20ef4a11")
	m = settleOn(press(m, "k"), "a3f91c2d5e")

	if !strings.Contains(stripANSI(render(t, m)), "Loading the diff") {
		t.Fatal("the pane dropped out of its loading state with the request still out")
	}
	if got := client.fetchedCommits(); len(got) != 2 {
		t.Errorf("fetched %v, want the return to ride the request already out", got)
	}

	// The glyph is the tell. A pane the reselection left reading idle renders
	// the spinner and then never advances it again.
	before := spinnerGlyph(render(t, m), "Loading the diff")
	m, _ = m.Update(spinner.TickMsg{})
	after := spinnerGlyph(render(t, m), "Loading the diff")

	if before == "" || after == "" {
		t.Fatalf("no spinner on the pane, glyphs %q and %q", before, after)
	}
	if before == after {
		t.Error("the spinner froze with the request still out")
	}
}

// spinnerGlyph is the frame of the spinner sitting beside a label.
func spinnerGlyph(frame, label string) string {
	for _, line := range strings.Split(stripANSI(frame), "\n") {
		at := strings.Index(line, label)
		if at <= 0 {
			continue
		}
		if lead := []rune(strings.TrimRight(line[:at], " ")); len(lead) > 0 {
			return string(lead[len(lead)-1])
		}
	}
	return ""
}

func TestAFailedCommitDiffSaysSoOnTheTab(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveCommits("PR_412", []gh.Commit{{SHA: "a3f91c2d5e", Short: "a3f91c2", Headline: "Cap the backoff"}})
	client.failCommits(errors.New("context deadline exceeded"))

	m := settleOn(press(loaded(t, client, 160, 40), "enter", "]"), "a3f91c2d5e")

	if !strings.Contains(stripANSI(render(t, m)), "Could not load the diff") {
		t.Error("a failed commit diff reads as an empty one")
	}
}

// A failed fetch leaves its error on the pane. With no key left to ask again,
// walking off the commit and back is the retry, and it has to be one or the
// error sits there for as long as the screen is open.
func TestWalkingBackOntoAFailedCommitRetriesIt(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveCommits("PR_412", []gh.Commit{
		{SHA: "a3f91c2d5e", Short: "a3f91c2", Headline: "Cap the backoff"},
		{SHA: "7b20ef4a11", Short: "7b20ef4", Headline: "Drop the count"},
	})
	client.failCommits(errors.New("context deadline exceeded"))

	m := settleOn(press(loaded(t, client, 160, 40), "enter", "]", "1"), "a3f91c2d5e")
	if got := client.fetchedCommits(); len(got) != 1 {
		t.Fatalf("fetched %v, want the first request", got)
	}

	client.failCommits(nil)
	client.serveCommit("a3f91c2d5e", sampleFiles())
	m = settleOn(press(m, "j"), "7b20ef4a11")
	m = settleOn(press(m, "k"), "a3f91c2d5e")

	if got := client.fetchedCommits(); len(got) != 3 {
		t.Errorf("fetched %v, want the failed commit asked for again", got)
	}
	if !strings.Contains(stripANSI(render(t, m)), "delay = min(delay*2, fetchTimeout)") {
		t.Error("the retry never reached the screen")
	}
}

// refreshing presses r and stops before the responses land, so a test can see
// the frame the reader gets while the requests are out. The pending fetches
// come back with it, to be delivered when the test is ready.
func refreshing(m tea.Model) (tea.Model, tea.Cmd) {
	m, cmd := m.Update(keyMsg("r"))
	for _, msg := range immediate(cmd) {
		m, cmd = m.Update(msg)
	}
	return m, cmd
}

// Backing out to the list to refresh and opening again is three keys to answer
// "has anything happened since". The detail screen refetches in place.
func TestRefreshingTheDetailRefetchesTheConversation(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(loaded(t, client, 160, 40), "enter")
	client.serveDetail("PR_412", "Caps the backoff at 60s.")
	m = press(m, "r")

	if got := client.opened(); len(got) != 2 {
		t.Errorf("opened %v, want the refresh to have refetched", got)
	}
	if !strings.Contains(stripANSI(render(t, m)), "Caps the backoff at 60s.") {
		t.Error("the refreshed conversation never reached the screen")
	}
}

// The conversation and the checks read the detail alone. A diff is a second
// request and the most expensive one on the screen; a refresh must not spend it
// on a tab that is not showing one.
func TestRefreshingTheConversationAsksForNoDiff(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveCommits("PR_412", []gh.Commit{{SHA: "a3f91c2d5e", Short: "a3f91c2", Headline: "Cap the backoff"}})

	m := press(loaded(t, client, 160, 40), "enter", "r")

	if got := client.fetched(); len(got) != 0 {
		t.Errorf("fetched %v, want no diff for a refresh on the conversation", got)
	}
	if got := client.fetchedCommits(); len(got) != 0 {
		t.Errorf("fetched commits %v, want none", got)
	}

	// The Checks tab reads the same response, so it asks for nothing extra either.
	press(m, "]", "]", "r")
	if got := client.fetched(); len(got) != 0 {
		t.Errorf("fetched %v, want no diff for a refresh on the checks", got)
	}
}

// A push lands and the Files tab is showing the change from before it. The
// detail carries the new counts but not the diff, so the diff has to go too.
func TestRefreshingOnTheFilesTabRefetchesTheDiff(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveFiles(412, sampleFiles())

	m := press(loaded(t, client, 160, 40), "enter", "]", "]", "]")
	if got := client.fetched(); len(got) != 1 {
		t.Fatalf("setup: fetched %v, want one request", got)
	}

	m = press(m, "r")

	if got := client.fetched(); len(got) != 2 {
		t.Errorf("fetched %v, want the diff asked for again", got)
	}
	if !strings.Contains(stripANSI(render(t, m)), "delay = min(delay*2, fetchTimeout)") {
		t.Error("the diff left the screen over the refresh")
	}
}

// A commit's diff is cached by sha and nothing else asks for one twice, so the
// refresh is the only way to see an amended commit.
func TestRefreshingOnTheCommitsTabRefetchesTheCommitOnThePane(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveCommits("PR_412", []gh.Commit{{SHA: "a3f91c2d5e", Short: "a3f91c2", Headline: "Cap the backoff"}})
	client.serveCommit("a3f91c2d5e", sampleFiles())

	m := settleOn(press(loaded(t, client, 160, 40), "enter", "]"), "a3f91c2d5e")
	if got := client.fetchedCommits(); len(got) != 1 {
		t.Fatalf("setup: fetched %v, want one request", got)
	}

	m = press(m, "r")

	want := []string{"zen-octo/zen-octo@a3f91c2d5e", "zen-octo/zen-octo@a3f91c2d5e"}
	if got := client.fetchedCommits(); !slices.Equal(got, want) {
		t.Errorf("fetched commits %v, want %v", got, want)
	}
	if !strings.Contains(stripANSI(render(t, m)), "delay = min(delay*2, fetchTimeout)") {
		t.Error("the commit diff left the screen over the refresh")
	}
}

// The screen keeps what it has through the refresh. Clearing it would take the
// conversation away from the reader for as long as the request is out, which is
// the whole reason the detail screen does not spin over content.
func TestARefreshKeepsTheConversationOnScreen(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(loaded(t, client, 160, 40), "enter")
	waiting, pending := refreshing(m)

	out := stripANSI(render(t, waiting))
	if !strings.Contains(out, "Caps the backoff at 30s.") {
		t.Error("the conversation went away while the refresh was out")
	}
	if strings.Contains(out, "Loading the conversation") {
		t.Error("the screen spun over content it was already showing")
	}
	settle(waiting, immediate(pending)...)
}

// Nothing moves on the screen during a refresh, so the bar is the only place
// anything can say r did something.
func TestARefreshSpinsInTheStatusBar(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(loaded(t, client, 160, 40), "enter")
	waiting, pending := refreshing(m)

	if !strings.Contains(lastLine(render(t, waiting)), "Refreshing") {
		t.Errorf("status bar = %q, want the refresh on it", strings.TrimSpace(lastLine(render(t, waiting))))
	}

	done := settle(waiting, immediate(pending)...)
	if strings.Contains(lastLine(render(t, done)), "Refreshing") {
		t.Error("the bar is still spinning after the refresh landed")
	}
}

// A refresh usually comes back with the same conversation, so the toast is the
// only sign it happened.
func TestTheDetailRefreshToastNamesThePullRequest(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(loaded(t, client, 160, 40), "enter", "r")

	if !strings.Contains(lastLine(render(t, m)), "Refreshed #412") {
		t.Errorf("status bar = %q, want the refresh reported", strings.TrimSpace(lastLine(render(t, m))))
	}
}

// One press is one toast. Reporting the detail the moment it lands would call
// the refresh done while its diff was still out.
func TestTheDetailRefreshToastWaitsForTheDiff(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveFiles(412, sampleFiles())

	m := press(loaded(t, client, 160, 40), "enter", "]", "]", "]")
	waiting, pending := refreshing(m)

	answers := responses(pending)
	if len(answers) != 2 {
		t.Fatalf("the refresh started %d requests, want the detail and the diff", len(answers))
	}

	half := settle(waiting, answers[0])
	if strings.Contains(lastLine(render(t, half)), "Refreshed") {
		t.Error("the refresh reported itself with a request still out")
	}

	done := settle(half, answers[1])
	if !strings.Contains(lastLine(render(t, done)), "Refreshed #412") {
		t.Errorf("status bar = %q, want the refresh reported once both landed",
			strings.TrimSpace(lastLine(render(t, done))))
	}
}

// The summary is the one toast a refresh raises. The per-request failures the
// reopen path uses would report the same failure twice beside it, and with at
// most two requests out, naming which leg failed is what says whether the thing
// in front of the reader is the stale one.
func TestADetailRefreshReportsItselfOnce(t *testing.T) {
	boom := errors.New("context deadline exceeded")

	tests := []struct {
		name string
		fail func(*fakeSearcher)
		want string
	}{
		{"both back", func(*fakeSearcher) {}, "Refreshed #412"},
		{"both failed", func(f *fakeSearcher) { f.failDetails(boom); f.failFiles(boom) }, "Refresh failed"},
		{"diff failed", func(f *fakeSearcher) { f.failFiles(boom) }, "Refreshed #412, the diff failed"},
		{"detail failed", func(f *fakeSearcher) { f.failDetails(boom) }, "Refreshed the diff, #412 failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeSearcher{prs: samplePRs()}
			client.serveDetail("PR_412", "Caps the backoff at 30s.")
			client.serveFiles(412, sampleFiles())

			m := press(loaded(t, client, 160, 40), "enter", "]", "]", "]")
			tt.fail(client)
			m = press(m, "r")

			bar := lastLine(render(t, m))
			if !strings.Contains(bar, tt.want) {
				t.Errorf("status bar = %q, want %q", strings.TrimSpace(bar), tt.want)
			}
			if strings.Contains(bar, "Could not refresh") {
				t.Errorf("status bar = %q, want the summary rather than a per-request failure",
					strings.TrimSpace(bar))
			}
		})
	}
}

// The Conversation and Checks tabs ask for no diff, so a refresh that failed
// there failed whole. Reading the failure flags alone made the toast name a
// request this press never sent.
func TestARefreshWithNoDiffOutNeverBlamesTheDiff(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(loaded(t, client, 160, 40), "enter")
	client.failDetails(errors.New("context deadline exceeded"))
	m = press(m, "r")

	bar := lastLine(render(t, m))
	if strings.Contains(bar, "diff") {
		t.Errorf("status bar = %q, want no diff named by a refresh that asked for none",
			strings.TrimSpace(bar))
	}
	if !strings.Contains(bar, "Refresh failed") {
		t.Errorf("status bar = %q, want the failure reported", strings.TrimSpace(bar))
	}
}

// A second r asks for whatever the first did not. Replacing the record rather
// than merging into it dropped the leg the first press was still waiting on,
// and its response then reported nothing at all.
func TestASecondRefreshJoinsTheOneStillRunning(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveCommits("PR_412", []gh.Commit{{SHA: "a3f91c2d5e", Short: "a3f91c2", Headline: "Cap the backoff"}})
	client.serveCommit("a3f91c2d5e", sampleFiles())

	// A commit already read, so the second r has a diff leg to start while the
	// first press's detail is still out.
	m := settleOn(press(loaded(t, client, 160, 40), "enter", "]"), "a3f91c2d5e")
	m = press(m, "[")

	client.failDetails(errors.New("context deadline exceeded"))
	waiting, pending := refreshing(m)
	again, alsoPending := refreshing(press(waiting, "]"))

	done := settle(settle(again, immediate(alsoPending)...), immediate(pending)...)

	bar := lastLine(render(t, done))
	if !strings.Contains(bar, "Refreshed the diff, #412 failed") {
		t.Errorf("status bar = %q, want both legs reported together", strings.TrimSpace(bar))
	}
	if strings.Contains(bar, "Could not refresh") {
		t.Errorf("status bar = %q, want the summary rather than a per-request failure",
			strings.TrimSpace(bar))
	}
}

// Every Begin refuses a request already out, so leaning on r costs one round
// trip rather than one per press.
func TestRefreshingTwiceWhileTheFirstIsOutCostsOneRequest(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(loaded(t, client, 160, 40), "enter")
	waiting, pending := refreshing(m)
	again, _ := refreshing(waiting)

	settle(again, immediate(pending)...)
	if got := client.opened(); len(got) != 2 {
		t.Errorf("opened %v, want the open and one refresh", got)
	}
}

// A diff that failed has nothing worth keeping, so the refresh puts the pane
// back into its loading state and the retry lands on it.
func TestRefreshingRetriesADiffThatFailed(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.failFiles(errors.New("context deadline exceeded"))

	m := press(loaded(t, client, 160, 40), "enter", "]", "]", "]")
	if !strings.Contains(stripANSI(render(t, m)), "Could not load the diff") {
		t.Fatal("setup: the diff did not fail")
	}

	client.failFiles(nil)
	client.serveFiles(412, sampleFiles())
	m = press(m, "r")

	if !strings.Contains(stripANSI(render(t, m)), "delay = min(delay*2, fetchTimeout)") {
		t.Error("the retried diff never reached the pane")
	}
}

// Every settle path drops a response for a screen the reader has left, so a
// refresh abandoned by esc never settles. Without clearing it the bar spins
// over the list with nothing coming.
func TestLeavingTheDetailStopsTheRefreshSpinner(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(loaded(t, client, 160, 40), "enter")
	waiting, _ := refreshing(m)

	if got := lastLine(render(t, press(waiting, "esc"))); strings.Contains(got, "Refreshing") {
		t.Errorf("the list's status bar = %q, want no refresh left running on it", strings.TrimSpace(got))
	}
}

// Naming the repository made the right side of the status bar long enough to
// stop fitting beside the detail screen's help line, and the bar used to drop
// that side whole rather than clip it, taking the number with it.
func TestTheDetailStatusBarKeepsTheNumberAtEveryWidth(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	for _, width := range []int{100, 120, 160, 200} {
		// The number is in the header too, so only the bar's own line answers.
		bar := stripANSI(lastLine(render(t, press(loaded(t, client, width, 40), "enter"))))
		if !strings.Contains(bar, "#412") {
			t.Errorf("width %d: the status bar lost the pull request number: %q", width, bar)
		}
	}
}

func TestAFailedDiffFetchSaysSoOnTheTab(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.failFiles(errors.New("context deadline exceeded"))

	m := press(loaded(t, client, 160, 40), "enter")
	m = press(m, "]", "]", "]")

	if !strings.Contains(stripANSI(render(t, m)), "Could not load the diff") {
		t.Error("a failed diff fetch reads as an empty one")
	}
}

// Both caches are keyed by pull request. Opening a second one must not paint
// the first one's diff under it.
func TestADiffDoesNotFollowTheReaderToTheNextPullRequest(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveFiles(412, sampleFiles())

	m := press(loaded(t, client, 160, 40), "enter", "]", "]", "]")
	if !strings.Contains(stripANSI(render(t, m)), "delay = min(delay*2, fetchTimeout)") {
		t.Fatal("the first diff never landed")
	}

	m = press(m, "esc", "j", "enter", "]", "]", "]")

	if strings.Contains(stripANSI(render(t, m)), "delay = min(delay*2, fetchTimeout)") {
		t.Error("the first pull request's diff is showing on the second")
	}
	if got := client.fetched(); len(got) != 2 || got[1] != "zen-octo/zen-octo#408" {
		t.Errorf("fetched %v, want a second request for #408", got)
	}
}

func sampleFiles() []gh.ChangedFile {
	return []gh.ChangedFile{{
		Path: "internal/gh/client.go", Status: gh.FileModified, Additions: 2, Deletions: 1,
		Hunks: []gh.Hunk{{
			Header: "@@ -40,4 +40,5 @@",
			Lines: []gh.DiffLine{
				{Kind: gh.DiffContext, Old: 40, New: 40, Content: "\tfor {"},
				{Kind: gh.DiffRemoved, Old: 41, Content: "\t\ttime.Sleep(delay)"},
				{Kind: gh.DiffAdded, New: 41, Content: "\t\tdelay = min(delay*2, fetchTimeout)"},
			},
		}},
	}}
}

// The number alone says which pull request only if you already know which
// repository you opened it from, and the tabs past the conversation carry
// nothing else that answers it.
func TestTheStatusBarNamesTheRepositoryOnTheDetailScreen(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := press(loaded(t, client, 160, 40), "enter")

	last := lastLine(render(t, m))
	if !strings.Contains(last, "#412 zen-octo/zen-octo") {
		t.Errorf("status bar = %q, want the number and the repository", strings.TrimSpace(last))
	}

	// The list names its section instead; a repository there would be wrong as
	// often as right, since a section can span any number of them.
	back := lastLine(render(t, press(m, "esc")))
	if strings.Contains(back, "zen-octo/zen-octo") {
		t.Errorf("the list's status bar carries a repository: %q", strings.TrimSpace(back))
	}
}

func lastLine(frame string) string {
	lines := strings.Split(stripANSI(frame), "\n")
	return lines[len(lines)-1]
}

// composed is a detail screen with a comment written and not yet sent.
func composed(t *testing.T, client *fakeSearcher, body string) tea.Model {
	t.Helper()

	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	m := press(loaded(t, client, 160, 40), "enter", "c")
	return write(m, body)
}

// The whole of what optimistic means: the card is on the screen before GitHub
// has been told, and it says it has not landed yet.
func TestAPostedCommentIsOnTheScreenBeforeItLands(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.holdPosts()

	m := press(composed(t, client, "ship it"), "ctrl+enter")

	out := stripANSI(render(t, m))
	if !strings.Contains(out, "ship it") {
		t.Errorf("the comment is not in the conversation:\n%s", out)
	}
	if !strings.Contains(out, "posting") {
		t.Error("the comment does not say it is still on its way")
	}
	if got := client.written(); len(got) != 1 || got[0] != "PR_412: ship it" {
		t.Errorf("wrote %v, want one comment on PR_412", got)
	}
}

func TestACommentThatLandsLosesItsMarkerAndSaysSo(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := press(composed(t, client, "ship it"), "ctrl+enter")

	out := stripANSI(render(t, m))
	if !strings.Contains(out, "ship it") {
		t.Fatalf("the comment left the conversation when it landed:\n%s", out)
	}
	if strings.Contains(out, "posting") {
		t.Error("the comment still says it is on its way after GitHub confirmed it")
	}
	if !strings.Contains(out, "Posted") {
		t.Error("nothing on the bar says the comment landed")
	}
}

// The revert branch. The card comes off, the reason goes up, and the words go
// back in the pane: a comment lost to a dropped connection is the one thing
// here that cannot be fetched again.
func TestAFailedPostTakesTheCardBackAndKeepsTheWords(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs(), postErr: errors.New("502 Bad Gateway")}

	m := press(composed(t, client, "ship it"), "ctrl+enter")
	out := stripANSI(render(t, m))

	// Twice would mean the card is still in the conversation as well as in the
	// pane the words came back into.
	if n := strings.Count(out, "ship it"); n != 1 {
		t.Errorf("%q appears %d times, want it only in the composer:\n%s", "ship it", n, out)
	}
	// The box takes the keyboard back with the words, so the reader is looking
	// at the comment they have to do something about.
	if !strings.Contains(out, "ctrl+e editor") {
		t.Error("the box did not take the keyboard back with the failed comment")
	}
	if !strings.Contains(out, "502 Bad Gateway") {
		t.Error("the bar does not say why the comment did not post")
	}
}

// A refresh landing while a comment is out must not take it off the screen.
// The store holds it beside the fetched detail for exactly this.
func TestARefreshDoesNotTakeAwayACommentStillInFlight(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.holdPosts()

	m := press(composed(t, client, "ship it"), "ctrl+enter")
	m = press(m, "r")

	if out := stripANSI(render(t, m)); !strings.Contains(out, "ship it") {
		t.Errorf("the refresh dropped a comment still on its way:\n%s", out)
	}
}

// q is a letter in the compose pane. The root answers it everywhere else, and
// a quit on the way to "quick" would be unforgivable.
func TestQIsALetterWhileACommentIsBeingWritten(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := composed(t, client, "quick question")

	out := stripANSI(render(t, m))
	if !strings.Contains(out, "quick question") {
		t.Errorf("the root ate the letters:\n%s", out)
	}
	if strings.Contains(out, "My PRs") {
		t.Error("a letter in the composer left the detail screen")
	}
}

// ? opens the help overlay everywhere else. In the composer it is punctuation.
func TestTheHelpKeyIsPunctuationWhileACommentIsBeingWritten(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := composed(t, client, "does this work?")

	if out := stripANSI(render(t, m)); !strings.Contains(out, "does this work?") {
		t.Errorf("? opened the help instead of typing:\n%s", out)
	}
}

// One way out has to work from anywhere, including out of a pane taking text.
func TestCtrlCStillQuitsFromTheComposer(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := composed(t, client, "half written")
	_, cmd := m.Update(keyMsg("ctrl+c"))

	if cmd == nil {
		t.Fatal("ctrl+c did nothing in the composer")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Errorf("ctrl+c sent %T, want a quit", msg)
	}
}

// The help overlay stays inside the frame. It is sized from an estimate of how
// wide a column of bindings renders, and an estimate that reads narrow puts the
// modal's right border off the screen: the frame is still the right width, so
// nothing catches it but this.
func TestTheHelpOverlayStaysInsideTheFrame(t *testing.T) {
	sizes := []struct{ width, height int }{
		{width: 200, height: 50},
		{width: 120, height: 40},
		{width: 100, height: 30},
		{width: 80, height: 24},
		{width: 60, height: 20},
	}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			client := &fakeSearcher{prs: samplePRs()}
			client.serveDetail("PR_412", "Caps the backoff at 30s.")
			m := press(loaded(t, client, size.width, size.height), "enter", "?")

			lines := strings.Split(render(t, m), "\n")
			if len(lines) != size.height {
				t.Fatalf("frame is %d lines, want %d", len(lines), size.height)
			}
			for i, line := range lines {
				if w := lipgloss.Width(line); w > size.width {
					t.Errorf("line %d is %d cells wide, want no more than %d", i, w, size.width)
				}
			}

			// The corner is the tell. A modal one column too wide loses it, and
			// every row under it loses its right border with it.
			out := stripANSI(render(t, m))
			at := strings.Index(out, "╭─Keys")
			if at < 0 {
				t.Fatal("the help overlay is not on the frame")
			}
			if head := out[at : strings.Index(out[at:], "\n")+at]; !strings.Contains(head, "╮") {
				t.Errorf("the overlay's top border has no right corner: %q", head)
			}
		})
	}
}

// The viewer query answers after a screen is already open at startup. Taken
// only on open, the comment box is headed by nobody for the rest of the session.
func TestTheViewerReachesAScreenAlreadyOpen(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs(), viewer: gh.ViewerResult{Viewer: gh.Actor{Login: "drucial"}}}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	// Init's messages, with the viewer's held back so the detail screen opens
	// before it lands. The type is unexported and this test is outside the
	// package, so it is named rather than asserted on.
	m := app.New(testConfig(), client)
	var viewer, rest []tea.Msg
	for _, msg := range immediate(m.Init()) {
		if fmt.Sprintf("%T", msg) == "app.viewerFetchedMsg" {
			viewer = append(viewer, msg)
			continue
		}
		rest = append(rest, msg)
	}
	if len(viewer) != 1 {
		t.Fatalf("startup produced %d viewer responses, want one", len(viewer))
	}

	opened := press(settle(m, append(rest, tea.WindowSizeMsg{Width: 160, Height: 40})...), "enter")
	if out := stripANSI(render(t, opened)); strings.Contains(out, "drucial · write a comment") {
		t.Fatal("the viewer reached the screen before the response was applied")
	}

	m2 := settle(opened, viewer...)
	if out := stripANSI(render(t, m2)); !strings.Contains(out, "drucial · write a comment") {
		t.Errorf("the comment box is still headed by nobody:\n%s", out)
	}
}

// The overlay is drawn over the screen rather than into a pane, so a list too
// tall for the frame is cut off the bottom with nothing to say what went.
func TestTheHelpOverlaySaysWhenItCannotShowEverything(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "body")

	// Wide enough for every binding, so nothing is hidden and nothing is said.
	roomy := stripANSI(render(t, press(loaded(t, client, 120, 34), "enter", "?")))
	for _, want := range []string{"comment", "$EDITOR", "quit from anywhere", "refresh"} {
		if !strings.Contains(roomy, want) {
			t.Errorf("a frame with room for the help is missing %q", want)
		}
	}
	if strings.Contains(roomy, "more keys than this frame") {
		t.Error("a frame with room for the help claims it is short of room")
	}

	// Too small to hold it, so it says so rather than quietly dropping nine.
	cramped := stripANSI(render(t, press(loaded(t, client, 60, 20), "enter", "?")))
	if !strings.Contains(cramped, "more keys than this frame") {
		t.Errorf("the overlay drops bindings without saying so:\n%s", cramped)
	}
}
