package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/zen-octo/zen-octo/internal/config"
	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/store"
)

func configured() []config.Section {
	return []config.Section{
		{Title: "My PRs", Filters: "is:open is:pr author:@me"},
		{Title: "Needs My Review", Filters: "is:open is:pr review-requested:@me"},
		{Title: "Involved", Filters: "is:open is:pr involves:@me"},
	}
}

func result(ids ...string) gh.SearchResult {
	prs := make([]gh.PullRequest, len(ids))
	for i, id := range ids {
		prs[i] = gh.PullRequest{ID: id}
	}
	return gh.SearchResult{PullRequests: prs}
}

// Sections answer in whatever order they finish, so an arrival names its slot
// rather than the store assuming the one it is waiting on.
func TestResponsesLandInTheirOwnSectionWhateverTheOrder(t *testing.T) {
	s := store.New(configured())
	s.BeginAll()

	s.Applied(2, result("c1", "c2", "c3"))
	s.Applied(0, result("a1"))
	s.Applied(1, result("b1", "b2"))

	want := []int{1, 2, 3}
	for i, section := range s.Sections() {
		if len(section.PRs) != want[i] {
			t.Errorf("section %d holds %d rows, want %d", i, len(section.PRs), want[i])
		}
		if section.Status != store.StatusReady {
			t.Errorf("section %d status = %v, want ready", i, section.Status)
		}
	}
}

func TestTheBudgetFallsThroughABurst(t *testing.T) {
	window := time.Now().Add(time.Hour)
	later := window.Add(time.Hour)

	tests := []struct {
		name string
		in   []gh.RateLimit
		want int
	}{
		{
			name: "out of order arrivals keep the lowest in the window",
			in: []gh.RateLimit{
				{Limit: 5000, Remaining: 4998, ResetAt: window},
				{Limit: 5000, Remaining: 4996, ResetAt: window},
				{Limit: 5000, Remaining: 4997, ResetAt: window},
			},
			want: 4996,
		},
		{
			name: "a later reset is a new window, where it legitimately goes up",
			in: []gh.RateLimit{
				{Limit: 5000, Remaining: 12, ResetAt: window},
				{Limit: 5000, Remaining: 4999, ResetAt: later},
			},
			want: 4999,
		},
		{
			// The straggler was issued before the reset, so its exhausted
			// number describes a window that no longer exists.
			name: "a straggler from the previous window does not pull it back down",
			in: []gh.RateLimit{
				{Limit: 5000, Remaining: 4999, ResetAt: later},
				{Limit: 5000, Remaining: 8, ResetAt: window},
			},
			want: 4999,
		},
		{
			name: "a response carrying no budget leaves the held one alone",
			in: []gh.RateLimit{
				{Limit: 5000, Remaining: 4998, ResetAt: window},
				{},
			},
			want: 4998,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := store.New(configured())
			s.BeginAll()
			for i, rate := range tt.in {
				s.Applied(i, gh.SearchResult{RateLimit: rate})
			}

			if got := s.Rate().Remaining; got != tt.want {
				t.Errorf("remaining = %d, want %d", got, tt.want)
			}
		})
	}
}

// One request per section is the invariant everything above rests on: it is
// what lets an arrival be applied to its slot with no staleness check.
func TestBeginRefusesASectionAlreadyInFlight(t *testing.T) {
	s := store.New(configured())

	if !s.Begin(0) {
		t.Fatal("the first Begin was refused")
	}
	if s.Begin(0) {
		t.Error("a second Begin started a duplicate request")
	}
	if !s.Loading() {
		t.Error("Loading is false with a request out")
	}

	s.Applied(0, result("a1"))
	if s.Loading() {
		t.Error("Loading is true with nothing out")
	}
	if !s.Begin(0) {
		t.Error("a settled section refused a refresh")
	}
}

