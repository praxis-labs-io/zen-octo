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
}
