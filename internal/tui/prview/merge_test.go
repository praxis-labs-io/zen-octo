package prview_test

import (
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

func mergeableDetail() gh.PullRequestDetail {
	d := sampleDetail()
	d.Merge = gh.MergeClean
	d.HeadRefOid = "9f1c2b7"
	d.HeadRefID = "REF_88"
	d.MergeCommit = gh.MergeMessage{
		Headline: "Merge pull request #412 from acme/fix-auth-retry",
		Body:     "Fix auth retry",
	}
	d.SquashCommit = gh.MergeMessage{
		Headline: "Fix auth retry (#412)",
		Body:     "* Cap the backoff\n\n* Add a test",
	}
	return d
}

func mergeRepo(methods gh.MergeMethods) store.Repo {
	r := loadedRepo()
	r.Meta.Methods = methods
	return r
}

func allMethods() gh.MergeMethods {
	return gh.MergeMethods{Merge: true, Squash: true, Rebase: true}
}

func openMergeOn(t *testing.T, d gh.PullRequestDetail, repo store.Repo) prview.Model {
	t.Helper()

	label, _ := mergeLabelOf(d)
	m := onRailRow(t, detailed(held(d), 200, 60), label)

	_, cmd := key(m, "enter")
	if _, ok := runCmd(cmd).(prview.NeedRepoMetaMsg); !ok {
		t.Fatalf("enter on the Merge row sent %T, want a NeedRepoMetaMsg", runCmd(cmd))
	}

	m, _ = key(m, "enter")
	m.SetRepo(repo)
	return m
}

func openMerge(t *testing.T) prview.Model {
	t.Helper()
	return openMergeOn(t, mergeableDetail(), mergeRepo(allMethods()))
}

func mergeLabelOf(d gh.PullRequestDetail) (string, bool) {
	switch d.Merge {
	case gh.MergeClean:
		return "Ready to merge", true
	case gh.MergeUnstable:
		return "Checks failing", true
	case gh.MergeBlocked:
		return "Blocked", true
	case gh.MergeBehind:
		return "Behind the base", true
	case gh.MergeConflicting:
		return "Conflicts", false
	case gh.MergeDraft:
		return "Draft", false
	}
	return "Checking", false
}

func formBox(t *testing.T, m prview.Model) string {
	t.Helper()
	return menuBox(t, m, "Merge #412")
}

func chosenMethod(box, name string) bool {
	for _, row := range strings.Split(box, "\n") {
		if strings.Contains(row, name) {
			return strings.Contains(row, "✓")
		}
	}
	return false
}

func TestTheMergeRowIsAControlOnlyWhereThereIsAMergeToMake(t *testing.T) {
	tests := []struct {
		name  string
		state gh.MergeState
		admin bool
		want  bool
	}{
		{"clean", gh.MergeClean, false, true},
		{"checks failing", gh.MergeUnstable, false, true},
		{"blocked", gh.MergeBlocked, false, false},
		{"blocked, as an administrator", gh.MergeBlocked, true, true},
		{"behind", gh.MergeBehind, false, false},
		{"behind, as an administrator", gh.MergeBehind, true, true},
		{"conflicts", gh.MergeConflicting, true, false},
		{"draft", gh.MergeDraft, true, false},
		{"unknown", gh.MergeUnknown, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := mergeableDetail()
			d.Merge = tt.state
			d.Viewer.CanMergeAsAdmin = tt.admin

			label, _ := mergeLabelOf(d)
			m := press(detailed(held(d), 200, 60), "1")

			var reached bool
			for range 30 {
				m = press(m, "j")
				if markedRailRow(t, m.View()) == label {
					reached = true
					break
				}
			}
			if reached != tt.want {
				t.Errorf("the ring stops on the %q row = %v, want %v", label, reached, tt.want)
			}
		})
	}
}

func TestAMergedPullRequestSaysSoAndTheRingWalksPast(t *testing.T) {
	d := mergeableDetail()
	d.State = gh.PRStateMerged
	d.Merge = gh.MergeUnknown

	out := stripANSI(detailed(held(d), 200, 60).View())
	if !strings.Contains(out, "Merged into main") {
		t.Errorf("the Merge row does not name where it landed:\n%s", out)
	}

	m := press(detailed(held(d), 200, 60), "1")
	for range 30 {
		m = press(m, "j")
		if strings.Contains(markedRailRow(t, m.View()), "Merged into") {
			t.Error("the ring stopped on a merged pull request's Merge row")
		}
	}
}

