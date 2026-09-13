package gh

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

// Only the event types something here writes or renders, and no REBASE message, which GitHub answers empty.
const pullRequestQuery = `
query PullRequestDetail($id: ID!, $head: String!) {
  rateLimit { limit cost remaining resetAt }
  node(id: $id) {
    ... on PullRequest {
      id
      number
      title
      url
      isDraft
      state
      viewerCanUpdate
      viewerCanClose
      viewerCanReopen
      viewerCanAssign
      viewerCanMergeAsAdmin
      viewerCanReact
      reactionGroups { content viewerHasReacted reactors { totalCount } }
      isCrossRepository
      createdAt
      updatedAt
      additions
      deletions
      changedFiles
      headRefName
      baseRefName
      headRefOid
      headRef { id }
      reviewDecision
      mergeable
      mergeStateStatus
      body
      author { login }
      repository { nameWithOwner }

      mergeHeadline: viewerMergeHeadlineText(mergeType: MERGE)
      mergeBody: viewerMergeBodyText(mergeType: MERGE)
      squashHeadline: viewerMergeHeadlineText(mergeType: SQUASH)
      squashBody: viewerMergeBodyText(mergeType: SQUASH)

      baseRef { compare(headRef: $head) { behindBy } }

      labels(first: 100) { nodes { id name } }
      assignees(first: 10) { nodes { id login } }
      reviewRequests(first: 10) {
        nodes {
          requestedReviewer {
            ... on User { login }
            ... on Bot { login }
            ... on Team { slug organization { login } }
          }
        }
      }

      comments(first: 100) {
        totalCount
        nodes {
          id
          author { login }
          createdAt
          body
          viewerDidAuthor
          viewerCanUpdate
          viewerCanDelete
          viewerCanReact
          reactionGroups { content viewerHasReacted reactors { totalCount } }
        }
      }

      reviews(first: 100) {
        totalCount
        nodes {
          id
          state
          body
          submittedAt
          author { login }
          viewerDidAuthor
          viewerCanUpdate
          viewerCanDelete
          viewerCanReact
          reactionGroups { content viewerHasReacted reactors { totalCount } }
        }
      }

      reviewThreads(first: 100) {
        totalCount
        nodes {
          id
          isResolved
          isOutdated
          viewerCanReply
          viewerCanResolve
          viewerCanUnresolve
          path
          line
          startLine
          originalLine
          originalStartLine
          diffSide
          comments(first: 50) {
            totalCount
            nodes {
              id
              author { login }
              createdAt
              body
              diffHunk
              viewerDidAuthor
              viewerCanUpdate
              viewerCanDelete
              viewerCanReact
              reactionGroups { content viewerHasReacted reactors { totalCount } }
              pullRequestReview { id }
            }
          }
        }
      }

      commits(last: 100) {
        totalCount
        nodes {
          commit {
            oid
            abbreviatedOid
            messageHeadline
            messageBody
            committedDate
            author { name user { login } }
            statusCheckRollup { state }
          }
        }
      }

      timelineItems(last: 100, itemTypes: [
        MERGED_EVENT, CLOSED_EVENT, REOPENED_EVENT,
        READY_FOR_REVIEW_EVENT, CONVERT_TO_DRAFT_EVENT, HEAD_REF_FORCE_PUSHED_EVENT,
        LABELED_EVENT, UNLABELED_EVENT, ASSIGNED_EVENT, UNASSIGNED_EVENT,
        REVIEW_REQUESTED_EVENT, REVIEW_REQUEST_REMOVED_EVENT, BASE_REF_CHANGED_EVENT
      ]) {
        filteredCount
        nodes {
          __typename
          ... on MergedEvent { createdAt actor { login } }
          ... on ClosedEvent { createdAt actor { login } }
          ... on ReopenedEvent { createdAt actor { login } }
          ... on ReadyForReviewEvent { createdAt actor { login } }
          ... on ConvertToDraftEvent { createdAt actor { login } }
          ... on HeadRefForcePushedEvent { createdAt actor { login } }
          ... on LabeledEvent { createdAt actor { login } label { name } }
          ... on UnlabeledEvent { createdAt actor { login } label { name } }
          ... on AssignedEvent {
            createdAt actor { login }
            assignee { ... on User { login } ... on Bot { login } }
          }
          ... on UnassignedEvent {
            createdAt actor { login }
            assignee { ... on User { login } ... on Bot { login } }
          }
          ... on ReviewRequestedEvent {
            createdAt actor { login }
            requestedReviewer {
              ... on User { login }
              ... on Bot { login }
              ... on Team { slug organization { login } }
            }
          }
          ... on ReviewRequestRemovedEvent {
            createdAt actor { login }
            requestedReviewer {
              ... on User { login }
              ... on Bot { login }
              ... on Team { slug organization { login } }
            }
          }
          ... on BaseRefChangedEvent {
            createdAt actor { login } previousRefName currentRefName
          }
        }
      }
` + rollupSelection + `
    }
  }
}`

