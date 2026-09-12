package store_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/praxis-labs-io/zen-octo/internal/config"
	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
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

func TestADetailCorrectsTheRowInEverySectionHoldingIt(t *testing.T) {
	s := store.New(configured())
	s.BeginAll()
	s.Applied(0, result("a1", "shared"))
	s.Applied(1, result("shared"))

	s.BeginDetail("shared")
	s.DetailApplied("shared", gh.DetailResult{Detail: gh.PullRequestDetail{
		PullRequest: gh.PullRequest{ID: "shared", State: gh.PRStateMerged, Title: "Landed"},
	}})

	for _, at := range []struct{ section, row int }{{0, 1}, {1, 0}} {
		got := s.Sections()[at.section].PRs[at.row]
		if got.State != gh.PRStateMerged || got.Title != "Landed" {
			t.Errorf("section %d row %d = %+v, want the detail's row", at.section, at.row, got)
		}
	}
	if got := s.Sections()[0].PRs[0].ID; got != "a1" {
		t.Errorf("row beside it = %q, want it untouched", got)
	}
}

func TestCorrectingARowLeavesAnEarlierSnapshotAlone(t *testing.T) {
	s := store.New(configured())
	s.BeginAll()
	s.Applied(0, result("a1"))

	before := s.Sections()[0].PRs

	s.BeginDetail("a1")
	s.DetailApplied("a1", gh.DetailResult{Detail: gh.PullRequestDetail{
		PullRequest: gh.PullRequest{ID: "a1", State: gh.PRStateClosed},
	}})

	if before[0].State == gh.PRStateClosed {
		t.Error("the correction reached a snapshot handed out before it")
	}
}

func reviewed() gh.DetailResult {
	review := gh.Comment{ID: "PRR_1", Author: gh.Actor{Login: "nkr"}}
	detail := gh.PullRequestDetail{
		PullRequest: gh.PullRequest{ID: "pr1"},
		Reviewers: []gh.Reviewer{
			{Actor: gh.Actor{Login: "nkr"}, State: gh.ReviewStateChangesRequested},
		},
		Timeline: []gh.TimelineItem{{
			Kind: gh.TimelineReview, Actor: gh.Actor{Login: "nkr"},
			Comment: &review, Review: gh.ReviewStateChangesRequested,
		}},
		Threads: []gh.ReviewThread{
			{ID: "RT_1", ReviewID: "PRR_1"},
			{ID: "RT_2", ReviewID: "PRR_1"},
		},
	}
	gh.RecountThreads(&detail)
	return gh.DetailResult{Detail: detail}
}

func TestResolvingAThreadMovesTheReviewersCount(t *testing.T) {
	s := store.New(configured())
	s.BeginDetail("pr1")
	s.DetailApplied("pr1", reviewed())

	if got := s.Detail("pr1").Detail.Reviewers[0]; got.Unresolved != 2 || got.Threads != 2 {
		t.Fatalf("setup: fetched %d of %d open, want 2 of 2", got.Unresolved, got.Threads)
	}

	first := s.PendingResolve("pr1", "RT_1", true)
	if got := s.Detail("pr1").Detail.Reviewers[0]; got.Unresolved != 1 {
		t.Errorf("with one held, %d open, want 1: the mark has to move on the press", got.Unresolved)
	}

	s.ResolveApplied("pr1", first, gh.ThreadResult{IsResolved: true, CanUnresolve: true})
	got := s.Detail("pr1").Detail.Reviewers[0]
	if got.Unresolved != 1 || got.Threads != 2 {
		t.Errorf("settled at %d of %d, want 1 of 2", got.Unresolved, got.Threads)
	}

	second := s.PendingResolve("pr1", "RT_2", true)
	s.ResolveApplied("pr1", second, gh.ThreadResult{IsResolved: true, CanUnresolve: true})
	if got := s.Detail("pr1").Detail.Reviewers[0]; got.Unresolved != 0 || got.Threads != 2 {
		t.Errorf("all closed at %d of %d, want 0 of 2", got.Unresolved, got.Threads)
	}
}

