package prview_test

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

const (
	reactRow    = "Thumbs down"
	reactModal  = "React"
	thumbsUp    = "👍"
	rocketPill  = "🚀"
	confusedRow = "Confused"
)

func reactive() gh.PullRequestDetail {
	d := sampleDetail()
	d.Viewer.CanReact = true
	d.Reactions = []gh.Reaction{{Content: gh.ReactionRocket, Count: 3}}

	for _, item := range d.Timeline {
		if c := item.Comment; c != nil {
			c.CanReact = true
		}
	}
	d.Timeline[0].Comment.Reactions = []gh.Reaction{
		{Content: gh.ReactionThumbsUp, Count: 4, Viewer: true},
		{Content: gh.ReactionEyes, Count: 1},
	}

	for i := range d.Threads {
		for j := range d.Threads[i].Comments {
			d.Threads[i].Comments[j].CanReact = true
		}
	}
	d.Threads[0].Comments[0].Reactions = []gh.Reaction{
		{Content: gh.ReactionHeart, Count: 2},
	}
	return d
}

func reacting(n int) prview.Model {
	return walked(detailed(held(reactive()), 200, 60), n)
}

func reactAsked(t *testing.T, m prview.Model, k string) prview.ReactMsg {
	t.Helper()

	got := asked(t, m, k)
	msg, ok := got.(prview.ReactMsg)
	if !ok {
		t.Fatalf("%s produced %T, want a ReactMsg", k, got)
	}
	return msg
}

func enter(m prview.Model) (prview.Model, tea.Cmd) {
	return m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
}

func TestReactOpensTheEight(t *testing.T) {
	out := stripANSI(press(reacting(2), "+").View())

	if !strings.Contains(out, reactModal) {
		t.Fatalf("+ opened no list:\n%s", out)
	}
	for _, want := range []string{"Thumbs up", reactRow, "Laugh", "Hooray",
		confusedRow, "Heart", "Rocket", "Eyes"} {
		if !strings.Contains(out, want) {
			t.Errorf("the list does not offer %q", want)
		}
	}
}

func TestTheListNamesWhatEachReactionAlreadyHas(t *testing.T) {
	out := stripANSI(press(reacting(2), "+").View())

	line := rowFor(t, out, "Thumbs up")
	if !strings.Contains(line, "4") {
		t.Errorf("the thumbs up row is %q, want the four it already has", line)
	}
	if got := rowFor(t, out, reactRow); strings.ContainsAny(got, "0123456789") {
		t.Errorf("a reaction nobody gave is numbered: %q", got)
	}
}

func TestReactIsInertWhereGitHubSaysNo(t *testing.T) {
	m := walked(detailed(held(sampleDetail()), 200, 60), 2)

	if out := stripANSI(press(m, "+").View()); strings.Contains(out, reactRow) {
		t.Errorf("+ opened a list on a comment GitHub will not take one for:\n%s", out)
	}
}

func TestEnterOnAReactionAlreadyGivenTakesItBack(t *testing.T) {
	got := reactAsked(t, press(reacting(2), "+"), "enter")

	want := prview.ReactMsg{
		ID: "PR_412", SubjectID: "IC_octobot", CommentID: "IC_octobot",
		Content: gh.ReactionThumbsUp,
	}
	if got != want {
		t.Errorf("enter asked for %+v, want %+v", got, want)
	}
}

func TestEnterOnAReactionNotGivenAddsIt(t *testing.T) {
	m := press(reacting(2), "+")

	m, _ = arrow(m, tea.KeyDown)

	_, cmd := enter(m)
	if cmd == nil {
		t.Fatal("enter sent nothing")
	}
	msg, ok := cmd().(prview.ReactMsg)
	if !ok {
		t.Fatalf("enter produced %T, want a ReactMsg", cmd())
	}
	if msg.Content != gh.ReactionThumbsDown || !msg.On {
		t.Errorf("enter asked for %+v, want thumbs down added", msg)
	}
}

func TestReactOnTheDescriptionAddressesThePullRequest(t *testing.T) {
	got := reactAsked(t, press(reacting(1), "+"), "enter")

	if got.SubjectID != "PR_412" {
		t.Errorf("SubjectID = %q, want the pull request's own node", got.SubjectID)
	}
	if got.CommentID != "" || got.ThreadID != "" {
		t.Errorf("msg = %+v, want no comment for the store to look for", got)
	}
}

