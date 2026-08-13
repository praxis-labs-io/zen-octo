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

func staged(state gh.PRState, draft bool) gh.DetailResult {
	return gh.DetailResult{Detail: gh.PullRequestDetail{
		PullRequest: gh.PullRequest{State: state, IsDraft: draft},
	}}
}

func lifecycle(d store.Detail) (gh.PRState, bool) {
	return d.Detail.State, d.Detail.IsDraft
}

func TestAPendingTransitionRendersBeforeItLands(t *testing.T) {
	tests := []struct {
		name      string
		from      gh.PRState
		wasDraft  bool
		to        gh.PRTransition
		want      gh.PRState
		wantDraft bool
	}{
		{"ready", gh.PRStateOpen, true, gh.TransitionReady, gh.PRStateOpen, false},
		{"draft", gh.PRStateOpen, false, gh.TransitionDraft, gh.PRStateOpen, true},
		{"close", gh.PRStateOpen, false, gh.TransitionClose, gh.PRStateClosed, false},
		{"reopen", gh.PRStateClosed, false, gh.TransitionReopen, gh.PRStateOpen, false},
		// Closing a draft leaves it a draft, and reopening gives that back.
		{"close a draft", gh.PRStateOpen, true, gh.TransitionClose, gh.PRStateClosed, true},
		{"reopen a draft", gh.PRStateClosed, true, gh.TransitionReopen, gh.PRStateOpen, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := store.New(configured())
			s.DetailApplied("PR_1", staged(tt.from, tt.wasDraft))

			s.PendingState("PR_1", tt.to)

			state, draft := lifecycle(s.Detail("PR_1"))
			if state != tt.want || draft != tt.wantDraft {
				t.Errorf("state = %q draft = %v, want %q %v", state, draft, tt.want, tt.wantDraft)
			}
		})
	}
}

// The edit carries the move, not the landing, so two out at once compose in the
// order they were pressed rather than the second undoing the first.
func TestTwoTransitionsInFlightCompose(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", staged(gh.PRStateOpen, false))

	s.PendingState("PR_1", gh.TransitionDraft)
	s.PendingState("PR_1", gh.TransitionClose)

	state, draft := lifecycle(s.Detail("PR_1"))
	if state != gh.PRStateClosed || !draft {
		t.Errorf("state = %q draft = %v, want CLOSED true", state, draft)
	}
}

func TestARefetchDoesNotDropATransitionStillInFlight(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", staged(gh.PRStateOpen, false))
	s.PendingState("PR_1", gh.TransitionDraft)

	s.DetailApplied("PR_1", staged(gh.PRStateOpen, false))

	if _, draft := lifecycle(s.Detail("PR_1")); !draft {
		t.Error("the refetch dropped an edit still in flight")
	}
}

func TestStateAppliedTakesGitHubsAnswer(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", staged(gh.PRStateOpen, false))
	key := s.PendingState("PR_1", gh.TransitionClose)

	// GitHub says it closed and is a draft. Its answer is the authority.
	s.StateApplied("PR_1", key, gh.PRStateResult{State: gh.PRStateClosed, IsDraft: true})

	state, draft := lifecycle(s.Detail("PR_1"))
	if state != gh.PRStateClosed || !draft {
		t.Errorf("state = %q draft = %v, want CLOSED true", state, draft)
	}
}

func TestARevertedTransitionPutsTheFetchedStateBack(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", staged(gh.PRStateOpen, false))
	key := s.PendingState("PR_1", gh.TransitionClose)

	s.EditReverted("PR_1", key)

	state, draft := lifecycle(s.Detail("PR_1"))
	if state != gh.PRStateOpen || draft {
		t.Errorf("state = %q draft = %v, want OPEN false", state, draft)
	}
}

// The earlier write answering last must not overwrite the reader's newer ask.
func TestAnEarlierStateResponseDoesNotOverwriteALaterEdit(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", staged(gh.PRStateOpen, false))

	first := s.PendingState("PR_1", gh.TransitionDraft)
	s.PendingState("PR_1", gh.TransitionClose)

	s.StateApplied("PR_1", first, gh.PRStateResult{State: gh.PRStateOpen, IsDraft: true})

	if state, _ := lifecycle(s.Detail("PR_1")); state != gh.PRStateClosed {
		t.Errorf("state = %q, want the later edit still showing", state)
	}
}