func TestUnresolvingPutsTheCountBack(t *testing.T) {
	s := store.New(configured())
	s.BeginDetail("pr1")
	s.DetailApplied("pr1", reviewed())

	key := s.PendingResolve("pr1", "RT_1", true)
	s.ResolveApplied("pr1", key, gh.ThreadResult{IsResolved: true, CanUnresolve: true})

	back := s.PendingResolve("pr1", "RT_1", false)
	s.ResolveApplied("pr1", back, gh.ThreadResult{IsResolved: false, CanResolve: true})

	if got := s.Detail("pr1").Detail.Reviewers[0]; got.Unresolved != 2 {
		t.Errorf("%d open after unresolving, want 2", got.Unresolved)
	}
}

func TestASectionResponseDoesNotUndoAWriteMadeWhileItWasOut(t *testing.T) {
	s := store.New(configured())
	s.BeginAll()
	s.Applied(0, result("a1"))

	s.BeginDetail("a1")
	s.DetailApplied("a1", gh.DetailResult{Detail: gh.PullRequestDetail{
		PullRequest: gh.PullRequest{ID: "a1"},
	}})

	s.Begin(0)
	key := s.PendingState("a1", gh.TransitionClose)
	s.StateApplied("a1", key, gh.PRStateResult{State: gh.PRStateClosed})

	s.Applied(0, result("a1"))

	if got := s.Sections()[0].PRs[0].State; got != gh.PRStateClosed {
		t.Errorf("row = %v, want it still closed: the response predates the write", got)
	}
}

func TestASectionResponseStillReplacesRowsNothingWrote(t *testing.T) {
	s := store.New(configured())
	s.BeginAll()
	s.Applied(0, result("a1"))

	s.BeginDetail("a1")
	s.DetailApplied("a1", gh.DetailResult{Detail: gh.PullRequestDetail{
		PullRequest: gh.PullRequest{ID: "a1", Title: "from the detail"},
	}})

	s.Begin(0)
	fresh := gh.SearchResult{PullRequests: []gh.PullRequest{{ID: "a1", Title: "from the search"}}}
	s.Applied(0, fresh)

	if got := s.Sections()[0].PRs[0].Title; got != "from the search" {
		t.Errorf("row = %q, want the response that was asked for after the write", got)
	}
}

func TestALifecycleWriteReachesTheRowWithoutARefetch(t *testing.T) {
	s := store.New(configured())
	s.BeginAll()
	s.Applied(0, result("a1"))

	s.BeginDetail("a1")
	s.DetailApplied("a1", gh.DetailResult{Detail: gh.PullRequestDetail{
		PullRequest: gh.PullRequest{ID: "a1"},
	}})

	key := s.PendingState("a1", gh.TransitionClose)
	s.StateApplied("a1", key, gh.PRStateResult{State: gh.PRStateClosed})

	if got := s.Sections()[0].PRs[0].State; got != gh.PRStateClosed {
		t.Errorf("row = %v, want it closed on the write rather than on a refetch", got)
	}
}