func TestReactReachesAReply(t *testing.T) {
	got := reactAsked(t, press(reacting(tabReply), "+"), "enter")

	if got.CommentID != "RC_4" || got.ThreadID != "RT_1" {
		t.Errorf("msg = %+v, want the reply inside its thread", got)
	}
	if !got.On {
		t.Error("a reply nobody has reacted to came back as a removal")
	}
}

func TestReactOnAThreadReachesTheCommentThatOpenedIt(t *testing.T) {
	got := reactAsked(t, press(reacting(tabThread), "+"), "enter")

	if got.CommentID != "RC_1" || got.ThreadID != "RT_1" {
		t.Errorf("msg = %+v, want the comment the thread was opened with", got)
	}
}

func TestThePillsRenderOnACardNobodyIsOn(t *testing.T) {
	out := stripANSI(detailed(held(reactive()), 200, 60).View())

	if !strings.Contains(out, thumbsUp+" 4") {
		t.Errorf("the pills are missing from an unfocused card:\n%s", out)
	}
	if !strings.Contains(out, rocketPill+" 3") {
		t.Error("the description's own pills are missing")
	}
}

func TestThePillsDoNotMoveWhenTheRingArrives(t *testing.T) {
	quiet := strings.Count(stripANSI(detailed(held(reactive()), 200, 60).View()), "\n")
	lit := strings.Count(stripANSI(reacting(2).View()), "\n")

	if quiet != lit {
		t.Errorf("the page is %d lines with the ring on the card and %d without", lit, quiet)
	}
}

func TestACardWithNoReactionsDrawsNoRow(t *testing.T) {
	d := reactive()
	d.Timeline[0].Comment.Reactions = nil

	out := stripANSI(detailed(held(d), 200, 60).View())
	if strings.Contains(out, thumbsUp) {
		t.Errorf("a card with no reactions drew a pill:\n%s", out)
	}
}

func TestTheCardNamesTheReactKey(t *testing.T) {
	if got := stripANSI(reacting(2).View()); !strings.Contains(got, "+ react") {
		t.Errorf("the focused card does not name the react key:\n%s", got)
	}

	m := walked(detailed(held(sampleDetail()), 200, 60), 2)
	if got := stripANSI(m.View()); strings.Contains(got, "+ react") {
		t.Error("a card GitHub will take no reaction for names the key")
	}
}

func TestAReactionStillOutTakesNoSecondPress(t *testing.T) {
	d := reactive()
	d.Timeline[0].Comment.Reactions = []gh.Reaction{
		{Content: gh.ReactionThumbsUp, Count: 4, Viewer: true, Pending: true},
	}

	m := press(walked(detailed(held(d), 200, 60), 2), "+")
	if _, cmd := enter(m); cmd != nil {
		t.Errorf("enter on a reaction already being written sent %+v", cmd())
	}
}

func TestPillsDoNotMoveAnOpenBox(t *testing.T) {
	withPills := boxOffset(t, press(walked(viewing(reactive(), 200, 60), 1), "e").View())

	bare := reactive()
	bare.Reactions = nil
	withoutPills := boxOffset(t, press(walked(viewing(bare, 200, 60), 1), "e").View())

	if withPills != withoutPills {
		t.Errorf("the box opens %d lines under its heading with pills and %d without",
			withPills, withoutPills)
	}
}

func boxOffset(t *testing.T, frame string) int {
	t.Helper()

	lines := strings.Split(stripANSI(frame), "\n")
	head, box := -1, -1
	for i, line := range lines {
		if head < 0 && strings.Contains(line, "edit this description") {
			head = i
		}
		if strings.Contains(line, "Save") {
			box = i
			break
		}
	}
	if head < 0 || box < 0 {
		t.Fatalf("no box opened on the description:\n%s", frame)
	}
	return box - head
}

func TestThePillsGoWhileTheBoxIsOverTheBlock(t *testing.T) {
	m := press(walked(viewing(reactive(), 200, 60), 1), "e")

	out := stripANSI(m.View())
	if !strings.Contains(out, "Save") {
		t.Fatalf("e opened no box:\n%s", out)
	}
	if strings.Contains(out, rocketPill) {
		t.Errorf("the description's pills are under the box's own button:\n%s", out)
	}
}

func TestThePillsComeBackWhenTheBoxCloses(t *testing.T) {
	m := press(press(walked(viewing(reactive(), 200, 60), 1), "e"), "esc")

	if out := stripANSI(m.View()); !strings.Contains(out, rocketPill+" 3") {
		t.Errorf("the pills did not come back after the box closed:\n%s", out)
	}
}