func TestTheFormOffersOnlyTheMethodsTheRepositoryAllows(t *testing.T) {
	box := formBox(t, openMergeOn(t, mergeableDetail(),
		mergeRepo(gh.MergeMethods{Squash: true, Rebase: true})))

	if strings.Contains(box, "Create a merge commit") {
		t.Errorf("the form offers a method the repository forbids:\n%s", box)
	}
	for _, want := range []string{"Squash and merge", "Rebase and merge"} {
		if !strings.Contains(box, want) {
			t.Errorf("the form is missing %q:\n%s", want, box)
		}
	}
}

func TestTheFormOpensHoldingGitHubsOwnCommitMessage(t *testing.T) {
	box := formBox(t, openMerge(t))

	if !strings.Contains(box, "Fix auth retry (#412)") {
		t.Errorf("the headline is not GitHub's own squash title:\n%s", box)
	}
	if !strings.Contains(box, "Cap the backoff") {
		t.Errorf("the message is not GitHub's own squash body:\n%s", box)
	}
}

func TestSwitchingMethodRewritesAnUntouchedHeadline(t *testing.T) {
	box := formBox(t, press(openMerge(t), "up"))

	if !strings.Contains(box, "Merge pull request #412") {
		t.Errorf("the headline did not follow the method:\n%s", box)
	}
}

func TestSwitchingMethodKeepsAnEditedHeadline(t *testing.T) {
	m := press(openMerge(t), "tab", "!", "tab", "tab", "tab", "tab", "up")

	box := formBox(t, m)
	if !chosenMethod(box, "Create a merge commit") {
		t.Fatalf("setup: the method never changed, so nothing was put at risk:\n%s", box)
	}
	if !strings.Contains(box, "!") {
		t.Errorf("the edit did not survive the method change:\n%s", box)
	}
	if strings.Contains(box, "Merge pull request #412") {
		t.Errorf("the method change overwrote what was typed:\n%s", box)
	}
}

func TestARebaseDropsTheCommitMessageEntirely(t *testing.T) {
	m := press(openMerge(t), "down")

	box := formBox(t, m)
	for _, gone := range []string{"Headline", "Message", "Fix auth retry (#412)"} {
		if strings.Contains(box, gone) {
			t.Errorf("a rebase still renders %q:\n%s", gone, box)
		}
	}

	got, ok := merged(t, press(m, "tab", "tab"), "enter")
	if !ok {
		t.Fatalf("two tabs did not reach the button on a rebase form")
	}
	if got.Options.Method != gh.MergeMethodRebase {
		t.Errorf("method = %q, want REBASE", got.Options.Method)
	}
	if got.Options.Headline != "" || got.Options.Body != "" {
		t.Errorf("a rebase carried a commit message: %+v", got.Options)
	}
}

func merged(t *testing.T, m prview.Model, k string) (prview.MergeMsg, bool) {
	t.Helper()

	got, ok := asked(t, m, k).(prview.MergeMsg)
	return got, ok
}

func TestPressingMergeAsksTheRootWithEverythingItNeeds(t *testing.T) {
	got, ok := merged(t, press(openMerge(t), "tab", "tab", "tab", "tab"), "enter")
	if !ok {
		t.Fatal("enter on the button asked for no merge")
	}

	if got.ID != "PR_412" {
		t.Errorf("ID = %q, want PR_412", got.ID)
	}
	if got.Options.Method != gh.MergeMethodSquash {
		t.Errorf("method = %q, want SQUASH", got.Options.Method)
	}
	if got.Options.Headline != "Fix auth retry (#412)" {
		t.Errorf("headline = %q, want GitHub's own", got.Options.Headline)
	}
	if got.Options.ExpectedHeadOid != "9f1c2b7" {
		t.Errorf("ExpectedHeadOid = %q, want the head commit", got.Options.ExpectedHeadOid)
	}
	if got.RefID != "REF_88" {
		t.Errorf("RefID = %q, want the head branch", got.RefID)
	}
}