func TestRecountingLeavesAnEarlierDetailAlone(t *testing.T) {
	s := store.New(configured())
	s.BeginDetail("pr1")
	s.DetailApplied("pr1", reviewed())

	before := s.Detail("pr1").Detail.Reviewers

	key := s.PendingResolve("pr1", "RT_1", true)
	s.ResolveApplied("pr1", key, gh.ThreadResult{IsResolved: true, CanUnresolve: true})

	if before[0].Unresolved != 2 {
		t.Errorf("a detail handed out before the resolve now reads %d open, want 2", before[0].Unresolved)
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

func TestAFileViewedWriteFoldsAndSettlesAgainstTheLatestDiff(t *testing.T) {
	s := store.New(configured())
	s.BeginFiles("PR_412")
	s.FilesApplied("PR_412", gh.FilesResult{Files: []gh.ChangedFile{{Path: "a.go", Viewed: gh.FileUnviewed}}})

	key := s.PendingFileView("PR_412", "a.go", true)
	held := s.Files("PR_412")
	if held.Files[0].Viewed != gh.FileViewed || !held.Files[0].Viewing {
		t.Fatalf("pending file = %+v, want viewed and in flight", held.Files[0])
	}

	s.BeginFiles("PR_412")
	s.FilesApplied("PR_412", gh.FilesResult{Files: []gh.ChangedFile{{Path: "a.go", Viewed: gh.FileDismissed}}})
	s.FileViewApplied("PR_412", key)
	held = s.Files("PR_412")
	if held.Files[0].Viewed != gh.FileViewed || held.Files[0].Viewing {
		t.Fatalf("settled file = %+v, want the write promoted over the refetch", held.Files[0])
	}
}

func TestAFailedFileViewedWriteRevealsTheLatestFetchedState(t *testing.T) {
	s := store.New(configured())
	s.BeginFiles("PR_412")
	s.FilesApplied("PR_412", gh.FilesResult{Files: []gh.ChangedFile{{Path: "a.go", Viewed: gh.FileUnviewed}}})

	key := s.PendingFileView("PR_412", "a.go", true)
	s.BeginFiles("PR_412")
	s.FilesApplied("PR_412", gh.FilesResult{Files: []gh.ChangedFile{{Path: "a.go", Viewed: gh.FileDismissed}}})
	s.FileViewReverted("PR_412", key)

	held := s.Files("PR_412")
	if held.Files[0].Viewed != gh.FileDismissed || held.Files[0].Viewing {
		t.Fatalf("reverted file = %+v, want the refetched dismissed state", held.Files[0])
	}
}

func TestSettlingAFileViewedWriteRestoresTheDiffCacheBound(t *testing.T) {
	s := store.New(configured())
	keys := make([]string, 25)
	for i := range keys {
		id := fmt.Sprintf("PR_%d", i)
		s.BeginFiles(id)
		s.FilesApplied(id, gh.FilesResult{Files: []gh.ChangedFile{{Path: "a.go"}}})
		keys[i] = s.PendingFileView(id, "a.go", true)
	}
	s.BeginFiles("PR_25")
	s.FilesApplied("PR_25", gh.FilesResult{Files: []gh.ChangedFile{{Path: "a.go"}}})

	s.FileViewReverted("PR_0", keys[0])
	if s.Files("PR_0").Loaded {
		t.Error("the unpinned oldest diff remained above the cache bound")
	}
}

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

func TestACommitsDiffIsHeldApartFromThePullRequests(t *testing.T) {
	s := store.New(configured())
	s.BeginFiles("PR_412")
	s.FilesApplied("PR_412", filesResult())

	if held := s.CommitFiles("PR_412"); held.Loaded {
		t.Error("a pull request's diff answered for a commit")
	}
}

func TestTheDiffAndTheDetailAreHeldApart(t *testing.T) {
	s := store.New(configured())
	s.BeginDetail("PR_412")
	s.DetailApplied("PR_412", detailResult("PR_412", 4800))

	if held := s.Files("PR_412"); held.Loaded {
		t.Error("a loaded detail made the diff read as loaded")
	}
}

func bodies(d store.Detail) []string {
	var out []string
	for _, item := range d.Detail.Timeline {
		if item.Kind == gh.TimelineComment {
			out = append(out, item.Said().Body)
		}
	}
	return out
}

func detailWith(bodies ...string) gh.DetailResult {
	items := make([]gh.TimelineItem, len(bodies))
	for i, body := range bodies {
		c := gh.Comment{Kind: gh.CommentIssue, ID: "IC_" + body, Body: body}
		items[i] = gh.TimelineItem{Kind: gh.TimelineComment, Comment: &c}
	}
	return gh.DetailResult{Detail: gh.PullRequestDetail{Timeline: items}}
}

func TestAPendingCommentRendersBeforeItLands(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", detailWith("first"))

	s.PendingComment("PR_1", gh.Comment{Kind: gh.CommentIssue, Body: "mine"})

	if got := bodies(s.Detail("PR_1")); len(got) != 2 || got[1] != "mine" {
		t.Errorf("timeline = %q, want the pending comment at the end", got)
	}

	last := s.Detail("PR_1").Detail.Timeline[1].Said()
	if !last.Pending {
		t.Error("the pending comment is not marked pending")
	}
}

func TestARefetchDoesNotDropACommentStillInFlight(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", detailWith("first"))
	s.PendingComment("PR_1", gh.Comment{Kind: gh.CommentIssue, Body: "mine"})

	s.DetailApplied("PR_1", detailWith("first", "second"))

	if got := bodies(s.Detail("PR_1")); len(got) != 3 || got[2] != "mine" {
		t.Errorf("timeline = %q, want the pending comment still on the end", got)
	}
}

func TestReadingADetailTwiceDoesNotDoubleThePending(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", detailWith("first"))
	s.PendingComment("PR_1", gh.Comment{Kind: gh.CommentIssue, Body: "mine"})

	_ = s.Detail("PR_1")
	if got := bodies(s.Detail("PR_1")); len(got) != 2 {
		t.Errorf("timeline = %q on the second read, want two comments", got)
	}
}

func TestAPostedCommentReplacesItsPlaceholder(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", detailWith("first"))
	key := s.PendingComment("PR_1", gh.Comment{Kind: gh.CommentIssue, Body: "mine"})

	s.PendingApplied("PR_1", key, gh.CommentResult{
		Comment: gh.Comment{Kind: gh.CommentIssue, ID: "IC_REAL", Body: "mine"},
	})

	d := s.Detail("PR_1")
	if got := bodies(d); len(got) != 2 || got[1] != "mine" {
		t.Fatalf("timeline = %q, want the comment once", got)
	}

	last := d.Detail.Timeline[1].Said()
	if last.Pending {
		t.Error("the comment is still marked pending after GitHub confirmed it")
	}
	if last.ID != "IC_REAL" {
		t.Errorf("ID = %q, want the node id GitHub gave it", last.ID)
	}
}

func TestAFailedPostTakesItsCommentBack(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", detailWith("first"))
	key := s.PendingComment("PR_1", gh.Comment{Kind: gh.CommentIssue, Body: "mine"})

	s.PendingReverted("PR_1", key)

	if got := bodies(s.Detail("PR_1")); len(got) != 1 || got[0] != "first" {
		t.Errorf("timeline = %q, want only what was fetched", got)
	}
}

func TestTwoWritesInFlightSettleSeparately(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", detailWith("first"))

	one := s.PendingComment("PR_1", gh.Comment{Kind: gh.CommentIssue, Body: "one"})
	two := s.PendingComment("PR_1", gh.Comment{Kind: gh.CommentIssue, Body: "two"})
	if one == two {
		t.Fatalf("both writes got the key %q", one)
	}

	s.PendingReverted("PR_1", one)
	s.PendingApplied("PR_1", two, gh.CommentResult{
		Comment: gh.Comment{Kind: gh.CommentIssue, ID: "IC_TWO", Body: "two"},
	})

	if got := bodies(s.Detail("PR_1")); len(got) != 2 || got[1] != "two" {
		t.Errorf("timeline = %q, want the first reverted and the second landed", got)
	}
}

func TestAResponseForAWriteAlreadySettledIsIgnored(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", detailWith("first"))
	key := s.PendingComment("PR_1", gh.Comment{Kind: gh.CommentIssue, Body: "mine"})

	landed := gh.CommentResult{Comment: gh.Comment{Kind: gh.CommentIssue, ID: "IC_REAL", Body: "mine"}}
	s.PendingApplied("PR_1", key, landed)
	s.PendingApplied("PR_1", key, landed)

	if got := bodies(s.Detail("PR_1")); len(got) != 2 {
		t.Errorf("timeline = %q, want the comment once", got)
	}
}

func TestAPendingCommentStaysOnItsOwnPullRequest(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", detailWith("first"))
	s.DetailApplied("PR_2", detailWith("other"))

	s.PendingComment("PR_1", gh.Comment{Kind: gh.CommentIssue, Body: "mine"})

	if got := bodies(s.Detail("PR_2")); len(got) != 1 || got[0] != "other" {
		t.Errorf("PR_2 timeline = %q, want only its own", got)
	}
}

func TestACommentARefetchAlreadyCarriesIsNotAddedTwice(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", detailWith("first"))
	key := s.PendingComment("PR_1", gh.Comment{Kind: gh.CommentIssue, Body: "mine"})

	s.DetailApplied("PR_1", detailWith("first", "mine"))

	s.PendingApplied("PR_1", key, gh.CommentResult{
		Comment: gh.Comment{Kind: gh.CommentIssue, ID: "IC_mine", Body: "mine"},
	})

	if got := bodies(s.Detail("PR_1")); len(got) != 2 {
		t.Errorf("timeline = %q, want the comment once", got)
	}
}

func replies(d store.Detail, threadID string) []string {
	for _, t := range d.Detail.Threads {
		if t.ID != threadID {
			continue
		}
		out := make([]string, len(t.Comments))
		for i, c := range t.Comments {
			out[i] = c.Body
		}
		return out
	}
	return nil
}

// threadWith appends comments from nil, as gh does, so the aliasing tests see spare capacity.
func threadWith(id string, bodies ...string) gh.DetailResult {
	var comments []gh.Comment
	for _, body := range bodies {
		comments = append(comments, gh.Comment{Kind: gh.CommentThread, ID: "RC_" + body, Body: body})
	}
	return gh.DetailResult{Detail: gh.PullRequestDetail{
		Threads: []gh.ReviewThread{{ID: id, CanReply: true, Comments: comments}},
	}}
}

func TestAPendingReplyRendersUnderItsThread(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "asked"))

	s.PendingReply("PR_1", "RT_1", gh.Comment{Body: "answered"})

	if got := replies(s.Detail("PR_1"), "RT_1"); len(got) != 2 || got[1] != "answered" {
		t.Errorf("thread = %q, want the reply on the end of it", got)
	}
	if got := bodies(s.Detail("PR_1")); len(got) != 0 {
		t.Errorf("timeline = %q, want the reply nowhere near it", got)
	}

	last := s.Detail("PR_1").Detail.Threads[0].Comments[1]
	if !last.Pending {
		t.Error("the pending reply is not marked pending")
	}
	if last.Kind != gh.CommentThread {
		t.Errorf("Kind = %q, want a thread comment whatever the caller passed", last.Kind)
	}
}

