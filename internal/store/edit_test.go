package store_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/store"
)

func labelled(names ...string) gh.DetailResult {
	labels := make([]gh.Label, len(names))
	for i, n := range names {
		labels[i] = gh.Label{ID: "LA_" + n, Name: n}
	}
	return gh.DetailResult{Detail: gh.PullRequestDetail{Labels: labels}}
}

func labelSet(names ...string) []gh.Label {
	out := make([]gh.Label, len(names))
	for i, n := range names {
		out[i] = gh.Label{ID: "LA_" + n, Name: n}
	}
	return out
}

func labelNames(d store.Detail) []string {
	out := make([]string, 0, len(d.Detail.Labels))
	for _, l := range d.Detail.Labels {
		out = append(out, l.Name)
	}
	return out
}

func TestAPendingLabelSetRendersBeforeItLands(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", labelled("bug"))

	s.PendingLabels("PR_1", labelSet("bug", "urgent"))

	if got, want := labelNames(s.Detail("PR_1")), []string{"bug", "urgent"}; !slices.Equal(got, want) {
		t.Errorf("labels = %q, want %q", got, want)
	}
}

// Unchecking the last label is a real write. An empty set folding as "no edit"
// would leave the label on the screen with nothing to say why.
func TestAPendingEmptyLabelSetClearsThem(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", labelled("bug"))

	s.PendingLabels("PR_1", nil)

	if got := labelNames(s.Detail("PR_1")); len(got) != 0 {
		t.Errorf("labels = %q, want none", got)
	}
}

// The reason an edit is held beside the detail rather than written into it.
func TestARefetchDoesNotDropALabelEditStillInFlight(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", labelled("bug"))
	s.PendingLabels("PR_1", labelSet("bug", "urgent"))

	s.DetailApplied("PR_1", labelled("bug"))

	if got, want := labelNames(s.Detail("PR_1")), []string{"bug", "urgent"}; !slices.Equal(got, want) {
		t.Errorf("labels = %q, want the edit still folded over the refetch", got)
	}
}

func TestLabelsAppliedTakesGitHubsAnswer(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", labelled("bug"))
	key := s.PendingLabels("PR_1", labelSet("bug", "urgent"))

	// GitHub kept one of the two. Its answer is the authority, not the ask.
	s.LabelsApplied("PR_1", key, gh.LabelsResult{Labels: labelSet("urgent")})

	if got, want := labelNames(s.Detail("PR_1")), []string{"urgent"}; !slices.Equal(got, want) {
		t.Errorf("labels = %q, want %q", got, want)
	}
}

func TestARevertedLabelEditPutsTheFetchedSetBack(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", labelled("bug"))
	key := s.PendingLabels("PR_1", labelSet("bug", "urgent"))

	s.EditReverted("PR_1", key)

	if got, want := labelNames(s.Detail("PR_1")), []string{"bug"}; !slices.Equal(got, want) {
		t.Errorf("labels = %q, want the fetched set back", got)
	}
}

// Two writes out on one field settle last-held wins, which is the order they
// were pressed in. A map keyed by field would lose the first one's key and
// leave its response with nothing to settle.
func TestTwoLabelEditsInFlightSettleByKey(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", labelled("bug"))

	first := s.PendingLabels("PR_1", labelSet("urgent"))
	second := s.PendingLabels("PR_1", labelSet("urgent", "docs"))

	if first == second {
		t.Fatalf("both writes took the key %q", first)
	}
	if got, want := labelNames(s.Detail("PR_1")), []string{"urgent", "docs"}; !slices.Equal(got, want) {
		t.Errorf("labels = %q, want the later edit on top", got)
	}

	// The first answers late and fails. The second is still the reader's ask.
	s.EditReverted("PR_1", first)
	if got, want := labelNames(s.Detail("PR_1")), []string{"urgent", "docs"}; !slices.Equal(got, want) {
		t.Errorf("labels = %q after the first reverted, want %q", got, want)
	}
}

// A response for a key already settled must not apply twice.
func TestASettledLabelEditIgnoresASecondResponse(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", labelled("bug"))
	key := s.PendingLabels("PR_1", labelSet("urgent"))

	s.LabelsApplied("PR_1", key, gh.LabelsResult{Labels: labelSet("urgent")})
	s.LabelsApplied("PR_1", key, gh.LabelsResult{Labels: labelSet("nonsense")})

	if got, want := labelNames(s.Detail("PR_1")), []string{"urgent"}; !slices.Equal(got, want) {
		t.Errorf("labels = %q, want the second response ignored", got)
	}
}

