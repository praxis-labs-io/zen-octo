package gh

import (
	"context"
	"fmt"
)

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

type reactionGroup struct {
	Content          ReactionContent
	ViewerHasReacted bool
	Reactors         struct{ TotalCount int }
}

// GitHub answers with all eight groups on every subject, nearly all empty; the zeroes go and its order stays.
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

// Pointers, because an empty set is a write that worked and only a nil payload is not.
type reactionResponse struct {
	AddReaction    *struct{ ReactionGroups []reactionGroup }
	RemoveReaction *struct{ ReactionGroups []reactionGroup }
}

// SetReaction adds content to the subject with node id subjectID, or removes it when on is false, and returns
// the subject's reactions as GitHub recorded them.
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