// A pointer because GitHub returns a deleted account as null, not a blank login.
type actorNode *struct{ Login string }

type reviewerNode *struct {
	Login string
	Slug  string

	Organization struct{ Login string }
}

func subject(n reviewerNode) string {
	if n == nil {
		return ""
	}
	return cmp.Or(n.Login, teamHandle(n.Organization.Login, n.Slug))
}

// No timestamp: a review sorts by submittedAt where a comment has createdAt.
type commentNode struct {
	ID              string
	Author          actorNode
	Body            string
	ViewerDidAuthor bool
	ViewerCanUpdate bool
	ViewerCanDelete bool
	ViewerCanReact  bool
	ReactionGroups  []reactionGroup
}

func (n commentNode) comment(kind CommentKind, at time.Time) Comment {
	return Comment{
		Kind:            kind,
		ID:              n.ID,
		Author:          login(n.Author),
		CreatedAt:       at,
		Body:            n.Body,
		ViewerDidAuthor: n.ViewerDidAuthor,
		CanEdit:         n.ViewerCanUpdate,
		CanDelete:       n.ViewerCanDelete,
		CanReact:        n.ViewerCanReact,
		Reactions:       reactions(n.ReactionGroups),
	}
}

type pullRequestResponse struct {
	RateLimit struct {
		Limit     int
		Cost      int
		Remaining int
		ResetAt   time.Time
	}

	Node struct {
		ID                    string
		Number                int
		Title                 string
		URL                   string
		IsDraft               bool
		State                 string
		ViewerCanUpdate       bool
		ViewerCanClose        bool
		ViewerCanReopen       bool
		ViewerCanAssign       bool
		ViewerCanMergeAsAdmin bool
		ViewerCanReact        bool
		ReactionGroups        []reactionGroup
		IsCrossRepository     bool
		CreatedAt             time.Time
		UpdatedAt             time.Time
		Additions             int
		Deletions             int
		ChangedFiles          int
		HeadRefName           string
		BaseRefName           string
		HeadRefOid            string

		// Null once the branch is deleted.
		HeadRef *struct{ ID string }

		MergeHeadline  string
		MergeBody      string
		SquashHeadline string
		SquashBody     string

		ReviewDecision   string
		Mergeable        string
		MergeStateStatus string
		Body             string
		Author           actorNode
		Repository       struct{ NameWithOwner string }

		BaseRef *struct {
			Compare *struct{ BehindBy int }
		}

		Labels struct {
			Nodes []struct{ ID, Name string }
		}
		Assignees struct {
			Nodes []struct{ ID, Login string }
		}

		ReviewRequests struct {
			Nodes []struct {
				RequestedReviewer reviewerNode
			}
		}

		Comments struct {
			TotalCount int
			Nodes      []struct {
				commentNode
				CreatedAt time.Time
			}
		}

		Reviews struct {
			TotalCount int
			Nodes      []struct {
				commentNode
				State       string
				SubmittedAt time.Time
			}
		}

		ReviewThreads struct {
			TotalCount int
			Nodes      []struct {
				ID                 string
				IsResolved         bool
				IsOutdated         bool
				ViewerCanReply     bool
				ViewerCanResolve   bool
				ViewerCanUnresolve bool
				Path               string
				Line               int
				StartLine          int
				OriginalLine       int
				OriginalStartLine  int
				DiffSide           string
				Comments           struct {
					TotalCount int
					Nodes      []struct {
						commentNode
						CreatedAt         time.Time
						DiffHunk          string
						PullRequestReview *struct{ ID string }
					}
				}
			}
		}

		Commits struct {
			TotalCount int
			Nodes      []struct {
				Commit struct {
					OID             string
					AbbreviatedOID  string
					MessageHeadline string
					MessageBody     string
					CommittedDate   time.Time
					Author          *struct {
						Name string
						User actorNode
					}
					StatusCheckRollup *struct{ State string }
				}
			}
		}

		TimelineItems struct {
			// totalCount would count subscriptions and mentions this never renders.
			FilteredCount int
			Nodes         []struct {
				Typename  string `json:"__typename"`
				CreatedAt time.Time
				Actor     actorNode

				Label             *struct{ Name string }
				Assignee          reviewerNode
				RequestedReviewer reviewerNode
				PreviousRefName   string
				CurrentRefName    string
			}
		}

		StatusCheckRollup rollupNode
	}
}