// An edit and a label set are different fields, and both fold at once.
func TestATransitionAndALabelSetFoldTogether(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", gh.DetailResult{Detail: gh.PullRequestDetail{
		PullRequest: gh.PullRequest{State: gh.PRStateOpen},
		Labels:      labelSet("bug"),
	}})

	s.PendingState("PR_1", gh.TransitionClose)
	s.PendingLabels("PR_1", labelSet("bug", "urgent"))

	held := s.Detail("PR_1")
	if state, _ := lifecycle(held); state != gh.PRStateClosed {
		t.Errorf("state = %q, want CLOSED", state)
	}
	if got, want := labelNames(held), []string{"bug", "urgent"}; !slices.Equal(got, want) {
		t.Errorf("labels = %q, want %q", got, want)
	}
}

// Two writes on different fields are not evidence about each other. Gating one
// on the other drops an answer nobody is going to send again.
func TestAStateWriteDoesNotSuppressALabelAnswer(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", gh.DetailResult{Detail: gh.PullRequestDetail{
		PullRequest: gh.PullRequest{State: gh.PRStateOpen},
		Labels:      labelSet("bug"),
	}})

	labels := s.PendingLabels("PR_1", labelSet("bug", "urgent"))
	state := s.PendingState("PR_1", gh.TransitionClose)

	// The labels answer while the state change is still out.
	s.LabelsApplied("PR_1", labels, gh.LabelsResult{Labels: labelSet("bug", "urgent")})

	// The state change then fails and takes only itself off the screen.
	s.EditReverted("PR_1", state)

	if got, want := labelNames(s.Detail("PR_1")), []string{"bug", "urgent"}; !slices.Equal(got, want) {
		t.Errorf("labels = %q, want the answered set kept", got)
	}
}

func TestALabelWriteDoesNotSuppressAStateAnswer(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", gh.DetailResult{Detail: gh.PullRequestDetail{
		PullRequest: gh.PullRequest{State: gh.PRStateOpen},
		Labels:      labelSet("bug"),
	}})

	state := s.PendingState("PR_1", gh.TransitionClose)
	labels := s.PendingLabels("PR_1", labelSet("bug", "urgent"))

	s.StateApplied("PR_1", state, gh.PRStateResult{State: gh.PRStateClosed})
	s.LabelsApplied("PR_1", labels, gh.LabelsResult{Labels: labelSet("bug", "urgent")})

	if got, _ := lifecycle(s.Detail("PR_1")); got != gh.PRStateClosed {
		t.Errorf("state = %q, want the answered close kept", got)
	}
	if got, want := labelNames(s.Detail("PR_1")), []string{"bug", "urgent"}; !slices.Equal(got, want) {
		t.Errorf("labels = %q, want %q", got, want)
	}
}

// A detail fetch asked for before a write settled answers from the state the
// pull request was in beforehand. Storing it would put the landed write back on
// the screen undone, and its fetched permissions would take the row's key with
// it.
func TestADetailAskedForBeforeAWriteIsDropped(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", staged(gh.PRStateOpen, false))

	// The reader syncs, then closes before the sync answers.
	if !s.BeginDetail("PR_1") {
		t.Fatal("BeginDetail refused a detail that is not loading")
	}
	key := s.PendingState("PR_1", gh.TransitionClose)
	s.StateApplied("PR_1", key, gh.PRStateResult{State: gh.PRStateClosed})

	if !s.StaleDetail("PR_1") {
		t.Error("the fetch in flight is not marked stale")
	}

	// The sync answers with the pull request as it was before the close.
	s.DetailApplied("PR_1", staged(gh.PRStateOpen, false))

	if got, _ := lifecycle(s.Detail("PR_1")); got != gh.PRStateClosed {
		t.Errorf("state = %q, want the landed close kept", got)
	}
	if s.Detail("PR_1").Status != store.StatusReady {
		t.Error("the detail is still marked loading after the response landed")
	}
}