// The fold hands out a clone. Writing into what a reader was handed must not
// reach the edit still in flight, and a write in place is the one that would:
// an append past a full slice reallocates and hides the sharing.
func TestWritingIntoAFoldedLabelSetDoesNotReachTheStore(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", labelled("bug"))
	s.PendingLabels("PR_1", labelSet("bug", "urgent"))

	first := s.Detail("PR_1")
	first.Detail.Labels[0] = gh.Label{Name: "smuggled"}

	if got, want := labelNames(s.Detail("PR_1")), []string{"bug", "urgent"}; !slices.Equal(got, want) {
		t.Errorf("labels = %q on the second read, want %q", got, want)
	}
}

// The same guarantee on the way in. A caller that reuses the slice it handed to
// PendingLabels must not be editing what the store is holding.
func TestWritingIntoTheSliceHandedToPendingLabelsDoesNotReachTheStore(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", labelled("bug"))

	asked := labelSet("bug", "urgent")
	s.PendingLabels("PR_1", asked)
	asked[0] = gh.Label{Name: "smuggled"}

	if got, want := labelNames(s.Detail("PR_1")), []string{"bug", "urgent"}; !slices.Equal(got, want) {
		t.Errorf("labels = %q, want %q", got, want)
	}
}

func TestRepoMetaIsHeldForTheNextPicker(t *testing.T) {
	s := store.New(configured())

	if !s.BeginRepoMeta("zen-octo/zen-octo") {
		t.Fatal("BeginRepoMeta refused a repository never fetched")
	}
	s.RepoMetaApplied("zen-octo/zen-octo", gh.RepoMetaResult{
		Meta: gh.RepoMeta{Labels: labelSet("bug")},
	})

	held := s.Repo("zen-octo/zen-octo")
	if !held.Loaded {
		t.Error("metadata is not marked loaded")
	}
	if got, want := len(held.Meta.Labels), 1; got != want {
		t.Errorf("labels = %d, want %d", got, want)
	}

	// Refused twice over: already loaded, so a second picker costs nothing.
	if s.BeginRepoMeta("zen-octo/zen-octo") {
		t.Error("BeginRepoMeta started a second request for metadata already held")
	}
}

func TestBeginRepoMetaRefusesOneAlreadyInFlight(t *testing.T) {
	s := store.New(configured())

	if !s.BeginRepoMeta("zen-octo/zen-octo") {
		t.Fatal("the first BeginRepoMeta was refused")
	}
	if s.BeginRepoMeta("zen-octo/zen-octo") {
		t.Error("BeginRepoMeta started a second request while one was in flight")
	}
}

func TestInvalidateRepoMetaLetsTheNextPickerAskAgain(t *testing.T) {
	s := store.New(configured())
	s.BeginRepoMeta("zen-octo/zen-octo")
	s.RepoMetaApplied("zen-octo/zen-octo", gh.RepoMetaResult{Meta: gh.RepoMeta{Labels: labelSet("bug")}})

	s.InvalidateRepoMeta("zen-octo/zen-octo")

	if !s.BeginRepoMeta("zen-octo/zen-octo") {
		t.Error("BeginRepoMeta still refuses after the metadata was invalidated")
	}
}

func TestFailedRepoMetaCarriesItsError(t *testing.T) {
	s := store.New(configured())
	boom := errors.New("boom")

	s.BeginRepoMeta("zen-octo/zen-octo")
	s.RepoMetaFailed("zen-octo/zen-octo", boom)

	held := s.Repo("zen-octo/zen-octo")
	if !errors.Is(held.Err, boom) {
		t.Errorf("err = %v, want %v", held.Err, boom)
	}
	if held.Loaded {
		t.Error("a failed first fetch is marked loaded")
	}
}

// Two writes settle in whatever order the network gives them. The earlier one
// answering last must not overwrite the reader's newer ask with a set they have
// already moved on from.
func TestAnEarlierLabelResponseDoesNotOverwriteALaterEdit(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", labelled("bug"))

	first := s.PendingLabels("PR_1", labelSet("urgent"))
	s.PendingLabels("PR_1", labelSet("urgent", "docs"))

	// The first write answers last, carrying the set nobody is asking for now.
	s.LabelsApplied("PR_1", first, gh.LabelsResult{Labels: labelSet("urgent")})

	if got, want := labelNames(s.Detail("PR_1")), []string{"urgent", "docs"}; !slices.Equal(got, want) {
		t.Errorf("labels = %q, want the later edit still showing", got)
	}
}

// Once the last write settles, GitHub's answer is the authority again.
func TestTheLastLabelResponseWritesTheHeldSet(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", labelled("bug"))

	first := s.PendingLabels("PR_1", labelSet("urgent"))
	second := s.PendingLabels("PR_1", labelSet("urgent", "docs"))

	s.LabelsApplied("PR_1", first, gh.LabelsResult{Labels: labelSet("urgent")})
	s.LabelsApplied("PR_1", second, gh.LabelsResult{Labels: labelSet("urgent", "docs")})

	if got, want := labelNames(s.Detail("PR_1")), []string{"urgent", "docs"}; !slices.Equal(got, want) {
		t.Errorf("labels = %q, want %q", got, want)
	}
}