func TestEnterInTheHeadlineDoesNotMerge(t *testing.T) {
	if got := asked(t, press(openMerge(t), "tab"), "enter"); got != nil {
		if _, merged := got.(prview.MergeMsg); merged {
			t.Error("enter in the headline merged the pull request")
		}
	}
}

func TestUntickingTheDeleteRowKeepsTheBranch(t *testing.T) {
	m := press(openMerge(t), "tab", "tab", "tab", " ")

	got, ok := merged(t, press(m, "tab"), "enter")
	if !ok {
		t.Fatal("enter on the button asked for no merge")
	}
	if got.RefID != "" {
		t.Errorf("RefID = %q, want nothing once the box is unticked", got.RefID)
	}
}

func TestTheFormOffersNoDeleteWhereTheRepositoryDoesItself(t *testing.T) {
	methods := allMethods()
	methods.DeleteOnMerge = true

	box := formBox(t, openMergeOn(t, mergeableDetail(), mergeRepo(methods)))
	if strings.Contains(box, "after merging") {
		t.Errorf("the form offers a delete the repository is going to make anyway:\n%s", box)
	}
}

func TestTheFormOffersNoDeleteForAForksHead(t *testing.T) {
	d := mergeableDetail()
	d.CrossRepository = true

	box := formBox(t, openMergeOn(t, d, mergeRepo(allMethods())))
	if strings.Contains(box, "after merging") {
		t.Errorf("the form offers to delete a contributor's own branch:\n%s", box)
	}
}

func TestTheFormOffersNoDeleteWithoutABranch(t *testing.T) {
	d := mergeableDetail()
	d.HeadRefID = ""

	box := formBox(t, openMergeOn(t, d, mergeRepo(allMethods())))
	if strings.Contains(box, "after merging") {
		t.Errorf("the form offers to delete a branch that is not there:\n%s", box)
	}
}

func TestTheFormOffersTheDeleteOnAnOrdinaryPullRequest(t *testing.T) {
	box := formBox(t, openMerge(t))

	if !strings.Contains(box, "Delete fix-auth-retry after merging") {
		t.Errorf("the form does not offer to delete the head branch at all:\n%s", box)
	}
}

func TestABypassMergeSaysSo(t *testing.T) {
	d := mergeableDetail()
	d.Merge = gh.MergeBlocked
	d.Viewer.CanMergeAsAdmin = true

	box := formBox(t, openMergeOn(t, d, mergeRepo(allMethods())))
	if !strings.Contains(box, "Bypasses branch protection on main") {
		t.Errorf("a blocked merge does not say it is overriding one:\n%s", box)
	}
}

func TestAnOrdinaryMergeDoesNotClaimToBypassAnything(t *testing.T) {
	if box := formBox(t, openMerge(t)); strings.Contains(box, "Bypasses") {
		t.Errorf("a clean merge claims to override a rule:\n%s", box)
	}
}

func TestTheButtonIsInertWithNoHeadline(t *testing.T) {
	m := press(openMerge(t), "tab")
	for range 30 {
		m = press(m, "ctrl+u")
	}

	if got := asked(t, press(m, "tab", "tab", "tab"), "enter"); got != nil {
		t.Errorf("the button merged with an empty headline, sending %T", got)
	}
}

func TestEscapeClosesTheFormWithoutMerging(t *testing.T) {
	m := press(openMerge(t), "esc")

	if got := asked(t, m, "enter"); got != nil {
		t.Errorf("a key after esc sent %T, want the form gone", got)
	}
	if strings.Contains(stripANSI(m.View()), "Rebase and merge") {
		t.Error("the form is still on the screen after esc")
	}
}

func TestTheFormCapturesTheKeyboard(t *testing.T) {
	if !openMerge(t).Capturing() {
		t.Error("the form is up and the screen does not report capturing")
	}
}