func TestARefetchDoesNotDropAReplyStillInFlight(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "asked"))
	s.PendingReply("PR_1", "RT_1", gh.Comment{Body: "answered"})

	s.DetailApplied("PR_1", threadWith("RT_1", "asked", "somebody else"))

	if got := replies(s.Detail("PR_1"), "RT_1"); len(got) != 3 || got[2] != "answered" {
		t.Errorf("thread = %q, want the reply still on the end", got)
	}
}

func TestFoldingAReplyDoesNotWriteIntoADetailAlreadyHandedOut(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "one", "two", "three"))

	first := s.PendingReply("PR_1", "RT_1", gh.Comment{Body: "mine"})
	held := s.Detail("PR_1")
	if got := replies(held, "RT_1"); len(got) != 4 || got[3] != "mine" {
		t.Fatalf("thread = %q, want the first reply folded in", got)
	}

	s.PendingReverted("PR_1", first)
	s.PendingReply("PR_1", "RT_1", gh.Comment{Body: "somebody else's"})
	_ = s.Detail("PR_1")

	if got := replies(held, "RT_1"); got[3] != "mine" {
		t.Errorf("the detail already handed out now reads %q, want it unchanged", got)
	}
}

func TestReadingADetailTwiceDoesNotDoubleAPendingReply(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "asked"))
	s.PendingReply("PR_1", "RT_1", gh.Comment{Body: "answered"})

	_ = s.Detail("PR_1")
	if got := replies(s.Detail("PR_1"), "RT_1"); len(got) != 2 {
		t.Errorf("thread = %q on the second read, want two comments", got)
	}
}

