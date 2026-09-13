package store_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
)

func open(s *store.Store, id string) {
	s.BeginDetail(id)
	s.DetailApplied(id, detailResult(id, 4800))
}

func openMany(s *store.Store, n int) []string {
	ids := make([]string, n)
	for i := range n {
		ids[i] = "PR_" + strconv.Itoa(i)
		open(s, ids[i])
	}
	return ids
}

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

func TestADetailBeingFetchedIsNotDropped(t *testing.T) {
	s := store.New(configured())
	s.BeginDetail("PR_fetching")

	openMany(&s, store.DetailCap+1)

	if s.BeginDetail("PR_fetching") {
		t.Error("the fetch in flight was dropped with the detail it was for")
	}
}

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

func TestAnEvictedDiffTakesItsDebtWithIt(t *testing.T) {
	s := store.New(configured())
	open(&s, "PR_owing")
	s.BeginFiles("PR_owing")
	s.FilesApplied("PR_owing", oneFile())

	s.BeginPulse("PR_owing")
	s.PulseApplied("PR_owing", gh.PulseResult{Pulse: gh.Pulse{HeadRefOid: "deadbee"}})
	if !s.StaleFiles("PR_owing") {
		t.Fatal("setup: the push left no debt to drop")
	}

	for i := range store.FilesCap + 1 {
		key := "PR_" + strconv.Itoa(i)
		s.BeginFiles(key)
		s.FilesApplied(key, oneFile())
	}

	if s.StaleFiles("PR_owing") {
		t.Error("the debt outlived the diff it was owed for")
	}
}

func TestTheRowStampsAreBoundedWithTheDetails(t *testing.T) {
	s := store.New(configured())
	openMany(&s, store.DetailCap*2)

	if got := s.RowStamps(); got > store.DetailCap {
		t.Errorf("%d row stamps held for %d details, want one per detail", got, s.Cached())
	}
}

func TestADiffReadAgainIsNotTheFirstDropped(t *testing.T) {
	s := store.New(configured())

	keys := make([]string, store.CommitCap)
	for i := range keys {
		keys[i] = "sha_" + strconv.Itoa(i)
		s.BeginCommitFiles(keys[i])
		s.CommitFilesApplied(keys[i], oneFile())
	}

	s.UseCommitFiles(keys[0])
	s.BeginCommitFiles("sha_new")
	s.CommitFilesApplied("sha_new", oneFile())

	if !s.CommitFiles(keys[0]).Loaded {
		t.Error("the commit read again was dropped as though it had only been fetched")
	}
	if s.CommitFiles(keys[1]).Loaded {
		t.Error("the oldest read is still held, so reading again moved nothing")
	}
}

func oneFile() gh.FilesResult {
	return gh.FilesResult{Files: []gh.ChangedFile{{Path: "main.go"}}}
}

func TestACacheOfNothingButPinnedGoesOverItsCap(t *testing.T) {
	s := store.New(configured())

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

func openOnCopy(s store.Store) { open(&s, "PR_onacopy") }

func TestTheCacheSurvivesTheCopyItIsFirstWrittenOn(t *testing.T) {
	s := store.New(configured())

	openOnCopy(s)

	if !s.Detail("PR_onacopy").Loaded {
		t.Error("the first detail of the session was written into a map the copy took with it")
	}
}