// The fetch the caller then owes carries everything, and is not stale.
func TestTheFetchAfterAWriteIsNotStale(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", staged(gh.PRStateOpen, false))

	key := s.PendingState("PR_1", gh.TransitionClose)
	s.StateApplied("PR_1", key, gh.PRStateResult{State: gh.PRStateClosed})

	if !s.BeginDetail("PR_1") {
		t.Fatal("BeginDetail refused after the write settled")
	}
	if s.StaleDetail("PR_1") {
		t.Error("a fetch asked for after the write is marked stale")
	}

	s.DetailApplied("PR_1", staged(gh.PRStateClosed, false))
	if got, _ := lifecycle(s.Detail("PR_1")); got != gh.PRStateClosed {
		t.Errorf("state = %q, want the fresh response taken", got)
	}
}

// The rail reads this to know the permissions beside the state are a round trip
// behind it.
func TestAStateWriteInFlightIsVisibleOnTheDetail(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", staged(gh.PRStateOpen, false))

	if s.Detail("PR_1").StateWriting {
		t.Error("StateWriting is set with nothing in flight")
	}

	key := s.PendingState("PR_1", gh.TransitionClose)
	if !s.Detail("PR_1").StateWriting {
		t.Error("StateWriting is not set while a lifecycle write is out")
	}

	s.EditReverted("PR_1", key)
	if s.Detail("PR_1").StateWriting {
		t.Error("StateWriting is still set after the write came back")
	}
}

// A label write is not a lifecycle write, so it must not hold the State row.
func TestALabelWriteDoesNotReadAsAStateWrite(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", labelled("bug"))

	s.PendingLabels("PR_1", labelSet("bug", "urgent"))

	if s.Detail("PR_1").StateWriting {
		t.Error("a label write reads as a lifecycle write")
	}
}

func people(logins ...string) []gh.Actor {
	out := make([]gh.Actor, len(logins))
	for i, l := range logins {
		out[i] = gh.Actor{ID: "U_" + l, Login: l}
	}
	return out
}

func assigned(logins ...string) gh.DetailResult {
	return gh.DetailResult{Detail: gh.PullRequestDetail{Assignees: people(logins...)}}
}

func assigneeLogins(d store.Detail) []string {
	out := make([]string, 0, len(d.Detail.Assignees))
	for _, a := range d.Detail.Assignees {
		out = append(out, a.Login)
	}
	return out
}

func reviewerLogins(d store.Detail) []string {
	out := make([]string, 0, len(d.Detail.Reviewers))
	for _, r := range d.Detail.Reviewers {
		out = append(out, r.Actor.Login)
	}
	return out
}

func TestAPendingAssigneeSetRendersBeforeItLands(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", assigned("drucial"))

	s.PendingAssignees("PR_1", people("drucial", "nkr"))

	if got, want := assigneeLogins(s.Detail("PR_1")), []string{"drucial", "nkr"}; !slices.Equal(got, want) {
		t.Errorf("assignees = %q, want %q", got, want)
	}
}

// Unchecking the last assignee is a real write, the same as unchecking the last
// label. An empty set folding as "no edit" would leave them on the screen with
// nothing to say why.
func TestAPendingEmptyAssigneeSetClearsThem(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", assigned("drucial"))

	s.PendingAssignees("PR_1", nil)

	if got := assigneeLogins(s.Detail("PR_1")); len(got) != 0 {
		t.Errorf("assignees = %q, want none", got)
	}
}

func TestARefetchDoesNotDropAnAssigneeEditStillInFlight(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", assigned("drucial"))
	s.PendingAssignees("PR_1", people("drucial", "nkr"))

	s.DetailApplied("PR_1", assigned("drucial"))

	if got, want := assigneeLogins(s.Detail("PR_1")), []string{"drucial", "nkr"}; !slices.Equal(got, want) {
		t.Errorf("assignees = %q, want the edit still folded over the refetch", got)
	}
}