func TestTwoRepliesToOneThreadBothShow(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "asked"))
	s.PendingReply("PR_1", "RT_1", gh.Comment{Body: "first"})
	s.PendingReply("PR_1", "RT_1", gh.Comment{Body: "second"})

	if got := replies(s.Detail("PR_1"), "RT_1"); len(got) != 3 {
		t.Errorf("thread = %q, want both replies on it", got)
	}
}

func TestAPostedReplyReplacesItsPlaceholder(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "asked"))
	key := s.PendingReply("PR_1", "RT_1", gh.Comment{Body: "answered"})

	s.PendingApplied("PR_1", key, gh.CommentResult{
		Comment: gh.Comment{Kind: gh.CommentThread, ID: "RC_REAL", Body: "answered"},
	})

	got := s.Detail("PR_1").Detail.Threads[0].Comments
	if len(got) != 2 {
		t.Fatalf("thread has %d comments, want the reply once", len(got))
	}
	if got[1].ID != "RC_REAL" || got[1].Pending {
		t.Errorf("comment = %+v, want the id GitHub gave it and no marker", got[1])
	}
}

func TestAFailedReplyTakesItsCommentBack(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "asked"))
	key := s.PendingReply("PR_1", "RT_1", gh.Comment{Body: "answered"})

	s.PendingReverted("PR_1", key)

	if got := replies(s.Detail("PR_1"), "RT_1"); len(got) != 1 {
		t.Errorf("thread = %q, want the reply gone", got)
	}
}

