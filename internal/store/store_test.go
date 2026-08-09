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

// bodies is the comment bodies on a detail's timeline, in order, which is what
// the conversation would render.
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

// A comment written here shows in the conversation before GitHub has seen it.
// That is the whole of what optimistic means.
func TestAPendingCommentRendersBeforeItLands(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", detailWith("first"))

	s.PendingComment("PR_1", gh.Comment{Kind: gh.CommentIssue, Body: "mine"})

	if got := bodies(s.Detail("PR_1")); len(got) != 2 || got[1] != "mine" {
		t.Errorf("timeline = %q, want the pending comment at the end", got)
	}

	// It says it has not landed. A placeholder that looks like the real thing
	// is a lie the moment the post fails.
	last := s.Detail("PR_1").Detail.Timeline[1].Said()
	if !last.Pending {
		t.Error("the pending comment is not marked pending")
	}
}

// The reason pending is held beside the detail rather than written into it. A
// refetch that answers before the mutation does must not take the comment away.
func TestARefetchDoesNotDropACommentStillInFlight(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", detailWith("first"))
	s.PendingComment("PR_1", gh.Comment{Kind: gh.CommentIssue, Body: "mine"})

	s.DetailApplied("PR_1", detailWith("first", "second"))

	if got := bodies(s.Detail("PR_1")); len(got) != 3 || got[2] != "mine" {
		t.Errorf("timeline = %q, want the pending comment still on the end", got)
	}
}

// Reading twice gives the same answer. Folding pending into the held slice
// rather than a copy would append it again on every call.
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

// The revert branch. A post that fails takes its comment back off the screen
// rather than leaving a card nothing will ever confirm.
func TestAFailedPostTakesItsCommentBack(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", detailWith("first"))
	key := s.PendingComment("PR_1", gh.Comment{Kind: gh.CommentIssue, Body: "mine"})

	s.PendingReverted("PR_1", key)

	if got := bodies(s.Detail("PR_1")); len(got) != 1 || got[0] != "first" {
		t.Errorf("timeline = %q, want only what was fetched", got)
	}
}

// Two comments in flight settle independently, and each names its own.
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

// A response for a key already settled is a second copy of an answer. Applying
// it would put the comment in the conversation twice.
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

// Writes belong to the pull request they were written on. One open in a second
// screen must not show the other's placeholder.
func TestAPendingCommentStaysOnItsOwnPullRequest(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", detailWith("first"))
	s.DetailApplied("PR_2", detailWith("other"))

	s.PendingComment("PR_1", gh.Comment{Kind: gh.CommentIssue, Body: "mine"})

	if got := bodies(s.Detail("PR_2")); len(got) != 1 || got[0] != "other" {
		t.Errorf("PR_2 timeline = %q, want only its own", got)
	}
}

// A refetch that lands while the write is out already carries the comment.
// Adding it again puts the same one on the page twice, and the two cards share
// a node id, which is the one thing the focus ring cannot survive.
func TestACommentARefetchAlreadyCarriesIsNotAddedTwice(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", detailWith("first"))
	key := s.PendingComment("PR_1", gh.Comment{Kind: gh.CommentIssue, Body: "mine"})

	// GitHub recorded it and the refetch answered before the mutation did.
	s.DetailApplied("PR_1", detailWith("first", "mine"))

	s.PendingApplied("PR_1", key, gh.CommentResult{
		Comment: gh.Comment{Kind: gh.CommentIssue, ID: "IC_mine", Body: "mine"},
	})

	if got := bodies(s.Detail("PR_1")); len(got) != 2 {
		t.Errorf("timeline = %q, want the comment once", got)
	}
}

// replies is the comment bodies on one thread, in order, which is what the
// thread's card would render.
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

// threadWith is a detail carrying one review thread and the comments on it.
//
// The comments are appended from nil rather than sized exactly, because that is
// what the gh package does and the difference is the whole of the aliasing test
// below: a slice with no spare capacity reallocates on every append, which hides
// a write that would otherwise land in the held detail.
func threadWith(id string, bodies ...string) gh.DetailResult {
	var comments []gh.Comment
	for _, body := range bodies {
		comments = append(comments, gh.Comment{Kind: gh.CommentThread, ID: "RC_" + body, Body: body})
	}
	return gh.DetailResult{Detail: gh.PullRequestDetail{
		Threads: []gh.ReviewThread{{ID: id, CanReply: true, Comments: comments}},
	}}
}

// A reply hangs off the thread it answers, not off the end of the timeline.
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

// The aliasing bug the two clones are there for. Cloning the threads copies the
// structs, and each one's comments are still the held slice; a thread with spare
// capacity takes the append in place, into the array every other caller is
// reading. A detail already handed out then changes under whoever is holding it,
// which on this screen is a rendered conversation.
func TestFoldingAReplyDoesNotWriteIntoADetailAlreadyHandedOut(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "one", "two", "three"))

	first := s.PendingReply("PR_1", "RT_1", gh.Comment{Body: "mine"})
	held := s.Detail("PR_1")
	if got := replies(held, "RT_1"); len(got) != 4 || got[3] != "mine" {
		t.Fatalf("thread = %q, want the first reply folded in", got)
	}

	// A second write folds into the same slot on the next read.
	s.PendingReverted("PR_1", first)
	s.PendingReply("PR_1", "RT_1", gh.Comment{Body: "somebody else's"})
	_ = s.Detail("PR_1")

	if got := replies(held, "RT_1"); got[3] != "mine" {
		t.Errorf("the detail already handed out now reads %q, want it unchanged", got)
	}
}

// Reading twice gives the same answer.
func TestReadingADetailTwiceDoesNotDoubleAPendingReply(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "asked"))
	s.PendingReply("PR_1", "RT_1", gh.Comment{Body: "answered"})

	_ = s.Detail("PR_1")
	if got := replies(s.Detail("PR_1"), "RT_1"); len(got) != 2 {
		t.Errorf("thread = %q on the second read, want two comments", got)
	}
}

// Two replies to one thread are two writes, and the second must not clobber the
// slice the first was folded into.
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

// A refetch that landed while the reply was out already carries it. Adding it
// again gives the thread two comments sharing a node id.
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

// The thread went away under the write: resolved and hidden, or off the first
// page. The refetch is the truer picture, and there is nowhere honest to put it.
func TestAReplyToAThreadTheRefetchDroppedIsDiscarded(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "asked"))
	key := s.PendingReply("PR_1", "RT_1", gh.Comment{Body: "answered"})

	s.DetailApplied("PR_1", threadWith("RT_OTHER", "elsewhere"))

	// Nothing to fold it into while it is out, and nowhere to land when it
	// answers. Neither is a panic.
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

// A comment and a reply in flight together settle in different places, and each
// answer has to find its own.
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