func TestTheFormKeepsItsButtonOnAShortTerminal(t *testing.T) {
	for _, height := range []int{60, 30, 24, 20, 19} {
		m := onRailRow(t, detailed(held(mergeableDetail()), 200, height), "Ready to merge")
		m, _ = key(m, "enter")
		m.SetRepo(mergeRepo(allMethods()))

		box := formBox(t, m)
		if !strings.Contains(box, "Squash and merge") {
			t.Fatalf("at %d rows the form did not open:\n%s", height, box)
		}

		rows := strings.Split(strings.TrimRight(box, "\n"), "\n")
		var footer string
		for _, row := range rows {
			if strings.Contains(row, "esc cancel") {
				footer = row
			}
		}
		if footer == "" {
			t.Errorf("at %d rows the footer is off the bottom:\n%s", height, box)
			continue
		}
		if !strings.Contains(footer, "Merge") {
			t.Errorf("at %d rows the button is clipped off its row: %q", height, footer)
		}

		if last := rows[len(rows)-1]; !strings.Contains(last, "╰") || !strings.Contains(last, "╯") {
			t.Errorf("at %d rows the modal is clipped and never closes: %q", height, last)
		}
	}
}

func TestTheChordMergesFromTheCommitMessage(t *testing.T) {
	m := press(openMerge(t), "tab", "tab")

	_, cmd := chord(m)
	got, ok := runCmd(cmd).(prview.MergeMsg)
	if !ok {
		t.Fatalf("the chord sent %T, want a MergeMsg", runCmd(cmd))
	}
	if got.Options.Method != gh.MergeMethodSquash {
		t.Errorf("method = %q, want the one the form was left on", got.Options.Method)
	}
}

func TestEnterOnTheDeleteRowTogglesIt(t *testing.T) {
	m := press(openMerge(t), "tab", "tab", "tab", "enter")

	got, ok := merged(t, press(m, "tab"), "enter")
	if !ok {
		t.Fatal("enter on the button asked for no merge")
	}
	if got.RefID != "" {
		t.Errorf("RefID = %q, want nothing once enter has unticked the box", got.RefID)
	}
}

func TestEnterOnAMethodRowMovesOn(t *testing.T) {
	m := press(openMerge(t), "enter", "!")

	box := formBox(t, m)
	if !strings.Contains(box, "Fix auth retry (#412)!") {
		t.Errorf("enter on the method row did not move on to the headline:\n%s", box)
	}
}

func TestShiftTabWalksTheFormBackwards(t *testing.T) {
	m, _ := openMerge(t).Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})

	got, ok := merged(t, m, "enter")
	if !ok {
		t.Fatal("shift+tab did not reach the button, so enter merged nothing")
	}
	if got.ID != "PR_412" {
		t.Errorf("ID = %q, want the pull request on screen", got.ID)
	}
}

func TestTheFormFallsBackWhenSquashIsForbidden(t *testing.T) {
	box := formBox(t, openMergeOn(t, mergeableDetail(),
		mergeRepo(gh.MergeMethods{Merge: true, Rebase: true})))

	if !chosenMethod(box, "Create a merge commit") {
		t.Errorf("the form did not fall back to the first method allowed:\n%s", box)
	}
}

func TestALongBranchNameDoesNotWidenTheForm(t *testing.T) {
	short := mergeableDetail()
	short.HeadRefName = "fix"

	long := mergeableDetail()
	long.HeadRefName = "feature/zno-48-m4-merge-from-the-rail-and-then-some-more-besides"

	got := formBox(t, openMergeOn(t, long, mergeRepo(allMethods())))
	want := formBox(t, openMergeOn(t, short, mergeRepo(allMethods())))

	if wide, narrow := boxWidth(got), boxWidth(want); wide != narrow {
		t.Errorf("the form is %d columns over a long branch and %d over a short one", wide, narrow)
	}

	if !strings.Contains(got, "after merging") {
		t.Errorf("the row lost the words saying what it does:\n%s", got)
	}
	if !strings.Contains(got, "Delete feature/zno-48") {
		t.Errorf("the branch lost the head of its name:\n%s", got)
	}
}

func boxWidth(box string) int {
	var wide int
	for _, row := range strings.Split(box, "\n") {
		wide = max(wide, len([]rune(stripANSI(row))))
	}
	return wide
}