func TestAReplyARefetchAlreadyCarriesIsNotAddedTwice(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "asked"))
	key := s.PendingReply("PR_1", "RT_1", gh.Comment{Body: "answered"})

	s.DetailApplied("PR_1", threadWith("RT_1", "asked", "answered"))
	s.PendingApplied("PR_1", key, gh.CommentResult{
		Comment: gh.Comment{Kind: gh.CommentThread, ID: "RC_answered", Body: "answered"},
	})

	if got := replies(s.Detail("PR_1"), "RT_1"); len(got) != 2 {
		t.Errorf("thread = %q, want the reply once", got)
	}
}

func TestAReplyToAThreadTheRefetchDroppedIsDiscarded(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "asked"))
	key := s.PendingReply("PR_1", "RT_1", gh.Comment{Body: "answered"})

	s.DetailApplied("PR_1", threadWith("RT_OTHER", "elsewhere"))

	if got := replies(s.Detail("PR_1"), "RT_OTHER"); len(got) != 1 {
		t.Errorf("thread = %q, want the reply nowhere on it", got)
	}

	s.PendingApplied("PR_1", key, gh.CommentResult{
		Comment: gh.Comment{Kind: gh.CommentThread, ID: "RC_REAL", Body: "answered"},
	})
	if got := replies(s.Detail("PR_1"), "RT_OTHER"); len(got) != 1 {
		t.Errorf("thread = %q, want the reply discarded with its thread", got)
	}
}

