# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repo is

Drew's Go terminal client for GitHub, at `praxis-labs-io/zen-octo` (`origin`). It handles a pull request end to end without opening a browser: read it, discuss it, watch its CI, fix its metadata, merge it. Issues get the same treatment where it makes sense.

`docs/` holds everything a user reads: the guide, the keymap, configuration and
install. `docs/CONTRIBUTING.md` holds the checks, the boundaries and the test
conventions, and `README.md` is the front page linking in. Read those rather
than restating them here. `docs/CONTRIBUTING.md` also maps each changed surface
to the document describing it.

gh-dash is the reference for GitHub search-query shapes and section config, not for code. It runs on Bubble Tea v1, so its view code does not lift verbatim.

**`main` is the product branch.** Feature work flows ticket → branch → PR on `origin` (see Project Management).

Two things skip the PR and commit straight to `main`:

- Genuinely trivial tweaks. A typo, a one-liner.
- **Doc-only changes with no code.** Markdown, comments, `CLAUDE.md`, rules files. A PR for prose is ceremony.

A tracked pre-push hook rejects pushes to `main`, so an agent commits these and Drew pushes them. Don't reach for `--no-verify`.

The installed binary is built from here to `~/.local/bin/zen-octo`; **rebuild after changes or Drew keeps running the old code**:

```sh
make install
```

**It doesn't reach a session already open.** Restart the app before believing a rendering bug.

The module path is `github.com/praxis-labs-io/zen-octo`. A `v*` tag cuts a release: `.github/workflows/release.yml` builds the six targets, writes the checksums, and cuts it from `docs/release-notes/<tag>.md`, which has to be on `main` before the tag is. There is no Homebrew tap.

Anything published under Drew's name (PR bodies, issues, README) must be shown to him word-for-word before pushing. His voice: terse, considerate, stoic, no strong adverbs, no em-dashes.

## Conventions

@.claude/rules/code-quality.md

That file holds only the Go and Bubble Tea specifics. The principles and voice rules are global and load automatically; don't copy them in here, that only creates drift.

## Commands

The checks, the lint version pin and the hook setup are in `docs/CONTRIBUTING.md`. `make all` is the gate. The SessionStart hook wires up `.githooks` on every session, so a fresh clone is covered.

## Charm module paths

The Charm v2 line lives under `charm.land/*`, not `github.com/charmbracelet/*`. `github.com/charmbracelet/bubbletea/v2` does not resolve. Version numbers are the same across both paths.

```
charm.land/bubbletea/v2
charm.land/lipgloss/v2
charm.land/bubbles/v2
charm.land/glamour/v2
```

`github.com/charmbracelet/fang` (v1 line) keeps its github path and pulls an older beta of `charm.land/lipgloss/v2`. Requiring v2.0.5 directly upgrades past it; there is no two-lipgloss problem as long as nothing imports the github v2 path.

## Project Management

Work is tracked in Linear: Praxis Labs workspace, **Zen Octo** team (key `ZNO`, tickets `ZNO-###`), reached through the `linear-zen-octo` MCP server declared in `.mcp.json`. Address projects and statuses **by name, never a UUID**; ids don't survive workspace moves.

The bucket names are shared with other teams, so `save_issue` resolving a bare project name can land on another team's copy and fail the call. Pass the Zen Octo project id in that one argument when it does.

### Projects

Five long-running buckets. They never complete; every ticket belongs to exactly one. Each bucket's Linear description holds a `File here when:` test and a routing list, and those descriptions are the tiebreaker when a ticket could fit two:

- **Polish & Bugs**: bugs and rough edges in surfaces that already ship. The dogfood inbox.
- **Feature Backlog**: net-new capabilities. Ideas live here until promoted.
- **Performance and Code-Quality**: improves the code, no user-visible change.
- **Website**: the public site, its copy, its SEO.
- **Release & Distribution**: how the binary gets from `main` to a user and stays current.

