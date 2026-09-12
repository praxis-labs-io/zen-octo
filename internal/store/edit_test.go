package store_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
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

func TestAPendingEmptyLabelSetClearsThem(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", labelled("bug"))

	s.PendingLabels("PR_1", nil)

	if got := labelNames(s.Detail("PR_1")); len(got) != 0 {
		t.Errorf("labels = %q, want none", got)
	}
}

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

	s.EditReverted("PR_1", first)
	if got, want := labelNames(s.Detail("PR_1")), []string{"urgent", "docs"}; !slices.Equal(got, want) {
		t.Errorf("labels = %q after the first reverted, want %q", got, want)
	}
}

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

	if !s.BeginRepoMeta("acme/rocket") {
		t.Fatal("BeginRepoMeta refused a repository never fetched")
	}
	s.RepoMetaApplied("acme/rocket", gh.RepoMetaResult{
		Meta: gh.RepoMeta{Labels: labelSet("bug")},
	})

	held := s.Repo("acme/rocket")
	if !held.Loaded {
		t.Error("metadata is not marked loaded")
	}
	if got, want := len(held.Meta.Labels), 1; got != want {
		t.Errorf("labels = %d, want %d", got, want)
	}

	if s.BeginRepoMeta("acme/rocket") {
		t.Error("BeginRepoMeta started a second request for metadata already held")
	}
}

func TestBeginRepoMetaRefusesOneAlreadyInFlight(t *testing.T) {
	s := store.New(configured())

	if !s.BeginRepoMeta("acme/rocket") {
		t.Fatal("the first BeginRepoMeta was refused")
	}
	if s.BeginRepoMeta("acme/rocket") {
		t.Error("BeginRepoMeta started a second request while one was in flight")
	}
}

func TestInvalidateRepoMetaLetsTheNextPickerAskAgain(t *testing.T) {
	s := store.New(configured())
	s.BeginRepoMeta("acme/rocket")
	s.RepoMetaApplied("acme/rocket", gh.RepoMetaResult{Meta: gh.RepoMeta{Labels: labelSet("bug")}})

	s.InvalidateRepoMeta("acme/rocket")

	if !s.BeginRepoMeta("acme/rocket") {
		t.Error("BeginRepoMeta still refuses after the metadata was invalidated")
	}
}

func TestFailedRepoMetaCarriesItsError(t *testing.T) {
	s := store.New(configured())
	boom := errors.New("boom")

	s.BeginRepoMeta("acme/rocket")
	s.RepoMetaFailed("acme/rocket", boom)

	held := s.Repo("acme/rocket")
	if !errors.Is(held.Err, boom) {
		t.Errorf("err = %v, want %v", held.Err, boom)
	}
	if held.Loaded {
		t.Error("a failed first fetch is marked loaded")
	}
}

func TestAnEarlierLabelResponseDoesNotOverwriteALaterEdit(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", labelled("bug"))

	first := s.PendingLabels("PR_1", labelSet("urgent"))
	s.PendingLabels("PR_1", labelSet("urgent", "docs"))

	s.LabelsApplied("PR_1", first, gh.LabelsResult{Labels: labelSet("urgent")})

	if got, want := labelNames(s.Detail("PR_1")), []string{"urgent", "docs"}; !slices.Equal(got, want) {
		t.Errorf("labels = %q, want the later edit still showing", got)
	}
}

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

func TestAStateWriteDoesNotSuppressALabelAnswer(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", gh.DetailResult{Detail: gh.PullRequestDetail{
		PullRequest: gh.PullRequest{State: gh.PRStateOpen},
		Labels:      labelSet("bug"),
	}})

	labels := s.PendingLabels("PR_1", labelSet("bug", "urgent"))
	state := s.PendingState("PR_1", gh.TransitionClose)

	s.LabelsApplied("PR_1", labels, gh.LabelsResult{Labels: labelSet("bug", "urgent")})

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

func TestADetailAskedForBeforeAWriteIsDropped(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", staged(gh.PRStateOpen, false))

	if !s.BeginDetail("PR_1") {
		t.Fatal("BeginDetail refused a detail that is not loading")
	}
	key := s.PendingState("PR_1", gh.TransitionClose)
	s.StateApplied("PR_1", key, gh.PRStateResult{State: gh.PRStateClosed})

	if !s.StaleDetail("PR_1") {
		t.Error("the fetch in flight is not marked stale")
	}

	s.DetailApplied("PR_1", staged(gh.PRStateOpen, false))

	if got, _ := lifecycle(s.Detail("PR_1")); got != gh.PRStateClosed {
		t.Errorf("state = %q, want the landed close kept", got)
	}
	if s.Detail("PR_1").Status != store.StatusReady {
		t.Error("the detail is still marked loading after the response landed")
	}
}

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

	s.AssigneesApplied("PR_1", first, gh.AssigneesResult{Assignees: people("nkr")})

	if got, want := assigneeLogins(s.Detail("PR_1")), []string{"nkr", "octocat"}; !slices.Equal(got, want) {
		t.Errorf("assignees = %q, want the later edit still showing", got)
	}
}