func TestACommentAndAReplyInFlightSettleSeparately(t *testing.T) {
	s := store.New(configured())
	d := threadWith("RT_1", "asked")
	d.Detail.Timeline = detailWith("first").Detail.Timeline
	s.DetailApplied("PR_1", d)

	comment := s.PendingComment("PR_1", gh.Comment{Kind: gh.CommentIssue, Body: "loose"})
	reply := s.PendingReply("PR_1", "RT_1", gh.Comment{Body: "answered"})

	if got := bodies(s.Detail("PR_1")); len(got) != 2 || got[1] != "loose" {
		t.Errorf("timeline = %q, want only the comment on it", got)
	}
	if got := replies(s.Detail("PR_1"), "RT_1"); len(got) != 2 || got[1] != "answered" {
		t.Errorf("thread = %q, want only the reply on it", got)
	}

	s.PendingReverted("PR_1", comment)
	s.PendingApplied("PR_1", reply, gh.CommentResult{
		Comment: gh.Comment{Kind: gh.CommentThread, ID: "RC_REAL", Body: "answered"},
	})

	if got := bodies(s.Detail("PR_1")); len(got) != 1 {
		t.Errorf("timeline = %q, want the comment taken back", got)
	}
	if got := replies(s.Detail("PR_1"), "RT_1"); len(got) != 2 {
		t.Errorf("thread = %q, want the reply landed", got)
	}
}

func threadIn(t *testing.T, d store.Detail, id string) gh.ReviewThread {
	t.Helper()

	for _, th := range d.Detail.Threads {
		if th.ID == id {
			return th
		}
	}
	t.Fatalf("no thread %q in the detail", id)
	return gh.ReviewThread{}
}

func TestAPendingResolveShowsResolvedBeforeItLands(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "asked"))

	s.PendingResolve("PR_1", "RT_1", true)

	if !threadIn(t, s.Detail("PR_1"), "RT_1").IsResolved {
		t.Error("the thread reads open, want it resolved while the write is out")
	}
}

func TestAPendingResolveLeavesThePermissionsAlone(t *testing.T) {
	s := store.New(configured())
	d := threadWith("RT_1", "asked")
	d.Detail.Threads[0].CanResolve = true
	s.DetailApplied("PR_1", d)

	s.PendingResolve("PR_1", "RT_1", true)

	got := threadIn(t, s.Detail("PR_1"), "RT_1")
	if !got.CanResolve || got.CanUnresolve {
		t.Errorf("permissions = %v/%v, want them as GitHub last sent them", got.CanResolve, got.CanUnresolve)
	}
}

func TestARefetchDoesNotDropAResolveStillInFlight(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "asked"))
	s.PendingResolve("PR_1", "RT_1", true)

	s.DetailApplied("PR_1", threadWith("RT_1", "asked", "somebody else"))

	if !threadIn(t, s.Detail("PR_1"), "RT_1").IsResolved {
		t.Error("the refetch took the resolve back off a write still in flight")
	}
}

func TestFoldingAResolveDoesNotWriteIntoADetailAlreadyHandedOut(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "asked"))

	held := s.Detail("PR_1")
	s.PendingResolve("PR_1", "RT_1", true)
	_ = s.Detail("PR_1")

	if threadIn(t, held, "RT_1").IsResolved {
		t.Error("the detail already handed out now reads resolved, want it unchanged")
	}
}

func TestAResolveAndAReplyInFlightBothFoldIn(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "asked"))

	s.PendingReply("PR_1", "RT_1", gh.Comment{Body: "answered"})
	s.PendingResolve("PR_1", "RT_1", true)

	got := threadIn(t, s.Detail("PR_1"), "RT_1")
	if !got.IsResolved {
		t.Error("the thread reads open, want it resolved")
	}
	if len(got.Comments) != 2 || got.Comments[1].Body != "answered" {
		t.Errorf("thread = %q, want the reply still on it", replies(s.Detail("PR_1"), "RT_1"))
	}
}