A body of work big enough to need milestones gets its own finite epic project, named for what it delivers, completed and closed when it ships. An epic is a Linear Project, never a tracking issue. When an epic closes, follow-ups move to the matching bucket.

### Tickets

- Every ticket gets the team, exactly one project, a priority, and a status. No orphans.
- Create tickets as we go; never dump a full backlog up front.
- PR-sized scoping: 1 ticket = 1 branch = 1 PR as the rule of thumb.
- Keep descriptions lean: clear title, short goal and scope. No boilerplate acceptance criteria.
- Use Linear's generated branch name (`gitBranchName` from the MCP), never an invented one.
- Reference the ticket id in commits and the PR title/body so Linear auto-links.
- Status ladder: agent drives Backlog → Todo → In Progress. The GitHub integration owns In Review and Done; never write those by hand.

### Shipping

Feature-complete work ships via the global `ship-feature` skill: `make all` green, push, draft PR, Copilot + `/code-review`, triage with no tech debt, push then mark ready as separate actions. Manual invocation only.

**This repo carries no copy of it.** A copy drifts, and nothing about a stale one reads as stale. A session that can't see the global skill should say so rather than follow a copy.

### Specs and plans

Scratch, never committed. `docs/` describes only what is true today. Durable context lives in Linear project descriptions and tickets.

## Architecture

`cmd/zen-octo` is the entrypoint (fang over cobra). Everything else lives in `internal/`. The package boundaries are in `docs/CONTRIBUTING.md` and the conventions file above. This section holds what neither covers and a contributor can break without noticing.

- **`View` runs on value receivers all the way down.** Anything written while drawing lands on a copy and is dropped, so size widgets when they open and on resize, never in a render. A store write made on a model nobody keeps is the same bug: a map survives the copy and an int doesn't, so a `nextSeq` bumped on a copy hands the next write a number already taken. `ring.reset` lets its slice go rather than reusing it, or two copies write into one backing array.
- **There is one poll chain.** `armPoll` is called from `Init` and from the tick's own handler and nowhere else. A third call site runs the beat at double the rate.
- **A background fetch fails quietly.** `PollFailed` and `PulseFailed` keep what's on the screen and never toast. `Failed` replaces a section's rows with its error, so it's only for a fetch the reader asked for.
- **A refetch isn't evidence a write failed.** Writes in flight sit beside the fetched detail in `internal/store` and fold in on the way out of `Detail`, never into it. Nothing is evicted while a fetch or a write is out for it, and an eviction takes the staleness maps keyed to that entry with it.
- **Reverting alone is right only for an all-or-nothing write.** A write made of two calls can fail half applied, and a refused merge usually means the screen is stale. Both go through `EditRevertedStale` and refetch.
- **The theme is built once, before Bubble Tea takes the tty.** `theme.Query` reads the terminal's colors, and `app.New` builds the theme and passes it down by value. No screen has a `SetTheme`. `internal/tui/theme` is a leaf that knows nothing about YAML, and `app.New` is where config meets it and where colors are validated.
- **The root sets the only cursor.** Every text box is a textarea with its own caret turned off, and the screen holding the keyboard reports where the caret is.
- **The shell draws nothing under 56 by 23.** The height is the merge form at its tallest, 21 rows, plus the status bar and the config notice, and a test opens that form at the floor. Under it `resize` returns early and only `ctrl+c`, `esc` and `q` work.
- **`internal/link` is where a URL leaves the process.** The browser goes through `go-gh`'s resolver with its output discarded, since a launcher printing into the alt screen tears the frame. A copy falls back to OSC52, and that failure isn't reported.

## GitHub API behaviors

Things GitHub does that the code depends on and cannot show.