func TestOnlyTheChosenMethodIsNotMuted(t *testing.T) {
	frame := openMerge(t).View()

	faint, primary := fgSeq(testTheme.Subtle), fgSeq(testTheme.Text)

	if got := colorBefore(t, frame, "Squash and merge"); got != primary {
		t.Errorf("the chosen method renders in %s, want the primary colour %s", got, primary)
	}
	for _, muted := range []string{"Create a merge commit", "Rebase and merge"} {
		if got := colorBefore(t, frame, muted); got != faint {
			t.Errorf("%q renders in %s, want it muted at %s", muted, got, faint)
		}
	}

	if got := colorBefore(t, frame, "Method"); got != primary {
		t.Errorf("the Method heading renders in %s, want the box titles' %s", got, primary)
	}
}

func colorBefore(t *testing.T, frame, needle string) string {
	t.Helper()

	at := strings.Index(frame, needle)
	if at < 0 {
		t.Fatalf("%q is nowhere in the frame", needle)
	}

	var color string
	for _, m := range sgr.FindAllStringSubmatch(frame[:at], -1) {
		if m[1] == "" {
			color = ""
		}
		parts := strings.Split(m[1], ";")
		for i := 0; i < len(parts); i++ {
			switch p := parts[i]; {
			case p == "0", p == "39":
				color = ""
			case p == "38" && i+4 < len(parts) && parts[i+1] == "2":
				color = strings.Join(parts[i:i+5], ";")
				i += 4
			case p == "38" && i+2 < len(parts) && parts[i+1] == "5":
				color = strings.Join(parts[i:i+3], ";")
				i += 2
			case len(p) == 2 && (p[0] == '3' || p[0] == '9') && p[1] >= '0' && p[1] <= '7':
				color = p
			}
		}
	}
	return color
}

var sgr = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

func TestALongHeadlineScrollsAsItIsTyped(t *testing.T) {
	d := mergeableDetail()
	d.SquashCommit.Headline = "Fix the auth retry backoff loop so it stops hammering the endpoint (#412)"

	m := press(openMergeOn(t, d, mergeRepo(allMethods())), "tab")

	before := formBox(t, m)
	after := formBox(t, press(m, "X", "Y", "Z"))

	if before == after {
		t.Errorf("typing changed nothing on screen, so the field never scrolled:\n%s", after)
	}
	if !strings.Contains(after, "XYZ") {
		t.Errorf("the typed characters are nowhere on screen:\n%s", after)
	}
}

func TestTheFormFollowsAResize(t *testing.T) {
	d := mergeableDetail()
	d.SquashCommit.Headline = "Fix the auth retry backoff loop so it stops hammering the endpoint (#412)"

	m := openMergeOn(t, d, mergeRepo(allMethods()))

	wide := boxWidth(formBox(t, m))
	m.SetSize(48, 40)
	narrow := boxWidth(formBox(t, m))
	if narrow >= wide {
		t.Fatalf("setup: the form is %d columns after the resize and %d before, so nothing narrowed", narrow, wide)
	}

	if after := formBox(t, press(m, "tab", "X", "Y", "Z")); !strings.Contains(after, "XYZ") {
		t.Errorf("what was typed is off the edge of the box the resize left:\n%s", after)
	}
}

func TestAClosedPullRequestOffersNoMerge(t *testing.T) {
	for _, state := range []gh.PRState{gh.PRStateClosed, gh.PRStateMerged} {
		t.Run(string(state), func(t *testing.T) {
			d := mergeableDetail()
			d.State = state

			m := press(detailed(held(d), 200, 60), "1")
			for range 30 {
				m = press(m, "j")
				if row := markedRailRow(t, m.View()); strings.Contains(row, "Ready to merge") {
					t.Fatalf("the ring stopped on a live merge control for a %s pull request", state)
				}
			}
		})
	}
}

func TestAMergeInFlightKeepsItsRowOnTheRing(t *testing.T) {
	d := mergeableDetail()
	d.State = gh.PRStateMerged

	writing := held(d)
	writing.StateWriting = true

	m := press(detailed(writing, 200, 60), "1")

	var reached []string
	for range 30 {
		m = press(m, "j")
		reached = append(reached, markedRailRow(t, m.View()))
	}

	var merge, base bool
	for _, row := range reached {
		merge = merge || strings.Contains(row, "Merged into") || strings.Contains(row, "Ready to merge")
		base = base || strings.Contains(row, "main")
	}
	if !merge {
		t.Errorf("the Merge row left the ring while its own write was out: %q", reached)
	}
	if !base {
		t.Errorf("the Base row left the ring while a lifecycle write was out: %q", reached)
	}
}