func TestAFailedSectionHoldsItsRowsAndItsError(t *testing.T) {
	s := store.New(configured())
	s.BeginAll()
	s.Applied(0, result("a1", "a2"))

	boom := errors.New("context deadline exceeded")
	s.Begin(0)
	s.Failed(0, boom)

	section := s.Sections()[0]
	if section.Status != store.StatusFailed {
		t.Errorf("status = %v, want failed", section.Status)
	}
	if !errors.Is(section.Err, boom) {
		t.Errorf("err = %v, want the one it failed with", section.Err)
	}
	if len(section.PRs) != 2 {
		t.Errorf("holds %d rows, want the 2 it had before the retry", len(section.PRs))
	}
}

// The view holds a snapshot across frames. Handing it the live slice would let
// a screen write into state only Update is allowed to touch.
func TestTheSnapshotIsACopy(t *testing.T) {
	s := store.New(configured())
	s.BeginAll()
	s.Applied(0, result("a1"))

	snapshot := s.Sections()
	snapshot[0].PRs = nil
	snapshot[0].Status = store.StatusIdle

	if got := s.Sections()[0]; len(got.PRs) != 1 || got.Status != store.StatusReady {
		t.Errorf("the store followed an edit to a snapshot: %d rows, status %v", len(got.PRs), got.Status)
	}
}

// Out of range is a caller bug, not a panic in the middle of Update.
func TestAnIndexOffTheEndIsIgnored(t *testing.T) {
	s := store.New(configured())
	off := len(s.Sections())

	if s.Begin(-1) || s.Begin(off) {
		t.Error("Begin started a section that does not exist")
	}
	s.Applied(off, result("nope"))
	s.Failed(-1, errors.New("nope"))

	for i, section := range s.Sections() {
		if section.Status != store.StatusIdle {
			t.Errorf("section %d moved to %v", i, section.Status)
		}
	}
}

func detailResult(id string, remaining int) gh.DetailResult {
	return gh.DetailResult{
		Detail:    gh.PullRequestDetail{PullRequest: gh.PullRequest{ID: id}, Body: "the description"},
		RateLimit: gh.RateLimit{Limit: 5000, Remaining: remaining, ResetAt: time.Now().Add(time.Hour)},
	}
}

func TestADetailIsHeldForTheNextOpen(t *testing.T) {
	s := store.New(configured())

	if held := s.Detail("PR_412"); held.Loaded || held.Status != store.StatusIdle {
		t.Errorf("a pull request never opened reads as %+v, want the zero value", held)
	}

	s.BeginDetail("PR_412")
	s.DetailApplied("PR_412", detailResult("PR_412", 4800))

	held := s.Detail("PR_412")
	if !held.Loaded || held.Status != store.StatusReady {
		t.Errorf("held = loaded %v status %v, want loaded and ready", held.Loaded, held.Status)
	}
	if held.Detail.Body != "the description" {
		t.Errorf("Body = %q, want what came back", held.Detail.Body)
	}
}

// One request per pull request is what makes opening the same row twice in
// quick succession cost one round trip rather than two.
func TestBeginDetailRefusesOneAlreadyInFlight(t *testing.T) {
	s := store.New(configured())

	if !s.BeginDetail("PR_412") {
		t.Fatal("the first BeginDetail was refused")
	}
	if s.BeginDetail("PR_412") {
		t.Error("a second BeginDetail started a duplicate request")
	}

	s.DetailApplied("PR_412", detailResult("PR_412", 4800))
	if !s.BeginDetail("PR_412") {
		t.Error("a settled pull request refused a refetch")
	}
}

// The screen keeps reading through a failed background refetch. Emptying it
// would be worse news than the news.
func TestAFailedRefetchKeepsTheDetailItAlreadyHad(t *testing.T) {
	s := store.New(configured())
	s.BeginDetail("PR_412")
	s.DetailApplied("PR_412", detailResult("PR_412", 4800))

	boom := errors.New("context deadline exceeded")
	s.BeginDetail("PR_412")
	s.DetailFailed("PR_412", boom)

	held := s.Detail("PR_412")
	if held.Status != store.StatusFailed {
		t.Errorf("status = %v, want failed", held.Status)
	}
	if !errors.Is(held.Err, boom) {
		t.Errorf("err = %v, want the one it failed with", held.Err)
	}
	if !held.Loaded || held.Detail.Body != "the description" {
		t.Error("the failure took the detail with it")
	}
}