- **A pending review is the viewer's own and nobody else's, and it has no `submittedAt`.** Sorting it into the timeline by that field puts it above everything, so it is carried on its own rather than folded in with the submitted ones.
- **A pending review's threads come back in `reviewThreads` like any other.** Nothing on the thread says so; the `state` on its comments is the only mark, and a reply written into a pending review sits in a published thread the same way.
- **A file-level thread reports `line: 1`.** `subjectType` is the only field telling it from a thread written against the first line.
- **`addPullRequestReviewThread` with no review opens one.** GitHub publishes no line comment standing alone, so a single comment is a pending review that still has to be submitted.
- **`deletePullRequestReview` takes its threads with it**, and `reviewThreads.totalCount` lags `nodes` for a moment after.
- **`startLine` equals `line` on a single-line thread.** GitHub nulls it only when the thread is outdated, so it never marks a range on its own.
- **An outdated review thread has no `line` or `startLine`.** GitHub nulls both once the code under it moves, so the thread is anchored by `originalLine` and `originalStartLine` instead.
- **A nil slice goes over the wire as `null`, and `[ID!]!` rejects it.** An empty label or assignee set has to be sent as `[]`.
- **A null merge headline or body puts GitHub's default text back.** Only a rebase sends null, since it writes no commit of its own. Every other method sends what the form holds, an empty body included.
- **A third-party check run has a database id and no Actions job.** The jobs endpoint answers 404 for it every time.
- **A job's log doesn't exist until the job finishes.** Asking while it runs follows a signed redirect to a blob that isn't there yet and 404s, so a running job is never asked for one.
- **Only the single-job rerun answers with a Date header.** The two bulk reruns report no time, so their marks are stamped with the local clock.
- **`UNSTABLE` means the commit status isn't passing, which includes a check that is still running.** A failing commit status that no check run produced can also sit under a green rollup.
- **A commit from an email GitHub can't match has no account.** The author is then just the name git recorded.
- **`viewerCanDeleteHeadRef` is false on every open pull request.** It turns true once the pull request closes, so it can't gate the merge form's delete. `Ref` has no viewer permission, so the delete's own failure is what reports a refusal.
- **Permission flags answer narrower questions than their names.** `viewerCanUpdate` stays true on a merged pull request that refuses a base change. `viewerCanAssign` is true for a triage collaborator the mutation refuses, so assignees need `viewerCanUpdate` too. `viewerCanDelete` is true on a submitted review, and no call deletes one. Review requests have no flag at all.
- **Copilot has a different name in each call.** REST `POST` takes `copilot-pull-request-reviewer[bot]`, `DELETE` takes `Copilot`, and GraphQL reports `copilot-pull-request-reviewer`. The `POST` response never lists the bot either way, so a request is confirmed through GraphQL `reviewRequests`.
- **Review requests go as additions and removals, never as a set.** A change is two calls and can land half applied. A reviewer drops off the requests once they submit, and a requested team isn't in `assignableUsers`.
- **A merged pull request's detail query answers with the data and an error together.** `compare` is null with a `NOT_FOUND` on `node.baseRef.compare`, and go-gh decodes the payload before it reads the errors. `deletedHeadRef` tolerates that one error and nothing beside it.
- **`refs` ignores `orderBy` on `refs/heads`.** Alphabetical is the only order and it's applied before paging, so a branch outside the page is reached through `refs(query:)`.
- **Mergeability is computed lazily.** A pull request nobody has looked at recently answers `UNKNOWN`, and the query is what starts the computation.
- **A head branch delete races `deleteBranchOnMerge`.** GitHub deletes the branch a moment after the merge, so a call made then fails on a ref that's already gone.
- **Search returns no `mergedAt` or `closedAt`, and its date qualifiers take only an absolute instant.** The limit is applied before the rows come back, so a section without `sort:` gets GitHub's relevance order. Indexing lags up to a minute.
- **Every GraphQL query costs one point.** The node count isn't the price. Payload is the real cost: a detail on a heavily reviewed pull request is megabytes where a pulse is a few hundred bytes.
- **Reactions come back as all eight groups, most at zero.** `internal/gh` drops the empty ones.
- **A timeline event's subject can be null.** The event is dropped rather than rendered as a verb with nothing after it.

## Rendering traps

Each of these looks like working code and produces a broken frame.