func TestAnEarlierAssigneeResponseDoesNotOverwriteALaterEdit(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", assigned("drucial"))

	first := s.PendingAssignees("PR_1", people("nkr"))
	s.PendingAssignees("PR_1", people("nkr", "octocat"))

	// The first write answers last, carrying the set nobody is asking for now.
	s.AssigneesApplied("PR_1", first, gh.AssigneesResult{Assignees: people("nkr")})

	if got, want := assigneeLogins(s.Detail("PR_1")), []string{"nkr", "octocat"}; !slices.Equal(got, want) {
		t.Errorf("assignees = %q, want the later edit still showing", got)
	}
}

// A failed write takes only itself off the screen.
func TestAFailedAssigneeWriteRestoresTheFetchedSet(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", assigned("drucial"))

	key := s.PendingAssignees("PR_1", people("drucial", "nkr"))
	s.EditReverted("PR_1", key)

	if got, want := assigneeLogins(s.Detail("PR_1")), []string{"drucial"}; !slices.Equal(got, want) {
		t.Errorf("assignees = %q, want the fetched set back", got)
	}
}

// The reviewer panel is the one write with no answer worth taking. The endpoint
// reports the requests it now holds and nothing about who already reviewed, so
// the optimistic panel stands until the refetch replaces it.
func TestAReviewerAnswerLeavesTheOptimisticPanelStanding(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", gh.DetailResult{Detail: gh.PullRequestDetail{
		Reviewers: []gh.Reviewer{{Actor: gh.Actor{Login: "nkr"}, State: gh.ReviewStateApproved}},
	}})

	key := s.PendingReviewers("PR_1", []gh.Reviewer{
		{Actor: gh.Actor{Login: "nkr"}, State: gh.ReviewStateApproved},
		{Actor: gh.Actor{Login: "octocat"}},
	})
	s.ReviewersApplied("PR_1", key)

	if got, want := reviewerLogins(s.Detail("PR_1")), []string{"nkr", "octocat"}; !slices.Equal(got, want) {
		t.Errorf("reviewers = %q, want the panel the write put up", got)
	}
}

func TestAFailedReviewerWriteRestoresTheFetchedPanel(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", gh.DetailResult{Detail: gh.PullRequestDetail{
		Reviewers: []gh.Reviewer{{Actor: gh.Actor{Login: "nkr"}}},
	}})

	key := s.PendingReviewers("PR_1", []gh.Reviewer{
		{Actor: gh.Actor{Login: "nkr"}},
		{Actor: gh.Actor{Login: "octocat"}},
	})
	s.EditReverted("PR_1", key)

	if got, want := reviewerLogins(s.Detail("PR_1")), []string{"nkr"}; !slices.Equal(got, want) {
		t.Errorf("reviewers = %q, want the fetched panel back", got)
	}
}

// A reviewer write fires a refetch, so a fetch already in flight when it settles
// is one asked for before it and cannot be taken.
func TestAReviewerWriteMarksAFetchInFlightStale(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", gh.DetailResult{Detail: gh.PullRequestDetail{
		Reviewers: []gh.Reviewer{{Actor: gh.Actor{Login: "nkr"}}},
	}})

	if !s.BeginDetail("PR_1") {
		t.Fatal("BeginDetail refused a detail that is not loading")
	}
	key := s.PendingReviewers("PR_1", []gh.Reviewer{{Actor: gh.Actor{Login: "octocat"}}})
	s.ReviewersApplied("PR_1", key)

	if !s.StaleDetail("PR_1") {
		t.Error("the fetch in flight is not marked stale")
	}
}