// The budget is one number across every call, so a detail has to move it the
// same way a section does.
func TestADetailResponseMovesTheBudget(t *testing.T) {
	s := store.New(configured())
	s.BeginAll()
	s.Applied(0, gh.SearchResult{RateLimit: gh.RateLimit{
		Limit: 5000, Remaining: 4800, ResetAt: time.Now().Add(time.Hour),
	}})

	s.BeginDetail("PR_412")
	s.DetailApplied("PR_412", detailResult("PR_412", 4797))

	if got := s.Rate().Remaining; got != 4797 {
		t.Errorf("remaining = %d, want 4797", got)
	}
}

// The login is asked for once and then read all session. It moves the budget
// like every other response, because the point it costs comes off the same one.
func TestTheViewerIsHeldAndMovesTheBudget(t *testing.T) {
	s := store.New(configured())

	if got := s.Viewer().Login; got != "" {
		t.Errorf("a store that has not asked yet reads as %q, want empty", got)
	}

	s.ViewerApplied(gh.ViewerResult{
		Viewer:    gh.Actor{Login: "drucial"},
		RateLimit: gh.RateLimit{Limit: 5000, Remaining: 4999, ResetAt: time.Now().Add(time.Hour)},
	})

	if got := s.Viewer().Login; got != "drucial" {
		t.Errorf("Viewer().Login = %q, want drucial", got)
	}
	if got := s.Rate().Remaining; got != 4999 {
		t.Errorf("remaining = %d, want 4999", got)
	}
}

// An empty id is a caller bug, not a map entry nothing can reach.
func TestAnEmptyIDIsIgnored(t *testing.T) {
	s := store.New(configured())

	if s.BeginDetail("") {
		t.Error("BeginDetail started a request for no pull request")
	}
	s.DetailApplied("", detailResult("", 4800))
	s.DetailFailed("", errors.New("nope"))

	if held := s.Detail(""); held.Loaded || held.Status != store.StatusIdle {
		t.Errorf("held = %+v, want nothing recorded", held)
	}

	if s.BeginFiles("") {
		t.Error("BeginFiles started a request for no pull request")
	}
	s.FilesApplied("", filesResult())
	s.FilesFailed("", errors.New("nope"))

	if held := s.Files(""); held.Loaded || held.Status != store.StatusIdle {
		t.Errorf("held = %+v, want nothing recorded", held)
	}
}

func filesResult() gh.FilesResult {
	return gh.FilesResult{
		Files:     []gh.ChangedFile{{Path: "internal/gh/files.go", Additions: 3}},
		MoreFiles: 2,
	}
}

func TestADiffIsHeldForTheNextVisitToTheTab(t *testing.T) {
	s := store.New(configured())

	if held := s.Files("PR_412"); held.Loaded || held.Status != store.StatusIdle {
		t.Errorf("a diff never fetched reads as %+v, want the zero value", held)
	}

	s.BeginFiles("PR_412")
	s.FilesApplied("PR_412", filesResult())

	held := s.Files("PR_412")
	if !held.Loaded || held.Status != store.StatusReady {
		t.Errorf("held = loaded %v status %v, want loaded and ready", held.Loaded, held.Status)
	}
	if len(held.Files) != 1 || held.Files[0].Path != "internal/gh/files.go" {
		t.Errorf("files = %+v, want what came back", held.Files)
	}
	if held.MoreFiles != 2 {
		t.Errorf("MoreFiles = %d, want 2", held.MoreFiles)
	}
}