func TestJAndKWalkTheReactionList(t *testing.T) {
	m := press(press(reacting(2), "+"), "j")

	if out := stripANSI(m.View()); strings.Contains(out, "No match") {
		t.Fatalf("j filtered the list instead of walking it:\n%s", out)
	}

	got := reactAsked(t, m, "enter")
	if got.Content != gh.ReactionThumbsDown {
		t.Errorf("j then enter asked for %v, want the second row", got.Content)
	}
}

func TestAWideRowOfPillsFoldsRatherThanClipping(t *testing.T) {
	d := reactive()
	all := make([]gh.Reaction, 0, len(gh.ReactionOrder))
	for i, c := range gh.ReactionOrder {
		all = append(all, gh.Reaction{Content: c, Count: i + 11})
	}
	d.Timeline[0].Comment.Reactions = all

	for _, width := range []int{50, 44} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			out := stripANSI(detailed(held(d), width, 40).View())

			for _, r := range all {
				want := reactionGlyphFor(r.Content) + " " + strconv.Itoa(r.Count)
				if !strings.Contains(out, want) {
					t.Errorf("the card lost %q off the end of its pill row:\n%s", want, out)
				}
			}
		})
	}
}

func reactionGlyphFor(c gh.ReactionContent) string {
	return map[gh.ReactionContent]string{
		gh.ReactionThumbsUp:   "👍",
		gh.ReactionThumbsDown: "👎",
		gh.ReactionLaugh:      "😄",
		gh.ReactionHooray:     "🎉",
		gh.ReactionConfused:   "😕",
		gh.ReactionHeart:      "❤️",
		gh.ReactionRocket:     "🚀",
		gh.ReactionEyes:       "👀",
	}[c]
}

func TestAReactionBeingTakenBackReadsAsPending(t *testing.T) {
	d := reactive()
	d.Timeline[0].Comment.Reactions = []gh.Reaction{
		{Content: gh.ReactionThumbsUp, Viewer: true, Pending: true},
	}

	out := stripANSI(detailed(held(d), 200, 60).View())
	if !strings.Contains(out, thumbsUp+" ·") {
		t.Errorf("a reaction on its way out does not say so:\n%s", out)
	}
	if strings.Contains(out, thumbsUp+" 0") {
		t.Error("a reaction still being written reads as one nobody gave")
	}
}

func TestPillsRenderOnAThreadInTheDiff(t *testing.T) {
	d := reactive()
	m := detailed(held(d), 200, 60)
	m.SetFiles(loadedFiles(sampleFiles(), 0))

	out := stripANSI(press(m, "]", "]", "]").View())
	if !strings.Contains(out, "❤️ 2") {
		t.Errorf("the thread's pills are missing from the diff:\n%s", out)
	}
}

func TestEscapeClosesTheListWithoutWriting(t *testing.T) {
	m := press(reacting(2), "+")

	m, cmd := m.Update(escape())
	if cmd != nil {
		t.Errorf("esc sent %+v", cmd())
	}
	if out := stripANSI(m.View()); strings.Contains(out, reactRow) {
		t.Error("esc left the list up")
	}
}

func TestPillsDoNotBreakTheFrameWidth(t *testing.T) {
	for _, size := range []struct{ width, height int }{
		{width: 200, height: 40},
		{width: 100, height: 24},
		{width: 60, height: 16},
	} {
		name := strconv.Itoa(size.width) + "x" + strconv.Itoa(size.height)
		t.Run(name, func(t *testing.T) {
			frame := detailed(held(reactive()), size.width, size.height).View()

			for i, line := range strings.Split(frame, "\n") {
				if w := lipgloss.Width(line); w != size.width {
					t.Errorf("line %d is %d cells wide, want %d: %q", i, w, size.width, line)
				}
			}
		})
	}
}

func TestTheReactionListDoesNotBreakTheFrameWidth(t *testing.T) {
	frame := press(reacting(2), "+").View()

	for i, line := range strings.Split(frame, "\n") {
		if w := lipgloss.Width(line); w != 200 {
			t.Errorf("line %d is %d cells wide, want 200: %q", i, w, line)
		}
	}
}

func rowFor(t *testing.T, frame, marker string) string {
	t.Helper()

	for _, line := range strings.Split(frame, "\n") {
		at := strings.Index(line, marker)
		if at < 0 {
			continue
		}
		open := strings.LastIndex(line[:at], "│")
		shut := strings.Index(line[at:], "│")
		if open < 0 || shut < 0 {
			return line
		}
		return line[open : at+shut]
	}
	t.Fatalf("no line holds %q:\n%s", marker, frame)
	return ""
}