// The four fields are four queues. An answer on one is not evidence about
// another, and gating them together drops an answer nobody will send again.
func TestAnAssigneeWriteDoesNotSuppressAReviewerAnswer(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", gh.DetailResult{Detail: gh.PullRequestDetail{
		Assignees: people("drucial"),
		Reviewers: []gh.Reviewer{{Actor: gh.Actor{Login: "nkr"}}},
	}})

	reviewers := s.PendingReviewers("PR_1", []gh.Reviewer{
		{Actor: gh.Actor{Login: "nkr"}},
		{Actor: gh.Actor{Login: "octocat"}},
	})
	assignees := s.PendingAssignees("PR_1", people("drucial", "nkr"))

	// The reviewers answer while the assignee write is still out.
	s.ReviewersApplied("PR_1", reviewers)
	// The assignee write then fails and takes only itself off the screen.
	s.EditReverted("PR_1", assignees)

	if got, want := reviewerLogins(s.Detail("PR_1")), []string{"nkr", "octocat"}; !slices.Equal(got, want) {
		t.Errorf("reviewers = %q, want the answered panel kept", got)
	}
	if got, want := assigneeLogins(s.Detail("PR_1")), []string{"drucial"}; !slices.Equal(got, want) {
		t.Errorf("assignees = %q, want the fetched set back", got)
	}
}

// Neither of the two new writes holds the State row: the rail reads StateWriting
// to know the permissions beside the state are a round trip behind it, and only
// a lifecycle write puts them there.
func TestNeitherPeopleWriteReadsAsAStateWrite(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", assigned("drucial"))

	s.PendingAssignees("PR_1", people("drucial", "nkr"))
	s.PendingReviewers("PR_1", []gh.Reviewer{{Actor: gh.Actor{Login: "nkr"}}})

	if s.Detail("PR_1").StateWriting {
		t.Error("a reviewer or assignee write reads as a lifecycle write")
	}
}

const repo = "zen-octo/zen-octo"

func based(base string, behind int) gh.DetailResult {
	return gh.DetailResult{Detail: gh.PullRequestDetail{
		PullRequest: gh.PullRequest{BaseRefName: base},
		BehindBy:    behind,
	}}
}

func TestAPendingRetargetRendersBeforeItLands(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", based("main", 3))

	s.PendingBase("PR_1", "develop")

	if got, want := s.Detail("PR_1").Detail.BaseRefName, "develop"; got != want {
		t.Errorf("BaseRefName = %q, want %q", got, want)
	}
}

// The old count was measured against a branch the pull request no longer
// targets, and zero already means up to date. Keeping either renders a number
// that is wrong under a name that is right.
func TestARetargetTakesTheBehindCountWithIt(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", based("main", 3))

	s.PendingBase("PR_1", "develop")

	if got := s.Detail("PR_1").Detail.BehindBy; got != gh.BehindUnknown {
		t.Errorf("BehindBy = %d, want BehindUnknown (%d)", got, gh.BehindUnknown)
	}
}

// The write has landed and the comparison has not been run. Letting the fetched
// number back here puts a count against the old branch under the name of the
// new one, for as long as the refetch takes.
func TestASettledRetargetStillHasNoCount(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", based("main", 3))
	key := s.PendingBase("PR_1", "develop")

	s.BaseApplied("PR_1", key, gh.BaseResult{BaseRefName: "develop"})

	held := s.Detail("PR_1").Detail
	if got, want := held.BaseRefName, "develop"; got != want {
		t.Errorf("BaseRefName = %q, want %q", got, want)
	}
	if got := held.BehindBy; got != gh.BehindUnknown {
		t.Errorf("BehindBy = %d, want BehindUnknown (%d)", got, gh.BehindUnknown)
	}
}

// GitHub's answer, not the ask. Somebody retargeting in the browser first is
// what parts them.
func TestASettledRetargetTakesTheBranchGitHubNamed(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", based("main", 3))
	key := s.PendingBase("PR_1", "develop")

	s.BaseApplied("PR_1", key, gh.BaseResult{BaseRefName: "release/2.0"})

	if got, want := s.Detail("PR_1").Detail.BaseRefName, "release/2.0"; got != want {
		t.Errorf("BaseRefName = %q, want %q", got, want)
	}
}

func TestARevertedRetargetPutsTheBranchAndItsCountBack(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", based("main", 3))
	key := s.PendingBase("PR_1", "develop")

	s.EditReverted("PR_1", key)

	held := s.Detail("PR_1").Detail
	if got, want := held.BaseRefName, "main"; got != want {
		t.Errorf("BaseRefName = %q, want %q", got, want)
	}
	if got, want := held.BehindBy, 3; got != want {
		t.Errorf("BehindBy = %d, want %d", got, want)
	}
}