func TestPastingReachesTheCommitMessage(t *testing.T) {
	m := press(openMerge(t), "tab", "tab")
	m, _ = m.Update(tea.PasteMsg{Content: "pasted from somewhere else"})

	box := formBox(t, m)
	if !strings.Contains(box, "pasted from somewhere else") {
		t.Errorf("the paste never reached the message box:\n%s", box)
	}
	if box := formBox(t, press(m, "tab", "tab", "tab", "up")); !strings.Contains(box, "pasted from") {
		t.Errorf("a method change threw away what was pasted:\n%s", box)
	}
}

func TestTheHintNamesAKeyThatWorksFromTheRowItIsOn(t *testing.T) {
	tests := []struct {
		row  string
		want string
	}{
		{row: "the method", want: "j/k method"},
		{row: "the headline", want: "tab to the button to merge"},
		{row: "the message", want: "tab to the button to merge"},
		{row: "delete the branch", want: "space toggle"},
		{row: "the button", want: "⏎ merge"},
	}

	m := openMerge(t)
	for i, tt := range tests {
		if i > 0 {
			m = press(m, "tab")
		}

		box := formBox(t, m)
		if !strings.Contains(box, tt.want) {
			t.Errorf("on %s the hint does not name %q:\n%s", tt.row, tt.want, box)
		}
		if tt.want != "⏎ merge" && strings.Contains(box, "⏎ merge") {
			t.Errorf("on %s the hint says enter merges, and it does not:\n%s", tt.row, box)
		}
	}
}

func TestTheHintDoesNotResizeTheForm(t *testing.T) {
	m := openMerge(t)

	want := boxWidth(formBox(t, m))
	for i := range 4 {
		m = press(m, "tab")
		if got := boxWidth(formBox(t, m)); got != want {
			t.Errorf("after %d tabs the form is %d columns, want %d", i+1, got, want)
		}
	}
}

func TestARepositoryAnswerDoesNotOpenOverAnotherRow(t *testing.T) {
	for _, tt := range []struct{ name, row, walkTo string }{
		{"the merge form", "Ready to merge", "+ Add reviewer"},
		{"a label picker", "+ Add label", "+ Add assignee"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := onRailRow(t, detailed(held(mergeableDetail()), 200, 60), tt.row)

			m, _ = key(m, "enter")
			m = onRailRow(t, m, tt.walkTo)
			m.SetRepo(mergeRepo(allMethods()))

			if out := stripANSI(m.View()); strings.Contains(out, "╭─Merge #412") ||
				strings.Contains(out, "╭─Labels") {
				t.Errorf("a modal opened over the row the reader walked to:\n%s", out)
			}
			if m.Capturing() {
				t.Error("the screen is capturing the keyboard for a modal nobody asked for here")
			}
		})
	}
}

func TestARepositoryAnswerStillOpensWhereTheReaderStayed(t *testing.T) {
	m := onRailRow(t, detailed(held(mergeableDetail()), 200, 60), "Ready to merge")

	m, _ = key(m, "enter")
	m.SetRepo(mergeRepo(allMethods()))

	if !strings.Contains(stripANSI(m.View()), "Squash and merge") {
		t.Error("the form did not open for a reader still standing on the row")
	}
}

func TestALongBaseNameDoesNotWidenTheBypassWarning(t *testing.T) {
	d := mergeableDetail()
	d.Merge = gh.MergeBlocked
	d.Viewer.CanMergeAsAdmin = true
	d.BaseRefName = "release/2026.08-the-long-lived-integration-branch-nobody-renamed"

	got := boxWidth(formBox(t, openMergeOn(t, d, mergeRepo(allMethods()))))
	want := boxWidth(formBox(t, openMerge(t)))

	if got != want {
		t.Errorf("the form is %d columns over a long base and %d over a short one", got, want)
	}
}
