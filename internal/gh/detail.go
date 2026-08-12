package gh

import (
	"cmp"
	"context"
	"fmt"
	"sort"
	"time"
)

// pullRequestQuery asks by node id rather than owner, name and number. The id
// came from GitHub with the search result, so there is nothing to split and no
// ambiguity about which repository is meant.
//
// The item types are deliberately short. Labeled, assigned, renamed, subscribed
// and the rest are noise in a terminal, and every one is another fragment.
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
      createdAt
      updatedAt
      additions
      deletions
      changedFiles
      headRefName
      baseRefName
      reviewDecision
      mergeable
      mergeStateStatus
      body
      author { login }
      repository { nameWithOwner }

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
        READY_FOR_REVIEW_EVENT, CONVERT_TO_DRAFT_EVENT, HEAD_REF_FORCE_PUSHED_EVENT
      ]) {
        nodes {
          __typename
          ... on MergedEvent { createdAt actor { login } }
          ... on ClosedEvent { createdAt actor { login } }
          ... on ReopenedEvent { createdAt actor { login } }
          ... on ReadyForReviewEvent { createdAt actor { login } }
          ... on ConvertToDraftEvent { createdAt actor { login } }
          ... on HeadRefForcePushedEvent { createdAt actor { login } }
        }
      }

      statusCheckRollup: commits(last: 1) {
        nodes {
          commit {
            statusCheckRollup {
              state
              contexts(first: 100) {
                nodes {
                  __typename
                  ... on CheckRun {
                    name
                    status
                    conclusion
                    startedAt
                    checkSuite { workflowRun { workflow { name } } }
                  }
                  ... on StatusContext { context state }
                }
              }
            }
          }
        }
      }
    }
  }
}`

// actorNode is GitHub's nullable actor. It is a pointer everywhere it appears,
// because a deleted account comes back as null rather than a blank login.
type actorNode *struct{ Login string }

// commentNode is what an issue comment, a review comment and a review body all
// answer with. It is embedded rather than repeated three times: encoding/json
// promotes the fields of an embedded struct, so the shape on the wire is flat,
// and one mapper then guarantees the three arrive as the same domain type.
//
// The timestamp is not in here. A comment carries createdAt and a review
// carries submittedAt, and the review's is the one the conversation sorts by.
type commentNode struct {
	ID              string
	Author          actorNode
	Body            string
	ViewerDidAuthor bool
	ViewerCanUpdate bool
	ViewerCanDelete bool
	ViewerCanReact  bool
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
		ID              string
		Number          int
		Title           string
		URL             string
		IsDraft         bool
		State           string
		ViewerCanUpdate bool
		ViewerCanClose  bool
		ViewerCanReopen bool
		ViewerCanAssign bool
		CreatedAt       time.Time
		UpdatedAt       time.Time
		Additions       int
		Deletions       int
		ChangedFiles    int
		HeadRefName     string
		BaseRefName     string

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
				RequestedReviewer *struct {
					Login string
					Slug  string

					Organization struct{ Login string }
				}
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
			Nodes []struct {
				Typename  string `json:"__typename"`
				CreatedAt time.Time
				Actor     actorNode
			}
		}

		StatusCheckRollup struct {
			Nodes []struct {
				Commit struct {
					StatusCheckRollup *struct {
						State    string
						Contexts struct {
							Nodes []struct {
								Typename   string `json:"__typename"`
								Name       string
								Context    string
								Status     string
								Conclusion string
								State      string
								StartedAt  time.Time

								CheckSuite struct {
									WorkflowRun *struct {
										Workflow struct{ Name string }
									}
								}
							}
						}
					}
				}
			}
		}
	}
}

// PullRequest fetches everything the detail screen shows. It is the most
// expensive call in the app, which is why the store caches what it returns.
// headRef is the branch the pull request is merging from. The query needs it up
// front to ask how far behind the base it has fallen, and GraphQL cannot read
// it off a sibling field, so the caller passes the one it already has.
func (c *Client) PullRequest(ctx context.Context, id, headRef string) (DetailResult, error) {
	var resp pullRequestResponse
	vars := map[string]any{"id": id, "head": headRef}

	if err := c.gql.DoWithContext(ctx, pullRequestQuery, vars, &resp); err != nil {
		return DetailResult{}, fmt.Errorf("fetching pull request (%s): %w", id, classify(err))
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
		Body:  n.Body,
		Merge: mergeState(n.Mergeable, n.MergeStateStatus),
		Viewer: ViewerActions{
			CanUpdate: n.ViewerCanUpdate,
			CanClose:  n.ViewerCanClose,
			CanReopen: n.ViewerCanReopen,
			CanAssign: n.ViewerCanAssign,
		},
		MoreComments: max(0, n.Comments.TotalCount-len(n.Comments.Nodes)),
		MoreThreads:  max(0, n.ReviewThreads.TotalCount-len(n.ReviewThreads.Nodes)),
		MoreCommits:  max(0, n.Commits.TotalCount-len(n.Commits.Nodes)),
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
		// GitHub nulls line and startLine once a thread goes outdated, so the
		// original pair is what is left to anchor it by.
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
			// Every comment carries the same hunk. The first one is the one the
			// thread was opened against, which is the context worth showing.
			if thread.Hunk == nil && c.DiffHunk != "" {
				if parsed := hunks(c.DiffHunk); len(parsed) > 0 {
					thread.Hunk = &parsed[0]
				}
			}
			thread.Comments = append(thread.Comments, c.comment(CommentThread, c.CreatedAt))
		}
		detail.Threads = append(detail.Threads, thread)
	}

	if ref := n.BaseRef; ref != nil && ref.Compare != nil {
		detail.BehindBy = ref.Compare.BehindBy
	}

	detail.Timeline = timeline(resp, detail.Commits)
	detail.Rollup = rollup(resp)
	// The embedded row's Checks is the same rollup the search result carries, so
	// a screen reading either one sees the same answer.
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

// reviewers is everyone GitHub lists on the reviewers panel. A submitted review
// takes its author off reviewRequests, so building the list from requests alone
// loses whoever has already looked at it, which is most of the point.
func reviewers(n pullRequestResponse) []Reviewer {
	var out []Reviewer
	at := make(map[string]int)
	byReview := make(map[string]string) // review id -> login

	for _, r := range n.Node.Reviews.Nodes {
		// A pending review is the viewer's own unsubmitted draft, and its state
		// is not a verdict anyone else can see.
		if ReviewState(r.State) == ReviewStatePending {
			continue
		}
		login := login(r.Author).Login
		if login == "" {
			continue
		}

		byReview[r.ID] = login

		// Someone can review more than once, and the last word is the one that
		// counts.
		if i, seen := at[login]; seen {
			out[i].State = ReviewState(r.State)
			continue
		}
		at[login] = len(out)
		out = append(out, Reviewer{Actor: Actor{Login: login}, State: ReviewState(r.State)})
	}

	// A reviewer who only commented is still waiting on something if any of
	// their threads are open, which is the difference between "had a look" and
	// "asked for a change".
	//
	// Every thread is counted, not only the open ones. A reviewer with all of
	// theirs resolved and one who opened none both leave Unresolved at zero, and
	// they are opposite answers: the first has had every point met, the second
	// has had nothing done about the changes they asked for in prose.
	for _, t := range n.Node.ReviewThreads.Nodes {
		if len(t.Comments.Nodes) == 0 {
			continue
		}
		review := t.Comments.Nodes[0].PullRequestReview
		if review == nil {
			continue
		}
		i, seen := at[byReview[review.ID]]
		if !seen {
			continue
		}

		out[i].Threads++
		if !t.IsResolved {
			out[i].Unresolved++
		}
	}

	for _, r := range n.Node.ReviewRequests.Nodes {
		// A requested reviewer is a user, a bot or a team, and only one shape is
		// filled in. Teams have no login; Copilot is a bot, and leaving its
		// fragment out drops it from the list entirely.
		if r.RequestedReviewer == nil {
			continue
		}
		login := r.RequestedReviewer.Login
		name := cmp.Or(login, teamHandle(r.RequestedReviewer.Organization.Login, r.RequestedReviewer.Slug))
		if name == "" {
			continue
		}

		// Somebody already on the list from a review they submitted, whose
		// review has since been asked for again. They keep the verdict they
		// gave and gain the open request, because they genuinely have both.
		// Skipping them here, which is what a plain dedupe does, loses the only
		// evidence that anyone is still waiting on them.
		if i, seen := at[name]; seen {
			out[i].Requested = true
			continue
		}

		at[name] = len(out)
		// A team is what is left when no login came back. The handle under it is
		// built here rather than sent by GitHub, so nothing may write it back
		// where a login goes.
		out = append(out, Reviewer{Actor: Actor{Login: name}, Requested: true, Team: login == ""})
	}
	return out
}

// teamHandle is how a team is written where a login goes. A team's display name
// is not a handle: it carries spaces and case, it is not unique, and it can
// collide with somebody's login. The slug under its organization is.
func teamHandle(org, slug string) string {
	if slug == "" {
		return ""
	}
	if org == "" {
		return slug
	}
	return org + "/" + slug
}

// commits is the branch's last hundred, oldest first, which is the order the
// connection returns them in and the order the tab reads them. The query asks
// from the newest end: a long branch is read from its head, and the commits
// that fall off are the ones already merged into it.
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
		// A commit written from an email GitHub cannot match has no account
		// behind it, and the name git recorded is then all there is.
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

// timeline folds comments, reviews, commits and events into one list in the
// order they happened. GitHub returns them in four connections; the
// conversation reads top to bottom.
//
// Commits are placed by committedDate. GitHub's own timeline uses the push,
// and pushedDate comes back null from the API for all but the newest commits,
// so a rebased branch sorts its commits to when they were written rather than
// when they arrived.
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
		// A pending review is the viewer's own unsubmitted draft. It has no
		// timestamp and nobody else can see it.
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
		items = append(items, TimelineItem{
			Kind:      kind,
			Actor:     login(e.Actor),
			CreatedAt: e.CreatedAt,
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items
}

var eventKinds = map[string]TimelineKind{
	"MergedEvent":             TimelineMerged,
	"ClosedEvent":             TimelineClosed,
	"ReopenedEvent":           TimelineReopened,
	"ReadyForReviewEvent":     TimelineReadyForReview,
	"ConvertToDraftEvent":     TimelineDraft,
	"HeadRefForcePushedEvent": TimelineForcePushed,
}

// rollup counts the head commit's checks. GitHub gives the summary state; the
// breakdown behind it is what makes the rail worth reading.
func rollup(n pullRequestResponse) CheckRollup {
	commits := n.Node.StatusCheckRollup.Nodes
	if len(commits) == 0 || commits[0].Commit.StatusCheckRollup == nil {
		return CheckRollup{}
	}

	src := commits[0].Commit.StatusCheckRollup
	out := CheckRollup{State: CheckState(src.State)}

	// A re-run leaves the previous attempt in the connection, so the same job
	// arrives twice with two different answers. Only the latest one is true.
	at := make(map[string]int, len(src.Contexts.Nodes))
	started := make([]time.Time, 0, len(src.Contexts.Nodes))

	for _, c := range src.Contexts.Nodes {
		check := Check{
			Name:  cmp.Or(c.Name, c.Context),
			State: checkState(c.Typename, c.Status, c.Conclusion, c.State),
		}
		// A job is named for what it does, so half a repository's checks are
		// called "test". The workflow it ran under is what tells them apart.
		if run := c.CheckSuite.WorkflowRun; run != nil {
			check.Workflow = run.Workflow.Name
		}

		key := check.Key()
		i, seen := at[key]
		switch {
		case !seen:
			at[key] = len(out.Checks)
			out.Checks = append(out.Checks, check)
			started = append(started, c.StartedAt)
		case c.StartedAt.After(started[i]):
			out.Checks[i], started[i] = check, c.StartedAt
		}
	}

	for _, check := range out.Checks {
		switch check.State {
		case CheckStateSuccess:
			out.Passed++
		case CheckStatePending, CheckStateExpected:
			out.Pending++
		case CheckStateSkipped:
			out.Skipped++
		default:
			out.Failed++
		}
	}
	return out
}

// mergeState folds the two fields GitHub answers with. mergeable is the one
// that knows about conflicts; mergeStateStatus knows everything else, and
// reports only the topmost reason a merge is held up.
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

// checkState folds a check run and a status context into the one vocabulary.
// They are different types on the wire carrying the same news: a check run has
// a status and a conclusion, a status context only a state.
func checkState(typename, status, conclusion, state string) CheckState {
	if typename != "CheckRun" {
		switch state {
		case "SUCCESS":
			return CheckStateSuccess
		case "PENDING":
			return CheckStatePending
		case "EXPECTED":
			return CheckStateExpected
		}
		return CheckStateFailure
	}

	if status != "COMPLETED" {
		return CheckStatePending
	}
	switch conclusion {
	case "SUCCESS", "NEUTRAL":
		return CheckStateSuccess
	case "SKIPPED":
		return CheckStateSkipped
	}
	return CheckStateFailure
}

func login(a actorNode) Actor {
	if a == nil {
		return Actor{}
	}
	return Actor{Login: a.Login}
}