// Two fields, two writes, no interference. An edit is only ever stale against a
// later one on its own field.
func TestARetargetAndALabelSetBothApply(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", based("main", 3))

	s.PendingBase("PR_1", "develop")
	s.PendingLabels("PR_1", labelSet("bug"))

	held := s.Detail("PR_1")
	if got, want := held.Detail.BaseRefName, "develop"; got != want {
		t.Errorf("BaseRefName = %q, want %q", got, want)
	}
	if got, want := labelNames(held), []string{"bug"}; !slices.Equal(got, want) {
		t.Errorf("labels = %q, want %q", got, want)
	}
}

func TestABranchSearchIsHeldForThePickerThatAskedForIt(t *testing.T) {
	s := store.New(configured())

	if !s.BeginBranches(repo, "rel") {
		t.Fatal("BeginBranches refused a search never run")
	}
	s.BranchesApplied(repo, gh.BranchResult{
		Query: "rel", Default: "main", Branches: []string{"release/2.0"}, More: 4,
	})

	held := s.Branches(repo)
	if !held.Loaded {
		t.Error("the search is not marked loaded")
	}
	if got, want := held.Names, []string{"release/2.0"}; !slices.Equal(got, want) {
		t.Errorf("names = %q, want %q", got, want)
	}
	if got, want := held.More, 4; got != want {
		t.Errorf("More = %d, want %d", got, want)
	}
	if got, want := held.Default, "main"; got != want {
		t.Errorf("Default = %q, want %q", got, want)
	}

	// Backspacing back onto a search that has answered costs nothing.
	if s.BeginBranches(repo, "rel") {
		t.Error("BeginBranches re-ran a search it already holds the answer to")
	}
}

func TestBeginBranchesRefusesTheSameSearchTwiceInFlight(t *testing.T) {
	s := store.New(configured())

	if !s.BeginBranches(repo, "rel") {
		t.Fatal("the first BeginBranches was refused")
	}
	if s.BeginBranches(repo, "rel") {
		t.Error("BeginBranches started a second request while one was in flight")
	}
	// A different question is a different request, even with one out.
	if !s.BeginBranches(repo, "rele") {
		t.Error("BeginBranches refused a search for something else")
	}
}

// Two searches settle in whatever order the network gives them, which is not
// the order they were typed. Painting the older one puts a list two keystrokes
// behind the filter above it.
func TestAnAnswerToASearchNobodyIsRunningIsDropped(t *testing.T) {
	s := store.New(configured())
	s.BeginBranches(repo, "rel")
	s.BeginBranches(repo, "release")

	s.BranchesApplied(repo, gh.BranchResult{Query: "rel", Branches: []string{"stale"}})

	if held := s.Branches(repo); held.Loaded {
		t.Errorf("the store took an answer to %q while asking %q", "rel", held.Query)
	}
}

// Same reason, other leg. A failure for a search two keystrokes ago would tell
// the reader the search they are waiting on had failed.
func TestAFailureForAnOldSearchIsDropped(t *testing.T) {
	s := store.New(configured())
	s.BeginBranches(repo, "rel")
	s.BeginBranches(repo, "release")

	s.BranchesFailed(repo, "rel", errors.New("boom"))

	if got := s.Branches(repo).Err; got != nil {
		t.Errorf("err = %v, want none: the search still running has not failed", got)
	}
}

// A failed search is the one state where the same question is worth asking
// again: the reader is retrying, not the store forgetting.
// The retry has to survive an earlier search having worked, which is the only
// way it comes up: the reader types, one keystroke answers, the next one drops
// the connection. Reading "has answered at least once" as "has this answer"
// leaves the picker on the older list with no way to ask again.
func TestAFailedSearchCanBeRunAgain(t *testing.T) {
	s := store.New(configured())
	boom := errors.New("boom")

	s.BeginBranches(repo, "rel")
	s.BranchesApplied(repo, gh.BranchResult{Query: "rel", Branches: []string{"release/2.0"}})

	s.BeginBranches(repo, "release")
	s.BranchesFailed(repo, "release", boom)

	if !errors.Is(s.Branches(repo).Err, boom) {
		t.Errorf("err = %v, want %v", s.Branches(repo).Err, boom)
	}
	if !s.BeginBranches(repo, "release") {
		t.Error("BeginBranches refuses to retry a search that failed")
	}
}