func TestAPostedResolveTakesGitHubsPermissionsBack(t *testing.T) {
	s := store.New(configured())
	d := threadWith("RT_1", "asked")
	d.Detail.Threads[0].CanResolve = true
	s.DetailApplied("PR_1", d)

	key := s.PendingResolve("PR_1", "RT_1", true)
	s.ResolveApplied("PR_1", key, gh.ThreadResult{ID: "RT_1", IsResolved: true, CanUnresolve: true})

	got := threadIn(t, s.Detail("PR_1"), "RT_1")
	if !got.IsResolved || got.CanResolve || !got.CanUnresolve {
		t.Errorf("thread = %+v, want it resolved with the permissions flipped", got)
	}
}

func TestReconcilingAResolveDoesNotWriteIntoADetailAlreadyHandedOut(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "asked"))

	held := s.Detail("PR_1")
	key := s.PendingResolve("PR_1", "RT_1", true)
	s.ResolveApplied("PR_1", key, gh.ThreadResult{ID: "RT_1", IsResolved: true, CanUnresolve: true})

	if threadIn(t, held, "RT_1").IsResolved {
		t.Error("the detail already handed out now reads resolved, want it unchanged")
	}
}

func TestAFailedResolvePutsTheThreadBack(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "asked"))
	key := s.PendingResolve("PR_1", "RT_1", true)

	s.ResolveReverted("PR_1", key)

	if threadIn(t, s.Detail("PR_1"), "RT_1").IsResolved {
		t.Error("the thread still reads resolved, want the write taken back")
	}
}

func TestAResolveForAThreadTheRefetchDroppedIsDiscarded(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "asked"))
	key := s.PendingResolve("PR_1", "RT_1", true)

	s.DetailApplied("PR_1", threadWith("RT_OTHER", "elsewhere"))
	s.ResolveApplied("PR_1", key, gh.ThreadResult{ID: "RT_1", IsResolved: true})

	if got := s.Detail("PR_1").Detail.Threads; len(got) != 1 || got[0].ID != "RT_OTHER" {
		t.Errorf("threads = %+v, want only the one the refetch carries", got)
	}
}

func TestACommentAndAResolveInFlightSettleSeparately(t *testing.T) {
	s := store.New(configured())
	d := threadWith("RT_1", "asked")
	d.Detail.Timeline = detailWith("first").Detail.Timeline
	s.DetailApplied("PR_1", d)

	comment := s.PendingComment("PR_1", gh.Comment{Kind: gh.CommentIssue, Body: "loose"})
	resolve := s.PendingResolve("PR_1", "RT_1", true)
	if comment == resolve {
		t.Fatalf("both writes were held under %q", comment)
	}

	s.PendingReverted("PR_1", comment)

	if got := bodies(s.Detail("PR_1")); len(got) != 1 {
		t.Errorf("timeline = %q, want the comment taken back", got)
	}
	if !threadIn(t, s.Detail("PR_1"), "RT_1").IsResolved {
		t.Error("the resolve went with the comment, want it still in flight")
	}
}

func TestAThreadWithAResolveInFlightIsMarkedPending(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "asked"))

	key := s.PendingResolve("PR_1", "RT_1", true)
	if !threadIn(t, s.Detail("PR_1"), "RT_1").Pending {
		t.Fatal("the thread is not marked while the write is out")
	}

	s.ResolveApplied("PR_1", key, gh.ThreadResult{ID: "RT_1", IsResolved: true, CanUnresolve: true})
	if threadIn(t, s.Detail("PR_1"), "RT_1").Pending {
		t.Error("the thread is still marked once the write landed")
	}
}

func TestAFailedResolveTakesTheMarkerWithIt(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "asked"))

	key := s.PendingResolve("PR_1", "RT_1", true)
	s.ResolveReverted("PR_1", key)

	if threadIn(t, s.Detail("PR_1"), "RT_1").Pending {
		t.Error("the thread is still marked after the write was taken back")
	}
}