- **A pull request's state and its draft flag are two fields, and a closed one carries both.** GitHub never clears the flag, so reading it ahead of the state labels a closed draft "Draft" and marks it as waiting for somebody to pick up. Read the lifecycle first; the flag is what reopening gives back.
- **Every styled cell ends in a full SGR reset**, which clears the background along with the foreground. A row background has to be set per cell; wrapping a joined row paints only the first one.
- **`lipgloss.Canvas.Compose` ignores a layer's position** and draws every layer at the origin. Compositing an overlay needs `lipgloss.NewCompositor`.
- **`Style.Width` wraps before it clips.** Truncating to a column width means clipping explicitly first, or one long title becomes two rows.
- **`viewport.EnsureVisible` is not a scroll-to-cursor.** It acts only once the line is already outside the window, then puts it on the top row. A cursor moving down a row at a time jumps a whole page and then sits still for the rest of it. Move the offset by hand.
- **The shortest scroll onto the screen is the wrong one.** Bringing a block into view by the minimum distance lands it at the foot of the window with everything under it below the fold, and a block taller than the window opens on its last line with its heading above the top. Move it to the top row instead, and leave it where it is when it already fits on screen whole. A fixture whose blocks are all shorter than the window proves none of this.
- **A pane clips overflow silently.** A row wider than the pane loses its trailing columns mid-cell with no ellipsis, and a width test still passes because the pane fills its line. The row has to fit before the pane sees it.
- **Glamour output belongs to the width it was rendered at.** It pads every line out to that width, so the viewport has to be handed exactly the same number or soft wrap puts every line onto two. Caching by body alone repaints the previous width's wrap.
- **A viewport offset is a line, and a row is not.** Once rows are two lines and group headers are one, scroll arithmetic that lands on the row it wants opens the window on that row's second line with its title cut off above. Round the offset up to the next item boundary. A test at an even content height proves nothing: the arithmetic lands on boundaries by accident there.
- **Rounding the offset is not enough if the window is not a whole number of rows.** At the end of a list the viewport clamps to its own last offset, and against an odd height that clamp lands back between two lines. Size the viewport down to a multiple of the row height; the pane pads the spare line back.
- **A key that moves by a page is counting lines, and a cursor is counting rows.** Handing the pane height straight to a two-line-row column moves the cursor twice as far as the window does, so every press skips a screenful that never appears on screen.
- **Soft wrap and a line-number gutter cannot both be on.** One long line of code folds onto a second row, and every line under it is then one further out of step with the number beside it. Turn `SoftWrap` off and clip, and only ever measure a diff at a width where something overflows.
- **A lexer carries state across lines.** Highlighting a diff line by line comes apart on the first multi-line string. Tokenise the whole file, and tokenise the two sides of the diff separately, or the lexer is reading a file that holds both halves of every change.
- **A single newline is a line break in a GitHub comment and a space in CommonMark.** Glamour follows the spec, so two lines somebody typed arrive as one and the comment reads differently here from the browser it was written in. `glamour.WithPreservedNewLines()` is the switch.
- **Soft wrap costs half the price of setting a viewport's content, and the conversation has nothing for it to fold.** Every block is wrapped to `bodyWidth` before the viewport sees it. Leaving it on spends 12.7ms against 7.0ms on a hundred-and-forty-comment thread, which is a per-keystroke bill once a comment is being written into the page.
- **A text box inside a scrolling pane rebuilds the whole page on every keystroke.** Cards are re-bordered one by one, so a long thread costs 27ms a character with the markdown cache hitting perfectly. Nothing around the box can change while it has the keyboard, so build the head and the tail once and join a fresh box between them.
- **The page splits at the outermost block holding the box, not at the block that holds it.** A review renders its own card and every thread it opened as one string with a branch gutter down the side, so cutting between two of them means splicing `├─`, `│ ` and `╰─` back together at the right variant. Cut either side of the whole review instead.
- **Scrolling the shortest distance is right wherever a box is involved.** Typing isn't taking the reader anywhere, and a box sits under what it answers, so moving it to the top row scrolls the thread away. Follow the row under the caret, which keeps the send button on the screen, and never look up the block holding the box: a reply box and an edit box aren't ring stops. The box is capped to the pane at its render site, since a thread's opening comment pays for two cards and a reply pays for one.
- **A text input sized during a render is sized on a copy.** `View` is reached through value receivers all the way down, so a `SetWidth` there is thrown away with the copy, and the real widget keeps a width of zero: it renders from the first character, never scrolls, and every keystroke past the visible edge is invisible while the caret sits off the box. Size the fields when the thing opens and when the screen resizes, never while drawing.
- **A text input recomputes its visible window only when the caret leaves it.** Writing a longer value and then putting the caret inside the window the old one had leaves that window exactly where it was, so the box goes on showing as many characters as the short value did. Ending first and coming back is what forces the recompute. A fixture whose two values are the same length proves none of this.
- **A box that has just opened is a journey, and the caret is not where it ends.** The caret opens on the box's first row, so a scroll that follows it leaves the rest of the writing area, the button and the border below the fold, and the reader is writing into something with no visible end. Opening lands the foot; typing follows the caret. `showOpenedBox` beside `showCaret` is that split, and neither ever scrolls past the caret's own row, because a box taller than the window can only show one end.
- **A write that changes a card's height has to put it back on the screen, and only where it was whole to begin with.** Unresolving opens a collapsed thread into its card, its code and every reply hanging off it, and that growth arrives through the store rather than under the key: `space` re-shows the focus itself and `x` has no equivalent, so the thread grew off the bottom and sat there. `SetDetail` asks before and after. Asking only after would haul a reader who had scrolled somewhere else back to the focus they left.
- **A block that answers the line above it cannot go to the top row either.** The rule holds past the compose box. A review thread in the diff hangs under the code it was written against, so a jump that tops the card scrolls that code away and lands the reader on a comment about something they cannot see. Open a few lines above it instead, and never above the file's own heading, which reads as the wrong file until the eye finds the border.
- **A caret's column is two different numbers.** `Column()` counts runes into the logical line and `LineInfo().CharOffset` counts cells across the screen. Detection wants the first and placement wants the second, and swapping them is invisible until a comment holds an emoji or a line of CJK, at which point anything drawn at the caret sits somewhere else entirely.
- **A block's own indent is not the indent it was drawn at.** `boxAt` is a line relative to its block and `boxCol` has to be a column relative to it, threaded through the same sites and gaining `treeGutter` at every rail it hangs off. A column measured once on the compose card is two cells wrong for a reply and four or six for a reply under a review's thread, and the page body carries a centring gutter on top of that which is tens of columns wide on a wide terminal.
- **An overlay anchored at the caret drifts if it is anchored at the caret.** A popup answering a word wants the word's first cell, not the caret's, or it steps right once per keystroke while the reader is reading it. Anchor on what the list is about and let the caret run.
- **`comp.Over` centres, and centring is not a special case of placing.** A positioned overlay has to clip to the frame before it clamps, or one wider than the pane is measured at its uncut width and pushed off the right edge; and it owes the same trailing-space re-pad, because the compositor trims every line and the pinned header's lines end in padding rather than in a border rune. Clamp to the pane rather than the frame, or a popup hangs over the rail beside it.
- **`lipgloss.Color` reads any integer.** Past 255 it packs the value as RGB, so `"256"` is a near-black, and a negative is silently made positive. Range-check a bare number first.
- **`RGBA()` on a nil color panics.** Painting reads nil as no background, but code weaving a fill in by hand has to check first.
- **`NoColor{}` writes no escape and its `RGBA()` is black.** Spelled out for glamour it renders every paragraph black, so it gets no spelling at all.
- **`textinput.Cursor()` ignores horizontal scroll.** Past the edge of the box it reports a clamped cell while the caret walks away from it, which is why every box here is a textarea.
- **Lipgloss writes an underline as one SGR run per rune.** Nothing can look for a single escape in front of a whole label.