// Tabbing in and out of Files while the first request is out has to cost one
// round trip, the same way opening a row twice does.
func TestBeginFilesRefusesOneAlreadyInFlight(t *testing.T) {
	s := store.New(configured())

	if !s.BeginFiles("PR_412") {
		t.Fatal("the first BeginFiles was refused")
	}
	if s.BeginFiles("PR_412") {
		t.Error("a second BeginFiles started a duplicate request")
	}

	s.FilesApplied("PR_412", filesResult())
	if !s.BeginFiles("PR_412") {
		t.Error("a settled diff refused a refetch")
	}
}

func TestAFailedRefetchKeepsTheDiffItAlreadyHad(t *testing.T) {
	s := store.New(configured())
	s.BeginFiles("PR_412")
	s.FilesApplied("PR_412", filesResult())

	boom := errors.New("context deadline exceeded")
	s.BeginFiles("PR_412")
	s.FilesFailed("PR_412", boom)

	held := s.Files("PR_412")
	if held.Status != store.StatusFailed {
		t.Errorf("status = %v, want failed", held.Status)
	}
	if !errors.Is(held.Err, boom) {
		t.Errorf("err = %v, want the one it failed with", held.Err)
	}
	if !held.Loaded || len(held.Files) != 1 {
		t.Error("the failure took the diff with it")
	}
}

func TestACommitsDiffIsHeldForTheNextTimeItIsOpened(t *testing.T) {
	s := store.New(configured())

	if held := s.CommitFiles("a3f91c2"); held.Loaded || held.Status != store.StatusIdle {
		t.Errorf("a commit never fetched reads as %+v, want the zero value", held)
	}

	if !s.BeginCommitFiles("a3f91c2") {
		t.Fatal("the first BeginCommitFiles was refused")
	}
	if s.BeginCommitFiles("a3f91c2") {
		t.Error("a second BeginCommitFiles started a duplicate request")
	}
	s.CommitFilesApplied("a3f91c2", filesResult())

	held := s.CommitFiles("a3f91c2")
	if !held.Loaded || held.Status != store.StatusReady {
		t.Errorf("held = loaded %v status %v, want loaded and ready", held.Loaded, held.Status)
	}
	if len(held.Files) != 1 || held.Files[0].Path != "internal/gh/files.go" {
		t.Errorf("files = %+v, want what came back", held.Files)
	}
}

func TestAFailedCommitRefetchKeepsTheDiffItAlreadyHad(t *testing.T) {
	s := store.New(configured())
	s.BeginCommitFiles("a3f91c2")
	s.CommitFilesApplied("a3f91c2", filesResult())

	boom := errors.New("context deadline exceeded")
	s.BeginCommitFiles("a3f91c2")
	s.CommitFilesFailed("a3f91c2", boom)

	held := s.CommitFiles("a3f91c2")
	if !errors.Is(held.Err, boom) || held.Status != store.StatusFailed {
		t.Errorf("held = %v / %v, want failed with the error it failed on", held.Status, held.Err)
	}
	if !held.Loaded || len(held.Files) != 1 {
		t.Error("the failure took the diff with it")
	}
}

// A commit and a pull request are keyed in separate maps, so a sha that happens
// to match an id answers for its own diff rather than the other's.
func TestACommitsDiffIsHeldApartFromThePullRequests(t *testing.T) {
	s := store.New(configured())
	s.BeginFiles("PR_412")
	s.FilesApplied("PR_412", filesResult())

	if held := s.CommitFiles("PR_412"); held.Loaded {
		t.Error("a pull request's diff answered for a commit")
	}
}

// The two caches are keyed the same but answer different questions. A diff must
// not read as loaded because the conversation is.
func TestTheDiffAndTheDetailAreHeldApart(t *testing.T) {
	s := store.New(configured())
	s.BeginDetail("PR_412")
	s.DetailApplied("PR_412", detailResult("PR_412", 4800))

	if held := s.Files("PR_412"); held.Loaded {
		t.Error("a loaded detail made the diff read as loaded")
	}
}