// A retarget rewrites every file in the diff, and the Files tab asks for one
// once per open, so the debt has to be recorded rather than acted on.
func TestARetargetMarksTheDiffStale(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", based("main", 3))
	s.BeginFiles("PR_1")
	s.FilesApplied("PR_1", gh.FilesResult{Files: []gh.ChangedFile{{Path: "a.go"}}})

	key := s.PendingBase("PR_1", "develop")
	if s.StaleFiles("PR_1") {
		t.Error("the diff is stale before the write has landed")
	}

	s.BaseApplied("PR_1", key, gh.BaseResult{BaseRefName: "develop"})

	if !s.StaleFiles("PR_1") {
		t.Error("a landed retarget did not mark the diff stale")
	}
}

// Starting the corrective fetch is what settles the debt.
func TestBeginningADiffClearsTheStaleMark(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", based("main", 3))
	key := s.PendingBase("PR_1", "develop")
	s.BaseApplied("PR_1", key, gh.BaseResult{BaseRefName: "develop"})

	if !s.BeginFiles("PR_1") {
		t.Fatal("BeginFiles refused a diff with nothing in flight")
	}
	if s.StaleFiles("PR_1") {
		t.Error("starting the corrective fetch left the debt outstanding")
	}
}

// A fetch already in flight was measured against the old base too, so it cannot
// settle the debt. The mark stays for the caller to answer once it lands.
func TestARetargetOverADiffInFlightKeepsTheDebt(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", based("main", 3))

	s.BeginFiles("PR_1") // out, and measured against main
	key := s.PendingBase("PR_1", "develop")
	s.BaseApplied("PR_1", key, gh.BaseResult{BaseRefName: "develop"})

	if s.BeginFiles("PR_1") {
		t.Fatal("BeginFiles started a second request while one was in flight")
	}
	if !s.StaleFiles("PR_1") {
		t.Error("the refused correction dropped the debt, so nothing owes the refetch")
	}

	// The one in flight lands, and the caller can now settle it.
	s.FilesApplied("PR_1", gh.FilesResult{Files: []gh.ChangedFile{{Path: "a.go"}}})
	if !s.StaleFiles("PR_1") {
		t.Error("the stale answer landing cleared the debt")
	}
	if !s.BeginFiles("PR_1") {
		t.Error("the corrective fetch cannot start once the old one has landed")
	}
}

// BeginBranches refuses a query already answered, so without this a branch made
// in the browser never reaches the picker.
func TestInvalidateBranchesLetsTheNextPickerAskAgain(t *testing.T) {
	s := store.New(configured())
	s.BeginBranches(repo, "")
	s.BranchesApplied(repo, gh.BranchResult{Branches: []string{"main"}})

	if s.BeginBranches(repo, "") {
		t.Fatal("setup: BeginBranches re-ran a search it already holds")
	}

	s.InvalidateBranches(repo)

	if !s.BeginBranches(repo, "") {
		t.Error("BeginBranches still refuses after the search was invalidated")
	}
}

// A merge lands the pull request before GitHub answers, the way every other
// write on this rail paints first.
func TestAPendingMergeRendersBeforeItLands(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", staged(gh.PRStateOpen, false))

	s.PendingMerge("PR_1")

	if state, _ := lifecycle(s.Detail("PR_1")); state != gh.PRStateMerged {
		t.Errorf("state = %q, want MERGED", state)
	}
}