// GitHub answers a merged pull request whole, plus a NOT_FOUND on the compare since the head branch is gone.
func deletedHeadRef(err error) bool {
	var gqlErr *api.GraphQLError
	return errors.As(err, &gqlErr) && gqlErr.Match("NOT_FOUND", "node.baseRef.compare")
}

// PullRequest fetches the detail for a pull request's node id; headRef is its head branch name.
func (c *Client) PullRequest(ctx context.Context, id, headRef string) (DetailResult, error) {
	var resp pullRequestResponse
	vars := map[string]any{"id": id, "head": headRef}

	headless := false
	if err := c.gql.DoWithContext(ctx, pullRequestQuery, vars, &resp); err != nil {
		if !deletedHeadRef(err) {
			return DetailResult{}, fmt.Errorf("fetching pull request (%s): %w", id, classify(err))
		}
		headless = true
	}

	n := resp.Node
	if n.ID == "" {
		return DetailResult{}, fmt.Errorf("fetching pull request (%s): no pull request behind that id", id)
	}

	detail := PullRequestDetail{
		PullRequest: PullRequest{
			ID:             n.ID,
			Number:         n.Number,
			Title:          n.Title,
			URL:            n.URL,
			Repository:     n.Repository.NameWithOwner,
			Author:         login(n.Author),
			State:          PRState(n.State),
			IsDraft:        n.IsDraft,
			HeadRefName:    n.HeadRefName,
			BaseRefName:    n.BaseRefName,
			Additions:      n.Additions,
			Deletions:      n.Deletions,
			ChangedFiles:   n.ChangedFiles,
			Comments:       n.Comments.TotalCount + n.ReviewThreads.TotalCount,
			ReviewDecision: ReviewDecision(n.ReviewDecision),
			CreatedAt:      n.CreatedAt,
			UpdatedAt:      n.UpdatedAt,
		},
		Body:            n.Body,
		Reactions:       reactions(n.ReactionGroups),
		Merge:           mergeState(n.Mergeable, n.MergeStateStatus),
		HeadRefOid:      n.HeadRefOid,
		CrossRepository: n.IsCrossRepository,
		MergeCommit:     MergeMessage{Headline: n.MergeHeadline, Body: n.MergeBody},
		SquashCommit:    MergeMessage{Headline: n.SquashHeadline, Body: n.SquashBody},
		Viewer: ViewerActions{
			CanUpdate:       n.ViewerCanUpdate,
			CanClose:        n.ViewerCanClose,
			CanReopen:       n.ViewerCanReopen,
			CanAssign:       n.ViewerCanAssign,
			CanMergeAsAdmin: n.ViewerCanMergeAsAdmin,
			CanReact:        n.ViewerCanReact,
		},
		MoreComments: max(0, n.Comments.TotalCount-len(n.Comments.Nodes)),
		MoreThreads:  max(0, n.ReviewThreads.TotalCount-len(n.ReviewThreads.Nodes)),
		MoreCommits:  max(0, n.Commits.TotalCount-len(n.Commits.Nodes)),
		MoreEvents:   max(0, n.TimelineItems.FilteredCount-len(n.TimelineItems.Nodes)),
		Commits:      commits(resp),
	}

	for _, l := range n.Labels.Nodes {
		detail.Labels = append(detail.Labels, Label{ID: l.ID, Name: l.Name})
	}
	for _, a := range n.Assignees.Nodes {
		detail.Assignees = append(detail.Assignees, Actor{ID: a.ID, Login: a.Login})
	}
	detail.Reviewers = reviewers(resp)

	for _, t := range n.ReviewThreads.Nodes {
		thread := ReviewThread{
			ID:           t.ID,
			Path:         t.Path,
			Line:         cmp.Or(t.Line, t.OriginalLine),
			StartLine:    cmp.Or(t.StartLine, t.OriginalStartLine),
			Side:         DiffSide(cmp.Or(t.DiffSide, string(SideRight))),
			IsResolved:   t.IsResolved,
			IsOutdated:   t.IsOutdated,
			CanReply:     t.ViewerCanReply,
			CanResolve:   t.ViewerCanResolve,
			CanUnresolve: t.ViewerCanUnresolve,
		}
		for _, c := range t.Comments.Nodes {
			if thread.ReviewID == "" && c.PullRequestReview != nil {
				thread.ReviewID = c.PullRequestReview.ID
			}
			if thread.Hunk == nil && c.DiffHunk != "" {
				if parsed := hunks(c.DiffHunk); len(parsed) > 0 {
					thread.Hunk = &parsed[0]
				}
			}
			thread.Comments = append(thread.Comments, c.comment(CommentThread, c.CreatedAt))
		}
		detail.Threads = append(detail.Threads, thread)
	}

	switch ref := n.BaseRef; {
	case headless, ref == nil, ref.Compare == nil:
		detail.BehindBy = BehindNoHead
	default:
		detail.BehindBy = ref.Compare.BehindBy
	}

	if ref := n.HeadRef; ref != nil {
		detail.HeadRefID = ref.ID
	}

	detail.Timeline = timeline(resp, detail.Commits)
	RecountThreads(&detail)
	detail.Rollup = rollup(resp.Node.StatusCheckRollup)
	detail.Checks = detail.Rollup.State

	return DetailResult{
		Detail: detail,
		RateLimit: RateLimit{
			Limit:     resp.RateLimit.Limit,
			Cost:      resp.RateLimit.Cost,
			Remaining: resp.RateLimit.Remaining,
			ResetAt:   resp.RateLimit.ResetAt,
		},
	}, nil
}