func TestAFailedAssigneeWriteRestoresTheFetchedSet(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", assigned("drucial"))

	key := s.PendingAssignees("PR_1", people("drucial", "nkr"))
	s.EditReverted("PR_1", key)

	if got, want := assigneeLogins(s.Detail("PR_1")), []string{"drucial"}; !slices.Equal(got, want) {
		t.Errorf("assignees = %q, want the fetched set back", got)
	}
}

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

	s.ReviewersApplied("PR_1", reviewers)
	s.EditReverted("PR_1", assignees)

	if got, want := reviewerLogins(s.Detail("PR_1")), []string{"nkr", "octocat"}; !slices.Equal(got, want) {
		t.Errorf("reviewers = %q, want the answered panel kept", got)
	}
	if got, want := assigneeLogins(s.Detail("PR_1")), []string{"drucial"}; !slices.Equal(got, want) {
		t.Errorf("assignees = %q, want the fetched set back", got)
	}
}

func TestNeitherPeopleWriteReadsAsAStateWrite(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", assigned("drucial"))

	s.PendingAssignees("PR_1", people("drucial", "nkr"))
	s.PendingReviewers("PR_1", []gh.Reviewer{{Actor: gh.Actor{Login: "nkr"}}})

	if s.Detail("PR_1").StateWriting {
		t.Error("a reviewer or assignee write reads as a lifecycle write")
	}
}

const repo = "acme/rocket"

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

func TestARetargetTakesTheBehindCountWithIt(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", based("main", 3))

	s.PendingBase("PR_1", "develop")

	if got := s.Detail("PR_1").Detail.BehindBy; got != gh.BehindUnknown {
		t.Errorf("BehindBy = %d, want BehindUnknown (%d)", got, gh.BehindUnknown)
	}
}

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
	if !s.BeginBranches(repo, "rele") {
		t.Error("BeginBranches refused a search for something else")
	}
}

func TestAnAnswerToASearchNobodyIsRunningIsDropped(t *testing.T) {
	s := store.New(configured())
	s.BeginBranches(repo, "rel")
	s.BeginBranches(repo, "release")

	s.BranchesApplied(repo, gh.BranchResult{Query: "rel", Branches: []string{"stale"}})

	if held := s.Branches(repo); held.Loaded {
		t.Errorf("the store took an answer to %q while asking %q", "rel", held.Query)
	}
}

func TestAFailureForAnOldSearchIsDropped(t *testing.T) {
	s := store.New(configured())
	s.BeginBranches(repo, "rel")
	s.BeginBranches(repo, "release")

	s.BranchesFailed(repo, "rel", errors.New("boom"))

	if got := s.Branches(repo).Err; got != nil {
		t.Errorf("err = %v, want none: the search still running has not failed", got)
	}
}

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

func TestARetargetOverADiffInFlightKeepsTheDebt(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", based("main", 3))

	s.BeginFiles("PR_1")
	key := s.PendingBase("PR_1", "develop")
	s.BaseApplied("PR_1", key, gh.BaseResult{BaseRefName: "develop"})

	if s.BeginFiles("PR_1") {
		t.Fatal("BeginFiles started a second request while one was in flight")
	}
	if !s.StaleFiles("PR_1") {
		t.Error("the refused correction dropped the debt, so nothing owes the refetch")
	}

	s.FilesApplied("PR_1", gh.FilesResult{Files: []gh.ChangedFile{{Path: "a.go"}}})
	if !s.StaleFiles("PR_1") {
		t.Error("the stale answer landing cleared the debt")
	}
	if !s.BeginFiles("PR_1") {
		t.Error("the corrective fetch cannot start once the old one has landed")
	}
}

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

func TestAPendingMergeRendersBeforeItLands(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", staged(gh.PRStateOpen, false))

	s.PendingMerge("PR_1")

	if state, _ := lifecycle(s.Detail("PR_1")); state != gh.PRStateMerged {
		t.Errorf("state = %q, want MERGED", state)
	}
}

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

	s.BeginDetail("PR_1")
	s.MergeApplied("PR_1", key, gh.MergeResult{State: gh.PRStateMerged})

	if state, _ := lifecycle(s.Detail("PR_1")); state != gh.PRStateMerged {
		t.Errorf("state = %q, want MERGED", state)
	}
	if !s.StaleDetail("PR_1") {
		t.Error("a landed merge did not mark the fetch in flight stale")
	}
}

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
			s.BeginDetail("PR_1")
			s.EditRevertedStale("PR_1", key)

			if !s.StaleDetail("PR_1") {
				t.Error("the fetch in flight is not marked stale, so the caller will believe it")
			}
		})
	}
}

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