// A merge is a lifecycle move, so the State row has to wait it out the same way
// it waits out a close: the state says merged while the permissions still say
// closable.
func TestAMergeInFlightReadsAsAStateWrite(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", staged(gh.PRStateOpen, false))

	key := s.PendingMerge("PR_1")
	if !s.Detail("PR_1").StateWriting {
		t.Error("StateWriting is not set while a merge is out")
	}

	s.EditReverted("PR_1", key)
	if s.Detail("PR_1").StateWriting {
		t.Error("StateWriting is still set after the merge came back")
	}
	if state, _ := lifecycle(s.Detail("PR_1")); state != gh.PRStateOpen {
		t.Errorf("state = %q, want the fetched state back", state)
	}
}

// Both are writes on the one field, so the later ask is what shows and the
// earlier answer must not overwrite it.
func TestAnEarlierCloseDoesNotOverwriteAMergeStillOut(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", staged(gh.PRStateOpen, false))

	closing := s.PendingState("PR_1", gh.TransitionClose)
	s.PendingMerge("PR_1")

	s.StateApplied("PR_1", closing, gh.PRStateResult{State: gh.PRStateClosed})

	if state, _ := lifecycle(s.Detail("PR_1")); state != gh.PRStateMerged {
		t.Errorf("state = %q, want the merge still showing", state)
	}
}

func TestMergeAppliedTakesGitHubsAnswerAndAsksForTheRest(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", staged(gh.PRStateOpen, false))
	key := s.PendingMerge("PR_1")

	s.BeginDetail("PR_1") // a refresh asked for before the write settled
	s.MergeApplied("PR_1", key, gh.MergeResult{State: gh.PRStateMerged})

	if state, _ := lifecycle(s.Detail("PR_1")); state != gh.PRStateMerged {
		t.Errorf("state = %q, want MERGED", state)
	}
	// The timeline, the checks and the merge state all move with it and none of
	// them can be computed here.
	if !s.StaleDetail("PR_1") {
		t.Error("a landed merge did not mark the fetch in flight stale")
	}
}

// A merge writes a commit onto the base and changes nothing about the
// difference between the two branches, so the Files tab owes no refetch.
func TestAMergeDoesNotMarkTheDiffStale(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", staged(gh.PRStateOpen, false))
	s.BeginFiles("PR_1")
	s.FilesApplied("PR_1", gh.FilesResult{Files: []gh.ChangedFile{{Path: "a.go"}}})

	key := s.PendingMerge("PR_1")
	s.MergeApplied("PR_1", key, gh.MergeResult{State: gh.PRStateMerged})

	if s.StaleFiles("PR_1") {
		t.Error("a merge marked the diff stale, which costs a request for a diff that did not change")
	}
}

// A write whose failure says the screen is behind GitHub owes a refetch, and
// the answer it takes has to be one asked for after the failure. A response
// already in flight was asked for before it and carries the same stale picture
// the write just tripped over.
func TestARevertThatOwesARefetchMarksTheFetchInFlightStale(t *testing.T) {
	for _, tt := range []struct {
		name string
		hold func(s *store.Store) string
	}{
		{"a merge", func(s *store.Store) string { return s.PendingMerge("PR_1") }},
		{"a reviewer write", func(s *store.Store) string {
			return s.PendingReviewers("PR_1", []gh.Reviewer{{Actor: gh.Actor{Login: "nkr"}}})
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := store.New(configured())
			s.DetailApplied("PR_1", staged(gh.PRStateOpen, false))

			key := tt.hold(&s)
			s.BeginDetail("PR_1") // asked for before the write came back
			s.EditRevertedStale("PR_1", key)

			if !s.StaleDetail("PR_1") {
				t.Error("the fetch in flight is not marked stale, so the caller will believe it")
			}
		})
	}
}

// An ordinary revert says the pull request never moved, so there is nothing to
// ask again for and no reason to spend the request.
func TestAnOrdinaryRevertOwesNoRefetch(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", staged(gh.PRStateOpen, false))

	key := s.PendingState("PR_1", gh.TransitionClose)
	s.BeginDetail("PR_1")
	s.EditReverted("PR_1", key)

	if s.StaleDetail("PR_1") {
		t.Error("a plain revert marked the fetch stale and bought a request nothing needed")
	}
}
