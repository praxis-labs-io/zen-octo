package store_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/store"
)

// open is one pull request read: fetched, then answered.
func open(s *store.Store, id string) {
	s.BeginDetail(id)
	s.DetailApplied(id, detailResult(id, 4800))
}

// openMany reads n of them in order, PR_0 first, and answers with the ids.
func openMany(s *store.Store, n int) []string {
	ids := make([]string, n)
	for i := range n {
		ids[i] = "PR_" + strconv.Itoa(i)
		open(s, ids[i])
	}
	return ids
}

// A terminal client left open all day reads tens of pull requests, and a detail
// on a heavily reviewed one is megabytes. Nothing used to free any of it.
func TestADetailPastTheCapIsDropped(t *testing.T) {
	s := store.New(configured())
	ids := openMany(&s, store.DetailCap+1)

	if got := s.Cached(); got != store.DetailCap {
		t.Errorf("the store holds %d details, want it inside its cap of %d", got, store.DetailCap)
	}
	if first := s.Detail(ids[0]); first.Loaded {
		t.Error("the pull request read first is still held past the cap")
	}
	if last := s.Detail(ids[len(ids)-1]); !last.Loaded {
		t.Error("the pull request read last was dropped, which is the one on screen")
	}
}

// Reading one again is reading it, so it goes to the back of the queue rather
// than staying where it first landed.
func TestReadingADetailAgainKeepsIt(t *testing.T) {
	s := store.New(configured())
	ids := openMany(&s, store.DetailCap)

	open(&s, ids[0])
	open(&s, "PR_new")

	if first := s.Detail(ids[0]); !first.Loaded {
		t.Error("the pull request read again was dropped as though it never had been")
	}
	if second := s.Detail(ids[1]); second.Loaded {
		t.Error("the oldest read is still held, so reading again moved nothing")
	}
}

// Detail folds a write in flight over the held detail at read time. Over an
// evicted one it folds over nothing, and an optimistic comment loses its page.
func TestADetailWithAWriteInFlightIsNotDropped(t *testing.T) {
	s := store.New(configured())
	open(&s, "PR_writing")
	s.PendingComment("PR_writing", gh.Comment{ID: "c1", Body: "on its way"})

	openMany(&s, store.DetailCap+1)

	held := s.Detail("PR_writing")
	if held.Detail.ID != "PR_writing" {
		t.Fatalf("the pull request being written to was dropped: %+v", held.Detail.PullRequest)
	}
	if len(held.Detail.Timeline) == 0 {
		t.Error("the comment in flight folded onto nothing")
	}
}

// Its response is still coming, and would land on a slot the cache no longer
// carries any bookkeeping for.
func TestADetailBeingFetchedIsNotDropped(t *testing.T) {
	s := store.New(configured())
	s.BeginDetail("PR_fetching")

	openMany(&s, store.DetailCap+1)

	if !s.BeginDetail("PR_fetching") == false {
		t.Error("the fetch in flight was dropped with the detail it was for")
	}
}

// A debt is owed about a pull request. Dropped with it, or the mark outlives
// what it was about and the next open answers a question nobody asked.
func TestAnEvictedDetailTakesItsDebtsWithIt(t *testing.T) {
	s := store.New(configured())
	open(&s, "PR_owing")
	s.BeginPulse("PR_owing")
	s.PulseApplied("PR_owing", gh.PulseResult{Pulse: gh.Pulse{UpdatedAt: time.Now()}})
	if !s.StaleTimeline("PR_owing") {
		t.Fatal("setup: the pulse left no debt to drop")
	}

	openMany(&s, store.DetailCap+1)

	if s.StaleTimeline("PR_owing") {
		t.Error("the debt outlived the detail it was owed for")
	}
}

// A pin outranks a cap: the alternative is dropping what a write is about to
// land on, and a write cannot be told to wait.
func TestACacheOfNothingButPinnedGoesOverItsCap(t *testing.T) {
	s := store.New(configured())

	// Pinned as each is read, or the cache has already dropped the early ones by
	// the time the last is pinned.
	ids := make([]string, store.DetailCap+5)
	for i := range ids {
		ids[i] = "PR_" + strconv.Itoa(i)
		open(&s, ids[i])
		s.PendingComment(ids[i], gh.Comment{ID: "c", Body: "on its way"})
	}
	open(&s, "PR_last")

	for _, id := range ids {
		if !s.Detail(id).Loaded {
			t.Fatalf("%s was dropped with a write in flight for it", id)
		}
	}
}

// Every diff opened used to be held for the session, and a branch walked in the
// Commits tab fetches one per commit the cursor rests on.
func TestADiffPastItsCapIsDropped(t *testing.T) {
	tests := []struct {
		name  string
		cap   int
		begin func(*store.Store, string) bool
		apply func(*store.Store, string, gh.FilesResult)
		held  func(store.Store, string) store.Files
	}{
		{
			"a pull request's own",
			store.FilesCap,
			(*store.Store).BeginFiles,
			(*store.Store).FilesApplied,
			store.Store.Files,
		},
		{
			"a commit's",
			store.CommitCap,
			(*store.Store).BeginCommitFiles,
			(*store.Store).CommitFilesApplied,
			store.Store.CommitFiles,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := store.New(configured())

			keys := make([]string, tt.cap+1)
			for i := range keys {
				keys[i] = "key_" + strconv.Itoa(i)
				tt.begin(&s, keys[i])
				tt.apply(&s, keys[i], gh.FilesResult{Files: []gh.ChangedFile{{Path: "main.go"}}})
			}

			if first := tt.held(s, keys[0]); first.Loaded {
				t.Error("the diff read first is still held past the cap")
			}
			if last := tt.held(s, keys[len(keys)-1]); !last.Loaded {
				t.Error("the diff read last was dropped, which is the one on screen")
			}
		})
	}
}

// The fetch in flight is the diff's only pin, and its answer needs the slot.
func TestADiffBeingFetchedIsNotDropped(t *testing.T) {
	s := store.New(configured())
	s.BeginFiles("PR_fetching")

	for i := range store.FilesCap + 1 {
		key := "PR_" + strconv.Itoa(i)
		s.BeginFiles(key)
		s.FilesApplied(key, gh.FilesResult{Files: []gh.ChangedFile{{Path: "main.go"}}})
	}

	if s.BeginFiles("PR_fetching") {
		t.Error("the fetch in flight was dropped with the diff it was for")
	}
}

// openOnCopy is how half this package's writers reach it: a value receiver on
// the model, so the Store they write is a copy that is thrown away.
func openOnCopy(s store.Store) { open(&s, "PR_onacopy") }

// The maps a cache is made of are built by New rather than by whichever write
// went first. Built on a copy, the first detail of a session goes with it.
func TestTheCacheSurvivesTheCopyItIsFirstWrittenOn(t *testing.T) {
	s := store.New(configured())

	openOnCopy(s)

	if !s.Detail("PR_onacopy").Loaded {
		t.Error("the first detail of the session was written into a map the copy took with it")
	}
}