// RecountThreads rewrites d's reviewer Threads and Unresolved in place. Call it after any change to d.Threads.
func RecountThreads(d *PullRequestDetail) {
	byReview := make(map[string]string, len(d.Timeline))
	for _, item := range d.Timeline {
		if item.Kind == TimelineReview {
			byReview[item.Said().ID] = item.Actor.Login
		}
	}

	at := make(map[string]int, len(d.Reviewers))
	for i := range d.Reviewers {
		d.Reviewers[i].Threads, d.Reviewers[i].Unresolved = 0, 0
		at[d.Reviewers[i].Actor.Login] = i
	}

	for _, t := range d.Threads {
		i, seen := at[byReview[t.ReviewID]]
		if !seen {
			continue
		}
		d.Reviewers[i].Threads++
		if !t.IsResolved {
			d.Reviewers[i].Unresolved++
		}
	}
}

// A submitted review drops its author from reviewRequests, so the panel needs reviews as well.
func reviewers(n pullRequestResponse) []Reviewer {
	var out []Reviewer
	at := make(map[string]int)

	for _, r := range n.Node.Reviews.Nodes {
		if ReviewState(r.State) == ReviewStatePending {
			continue
		}
		login := login(r.Author).Login
		if login == "" {
			continue
		}

		if i, seen := at[login]; seen {
			out[i].State = ReviewState(r.State)
			continue
		}
		at[login] = len(out)
		out = append(out, Reviewer{Actor: Actor{Login: login}, State: ReviewState(r.State)})
	}

	for _, r := range n.Node.ReviewRequests.Nodes {
		name := subject(r.RequestedReviewer)
		if name == "" {
			continue
		}

		if i, seen := at[name]; seen {
			out[i].Requested = true
			continue
		}

		at[name] = len(out)
		team := r.RequestedReviewer.Login == ""
		out = append(out, Reviewer{Actor: Actor{Login: name}, Requested: true, Team: team})
	}
	return out
}

func teamHandle(org, slug string) string {
	if slug == "" {
		return ""
	}
	if org == "" {
		return slug
	}
	return org + "/" + slug
}

