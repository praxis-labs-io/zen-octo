package gh

import (
	"context"
	"fmt"
)

// mergeMutation merges a pull request.
//
// expectedHeadOid is what makes it safe to send. GitHub refuses the merge when
// the branch has moved since the caller fetched it, so a push that landed while
// somebody was reading the diff comes back as an error rather than as a merge of
// code nobody has seen.
//
// commitHeadline and commitBody go over as null for a rebase, which writes no
// commit of its own and where GitHub ignores them anyway. Every other method
// sends them as they stand: null there would put GitHub's default back over
// whatever the caller had written.
//
// It asks the state back for the reason the lifecycle mutations do: the caller
// is holding an optimistic row and GitHub is the authority on where the pull
// request now sits. It asks for no rateLimit, which is on Query and nowhere
// else.
const mergeMutation = `
mutation MergePR(
  $pullRequestId: ID!,
  $mergeMethod: PullRequestMergeMethod!,
  $expectedHeadOid: GitObjectID,
  $commitHeadline: String,
  $commitBody: String
) {
  mergePullRequest(input: {
    pullRequestId: $pullRequestId,
    mergeMethod: $mergeMethod,
    expectedHeadOid: $expectedHeadOid,
    commitHeadline: $commitHeadline,
    commitBody: $commitBody
  }) {
    pullRequest {
      id
      state
    }
  }
}`

type mergeResponse struct {
	MergePullRequest struct {
		PullRequest *struct {
			ID    string
			State string
		}
	}
}

// Merge merges a pull request and returns where it landed. prID is the pull
// request's node id.
//
// Whether the viewer may merge at all is GitHub's to answer. There is no viewer
// field for it, so a refusal arrives here as an error and the caller's revert
// branch is what says so.
func (c *Client) Merge(ctx context.Context, prID string, opts MergeOptions) (MergeResult, error) {
	vars := map[string]any{
		"pullRequestId":   prID,
		"mergeMethod":     string(opts.Method),
		"expectedHeadOid": nil,
		"commitHeadline":  nil,
		"commitBody":      nil,
	}
	if opts.ExpectedHeadOid != "" {
		vars["expectedHeadOid"] = opts.ExpectedHeadOid
	}

	// A rebase writes no commit of its own and GitHub ignores both fields
	// there. Every other method sends what the caller was holding, an empty
	// body included: that is a body somebody cleared, and null would quietly
	// put GitHub's own default back under them.
	if opts.Method != MergeMethodRebase {
		vars["commitHeadline"] = opts.Headline
		vars["commitBody"] = opts.Body
	}

	var resp mergeResponse
	if err := c.gql.DoWithContext(ctx, mergeMutation, vars, &resp); err != nil {
		return MergeResult{}, fmt.Errorf("merging: %w", classify(err))
	}

	pr := resp.MergePullRequest.PullRequest
	if pr == nil || pr.ID == "" {
		return MergeResult{}, fmt.Errorf("merging: GitHub returned no pull request")
	}
	return MergeResult{State: PRState(pr.State)}, nil
}

// deleteRefMutation removes a branch. It takes the ref's node id rather than
// its name, which is why the detail query asks for headRef { id }.
const deleteRefMutation = `
mutation DeleteRef($refId: ID!) {
  deleteRef(input: {refId: $refId}) {
    clientMutationId
  }
}`

// DeleteRef deletes a branch by its node id.
//
// It is the second half of a merge and it cannot undo the first. A caller whose
// merge landed and whose delete failed has a merged pull request and a branch
// still standing, and saying so is all there is to do about it.
func (c *Client) DeleteRef(ctx context.Context, refID string) error {
	var resp struct {
		DeleteRef struct{ ClientMutationID string }
	}

	vars := map[string]any{"refId": refID}
	if err := c.gql.DoWithContext(ctx, deleteRefMutation, vars, &resp); err != nil {
		return fmt.Errorf("deleting the branch: %w", classify(err))
	}
	return nil
}
