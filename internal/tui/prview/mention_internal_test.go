package prview

import (
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

// A border and a gutter on each side of comp.Modal's rows.
const mentionModalChrome = 4

func mentionFixture() gh.PullRequestDetail {
	now := time.Now()

	return gh.PullRequestDetail{
		PullRequest: gh.PullRequest{
			ID:         "PR_1",
			Repository: "acme/rocket",
			Author:     gh.Actor{Login: "author"},
		},
		Assignees: []gh.Actor{{ID: "U_1", Login: "assignee"}},
		Reviewers: []gh.Reviewer{
			{Actor: gh.Actor{Login: "reviewer"}},
			{Actor: gh.Actor{Login: "acme/maintainers"}, Team: true},
			{Actor: gh.Actor{Login: gh.CopilotLogin}},
		},
		Timeline: []gh.TimelineItem{
			{Kind: gh.TimelineLabeled, Actor: gh.Actor{Login: "labeller"}, Subject: "bug", CreatedAt: now},
			{
				Kind:      gh.TimelineComment,
				Actor:     gh.Actor{Login: "commenter"},
				CreatedAt: now,
				Comment:   &gh.Comment{ID: "IC_1", Author: gh.Actor{Login: "commenter"}, Body: "hi"},
			},
			{
				Kind:      gh.TimelineComment,
				CreatedAt: now,
				Comment:   &gh.Comment{ID: "IC_2", Body: "gone"},
			},
		},
		Threads: []gh.ReviewThread{
			{ID: "RT_1", Comments: []gh.Comment{{ID: "RC_1", Author: gh.Actor{Login: "threader"}}}},
		},
		Commits: []gh.Commit{
			{SHA: "abc1234", Author: gh.Actor{Login: "committer"}},
			{SHA: "def5678", AuthorName: "Drew White"},
		},
	}
}

func TestParticipantsLeadWithTheAuthorAndFollowThePage(t *testing.T) {
	want := []string{
		"author", "assignee", "reviewer",
		"labeller", "commenter",
		"threader", "committer",
	}
	if got := participants(mentionFixture()); !slices.Equal(got, want) {
		t.Errorf("participants = %q, want %q", got, want)
	}
}

func TestParticipantsLeaveTeamsOut(t *testing.T) {
	if got := participants(mentionFixture()); slices.Contains(got, "acme/maintainers") {
		t.Errorf("participants = %q, want the team left out", got)
	}
}

func TestParticipantsLeaveCopilotOut(t *testing.T) {
	if got := participants(mentionFixture()); slices.Contains(got, gh.CopilotLogin) {
		t.Errorf("participants = %q, want Copilot left out", got)
	}
}

func TestParticipantsLeaveADeletedAccountOut(t *testing.T) {
	if got := participants(mentionFixture()); slices.Contains(got, "") {
		t.Errorf("participants = %q, want no empty login", got)
	}
}

func TestParticipantsCountACommitAuthorOnlyWhenGitHubKnowsThem(t *testing.T) {
	got := participants(mentionFixture())
	if !slices.Contains(got, "committer") {
		t.Errorf("participants = %q, want the commit author GitHub knows", got)
	}
	if slices.Contains(got, "Drew White") {
		t.Errorf("participants = %q, want git's own author name left out", got)
	}
}

func TestParticipantsNeverOfferALabelName(t *testing.T) {
	if got := participants(mentionFixture()); slices.Contains(got, "bug") {
		t.Errorf("participants = %q, want the label's name left out", got)
	}
}

func TestMentionChoicesPutTheConversationAheadOfTheRepository(t *testing.T) {
	repo := []gh.Mention{{Login: "stranger"}, {Login: "reviewer"}}

	got := mentionChoices(repo, mentionFixture(), "")
	if len(got) == 0 {
		t.Fatal("mentionChoices = empty, want the conversation and the repository")
	}
	if got[0].Login != "author" {
		t.Errorf("mentionChoices[0] = %q, want the pull request's author", got[0].Login)
	}

	conv := slices.IndexFunc(got, func(m gh.Mention) bool { return m.Login == "reviewer" })
	repoOnly := slices.IndexFunc(got, func(m gh.Mention) bool { return m.Login == "stranger" })
	if conv > repoOnly {
		t.Errorf("mentionChoices = %+v, want the reviewer ahead of the stranger", got)
	}
}

func TestMentionChoicesLeaveTheViewerOut(t *testing.T) {
	got := mentionChoices(nil, mentionFixture(), "AUTHOR")
	if slices.ContainsFunc(got, func(m gh.Mention) bool { return m.Login == "author" }) {
		t.Errorf("mentionChoices = %+v, want the viewer left out whatever the case", got)
	}
}

func TestMentionChoicesDedupeALoginWhateverTheCase(t *testing.T) {
	repo := []gh.Mention{{Login: "Reviewer", Name: "Nikita Rushmanov"}}

	got := mentionChoices(repo, mentionFixture(), "")
	n := 0
	for _, m := range got {
		if m.Login == "reviewer" || m.Login == "Reviewer" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("mentionChoices = %+v, want one row for the reviewer and got %d", got, n)
	}
}

func TestMentionChoicesTakeTheRealNameFromTheRepositoryList(t *testing.T) {
	repo := []gh.Mention{{Login: "commenter", Name: "Sam Reed"}}

	got := mentionChoices(repo, mentionFixture(), "")
	i := slices.IndexFunc(got, func(m gh.Mention) bool { return m.Login == "commenter" })
	if i < 0 {
		t.Fatalf("mentionChoices = %+v, want a row for the commenter", got)
	}
	if want := "Sam Reed"; got[i].Name != want {
		t.Errorf("commenter's name = %q, want %q", got[i].Name, want)
	}
}

func TestMentionChoicesKeepAParticipantTheRepositoryPageDidNotReach(t *testing.T) {
	got := mentionChoices([]gh.Mention{{Login: "stranger"}}, mentionFixture(), "")
	if !slices.ContainsFunc(got, func(m gh.Mention) bool { return m.Login == "threader" }) {
		t.Errorf("mentionChoices = %+v, want the thread's author kept", got)
	}
}

func mentionRow(login, name string) mention {
	return mention{open: true, rows: []gh.Mention{{Login: login, Name: name}}}
}

func TestALongHandleIsClippedOnceAndKeepsThePopupInItsWidth(t *testing.T) {
	const width = 20
	n := mentionRow(strings.Repeat("z", 60), "Somebody With A Name")

	out := n.render(testTheme, "", 1, width)
	if got := lipgloss.Width(out); got > width+mentionModalChrome {
		t.Errorf("the popup is %d cells wide against a budget of %d:\n%s",
			got, width+mentionModalChrome, stripSeqs(out))
	}
	if n := strings.Count(stripSeqs(out), "@z"); n != 1 {
		t.Errorf("the handle is drawn %d times, want once:\n%s", n, stripSeqs(out))
	}
}

func TestALongNoteIsClippedToTheSameWidth(t *testing.T) {
	const width = 12
	n := mention{open: true}

	out := n.render(testTheme, "Could not read the repository", 0, width)
	if got := lipgloss.Width(out); got > width+mentionModalChrome {
		t.Errorf("the note makes the popup %d cells wide against a budget of %d:\n%s",
			got, width+mentionModalChrome, stripSeqs(out))
	}
}

func TestANoteUnderTheRowsIsClippedWithThem(t *testing.T) {
	const width = 14
	n := mentionRow("nkr", "Nikita Rushmanov")

	out := n.render(testTheme, "Could not read the repository", 1, width)
	if got := lipgloss.Width(out); got > width+mentionModalChrome {
		t.Errorf("the popup is %d cells wide against a budget of %d:\n%s",
			got, width+mentionModalChrome, stripSeqs(out))
	}
}