func commits(n pullRequestResponse) []Commit {
	out := make([]Commit, 0, len(n.Node.Commits.Nodes))
	for _, node := range n.Node.Commits.Nodes {
		c := node.Commit
		commit := Commit{
			SHA:         c.OID,
			Short:       c.AbbreviatedOID,
			Headline:    c.MessageHeadline,
			Body:        c.MessageBody,
			CommittedAt: c.CommittedDate,
		}
		if c.Author != nil {
			commit.Author, commit.AuthorName = login(c.Author.User), c.Author.Name
		}
		if r := c.StatusCheckRollup; r != nil {
			commit.Checks = CheckState(r.State)
		}
		out = append(out, commit)
	}
	return out
}

// Commits sort by committedDate because the API nulls pushedDate on all but the newest.
func timeline(n pullRequestResponse, made []Commit) []TimelineItem {
	items := make([]TimelineItem, 0,
		len(n.Node.Comments.Nodes)+len(n.Node.Reviews.Nodes)+
			len(made)+len(n.Node.TimelineItems.Nodes))

	for _, c := range made {
		items = append(items, TimelineItem{
			Kind:      TimelineCommit,
			Actor:     c.Author,
			CreatedAt: c.CommittedAt,
			Commit:    &c,
		})
	}

	for _, c := range n.Node.Comments.Nodes {
		comment := c.comment(CommentIssue, c.CreatedAt)
		items = append(items, TimelineItem{
			Kind:      TimelineComment,
			Actor:     comment.Author,
			CreatedAt: comment.CreatedAt,
			Comment:   &comment,
		})
	}

	for _, r := range n.Node.Reviews.Nodes {
		if ReviewState(r.State) == ReviewStatePending {
			continue
		}
		comment := r.comment(CommentReview, r.SubmittedAt)
		items = append(items, TimelineItem{
			Kind:      TimelineReview,
			Actor:     comment.Author,
			CreatedAt: comment.CreatedAt,
			Comment:   &comment,
			Review:    ReviewState(r.State),
		})
	}

	for _, e := range n.Node.TimelineItems.Nodes {
		kind, ok := eventKinds[e.Typename]
		if !ok {
			continue
		}
		item := TimelineItem{Kind: kind, Actor: login(e.Actor), CreatedAt: e.CreatedAt}

		switch kind {
		case TimelineLabeled, TimelineUnlabeled:
			if e.Label == nil {
				continue
			}
			item.Subject = e.Label.Name
		case TimelineAssigned, TimelineUnassigned:
			if item.Subject = subject(e.Assignee); item.Subject == "" {
				continue
			}
		case TimelineReviewRequested, TimelineReviewCancelled:
			if item.Subject = subject(e.RequestedReviewer); item.Subject == "" {
				continue
			}
		case TimelineBaseChanged:
			item.Subject, item.Was = e.CurrentRefName, e.PreviousRefName
		}
		items = append(items, item)
	}

	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items
}

var eventKinds = map[string]TimelineKind{
	"MergedEvent":               TimelineMerged,
	"ClosedEvent":               TimelineClosed,
	"ReopenedEvent":             TimelineReopened,
	"ReadyForReviewEvent":       TimelineReadyForReview,
	"ConvertToDraftEvent":       TimelineDraft,
	"HeadRefForcePushedEvent":   TimelineForcePushed,
	"LabeledEvent":              TimelineLabeled,
	"UnlabeledEvent":            TimelineUnlabeled,
	"AssignedEvent":             TimelineAssigned,
	"UnassignedEvent":           TimelineUnassigned,
	"ReviewRequestedEvent":      TimelineReviewRequested,
	"ReviewRequestRemovedEvent": TimelineReviewCancelled,
	"BaseRefChangedEvent":       TimelineBaseChanged,
}

// mergeable is the field that knows about conflicts; mergeStateStatus reports only the topmost other reason.
func mergeState(mergeable, status string) MergeState {
	if mergeable == "CONFLICTING" {
		return MergeConflicting
	}
	switch MergeState(status) {
	case MergeClean, MergeBlocked, MergeBehind, MergeUnstable, MergeDraft, MergeConflicting:
		return MergeState(status)
	case MergeHasHooks:
		return MergeClean
	}
	return MergeUnknown
}

func login(a actorNode) Actor {
	if a == nil {
		return Actor{}
	}
	return Actor{Login: a.Login}
}
