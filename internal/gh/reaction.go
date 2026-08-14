package gh

import (
	"context"
	"fmt"
)

// addReactionMutation and removeReactionMutation give a reaction and take it
// back. Both are addressed to a subject id and nothing else: unlike an edit, a
// node id is the whole address here, because one pair of calls covers an issue
// comment, a review, a review comment and the pull request itself.
//
// Both select the payload's own reactionGroups, which is the subject's whole
// new set. Nothing on this side has to work out what the count became.
//
// Neither selects rateLimit, for the reason addComment gives: that field is on
// Query alone and a mutation naming it is rejected whole.
const addReactionMutation = `
mutation AddReaction($subjectId: ID!, $content: ReactionContent!) {
  addReaction(input: {subjectId: $subjectId, content: $content}) {
    reactionGroups { content viewerHasReacted reactors { totalCount } }
  }
}`

const removeReactionMutation = `
mutation RemoveReaction($subjectId: ID!, $content: ReactionContent!) {
  removeReaction(input: {subjectId: $subjectId, content: $content}) {
    reactionGroups { content viewerHasReacted reactors { totalCount } }
  }
}`

// reactionGroup is one of the eight groups GitHub answers with, on every
// subject and on both mutation payloads.
type reactionGroup struct {
	Content          ReactionContent
	ViewerHasReacted bool
	Reactors         struct{ TotalCount int }
}

// reactions is the groups somebody actually gave, in the order they arrived.
//
// GitHub answers with all eight whatever the subject, nearly all of them empty,
// so dropping the zeroes is what makes this a list of reactions rather than a
// list of the reactions that exist. The order is GitHub's own and is left
// alone: a card that sorted by count would move its pills every time somebody
// pressed one.
func reactions(groups []reactionGroup) []Reaction {
	var out []Reaction
	for _, g := range groups {
		if g.Reactors.TotalCount == 0 {
			continue
		}
		out = append(out, Reaction{
			Content: g.Content,
			Count:   g.Reactors.TotalCount,
			Viewer:  g.ViewerHasReacted,
		})
	}
	return out
}

// reactionResponse decodes either payload. The method reads the half its own
// document produced rather than whichever field came back filled, so a response
// landing in the wrong one is a failure instead of a silent pass.
//
// Pointers, because the two questions are different and an empty slice answers
// both: taking the last reaction off a subject leaves it with none, and that is
// a write that worked. A nil payload is the one that did not.
type reactionResponse struct {
	AddReaction    *struct{ ReactionGroups []reactionGroup }
	RemoveReaction *struct{ ReactionGroups []reactionGroup }
}

// SetReaction gives a reaction or takes it back, and returns the subject's
// reactions as GitHub recorded them. subjectID is the node id of whatever is
// being reacted to: a comment, a review, or the pull request whose description
// is on screen.
//
// One method over two because this is one key with two directions, which is how
// the card renders it and how the picker offers it. The direction picks the
// document and the words the error uses.
//
// No reactions is not an error. Taking the last one off leaves a subject with
// none, and that is the write having worked; the failure is the payload the
// document did not ask for.
func (c *Client) SetReaction(ctx context.Context, subjectID string,
	content ReactionContent, on bool,
) (ReactionResult, error) {
	doc, doing := removeReactionMutation, "removing a reaction"
	if on {
		doc, doing = addReactionMutation, "adding a reaction"
	}

	var resp reactionResponse
	vars := map[string]any{"subjectId": subjectID, "content": string(content)}

	if err := c.gql.DoWithContext(ctx, doc, vars, &resp); err != nil {
		return ReactionResult{}, fmt.Errorf("%s: %w", doing, classify(err))
	}

	payload := resp.RemoveReaction
	if on {
		payload = resp.AddReaction
	}
	if payload == nil {
		return ReactionResult{}, fmt.Errorf("%s: GitHub answered for the other one", doing)
	}

	return ReactionResult{Reactions: reactions(payload.ReactionGroups)}, nil
}
