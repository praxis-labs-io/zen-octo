package prview

// In the package because the candidate list never reaches a frame: it is a walk
// over a fetched detail returning logins, and exporting it so a black-box test
// could call it would widen the package for the test's convenience.

import (
	"slices"
	"testing"
	"time"

	"github.com/zen-octo/zen-octo/internal/gh"
)

// mentionFixture is a pull request with somebody in every place a login can
// hide: the author, an assignee, a reviewer, a team, a timeline event, a
// comment, a thread, and a commit.
func mentionFixture() gh.PullRequestDetail {
	now := time.Now()

	return gh.PullRequestDetail{
		PullRequest: gh.PullRequest{
			ID:         "PR_1",
			Repository: "zen-octo/zen-octo",
			Author:     gh.Actor{Login: "author"},
		},
		Assignees: []gh.Actor{{ID: "U_1", Login: "assignee"}},
		Reviewers: []gh.Reviewer{
			{Actor: gh.Actor{Login: "reviewer"}},
			{Actor: gh.Actor{Login: "zen-octo/maintainers"}, Team: true},
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
			// A deleted account, which GitHub answers as a null author.
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
	if got := participants(mentionFixture()); slices.Contains(got, "zen-octo/maintainers") {
		t.Errorf("participants = %q, want the team left out", got)
	}
}

func TestParticipantsLeaveCopilotOut(t *testing.T) {
	if got := participants(mentionFixture()); slices.Contains(got, gh.CopilotLogin) {
		t.Errorf("participants = %q, want Copilot left out", got)
	}
}

// A deleted account comes back as a null author, so its login is empty. Offered
// it would be a bare @ on a row that inserts nothing.
func TestParticipantsLeaveADeletedAccountOut(t *testing.T) {
	if got := participants(mentionFixture()); slices.Contains(got, "") {
		t.Errorf("participants = %q, want no empty login", got)
	}
}

// AuthorName is git's record of who wrote the commit, not a handle. @Drew White
// is not a mention anybody can be reached at.
func TestParticipantsCountACommitAuthorOnlyWhenGitHubKnowsThem(t *testing.T) {
	got := participants(mentionFixture())
	if !slices.Contains(got, "committer") {
		t.Errorf("participants = %q, want the commit author GitHub knows", got)
	}
	if slices.Contains(got, "Drew White") {
		t.Errorf("participants = %q, want git's own author name left out", got)
	}
}

// Subject is a handle on a review request and a label's name on a labelling.
// Read as a login it offers @bug.
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

// A participant reached through the timeline carries no name, and the
// repository's list is the only thing that has one for them.
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

// The repository's list is one page of a hundred. Somebody past it who is on
// this very pull request is exactly who a reply is addressed to.
func TestMentionChoicesKeepAParticipantTheRepositoryPageDidNotReach(t *testing.T) {
	got := mentionChoices([]gh.Mention{{Login: "stranger"}}, mentionFixture(), "")
	if !slices.ContainsFunc(got, func(m gh.Mention) bool { return m.Login == "threader" }) {
		t.Errorf("mentionChoices = %+v, want the thread's author kept", got)
	}
}
