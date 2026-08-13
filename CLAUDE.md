# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repo is

Drew's Go terminal client for GitHub, at `zen-octo/zen-octo` (`origin`). It handles a pull request end to end without opening a browser: read it, discuss it, watch its CI, fix its metadata, merge it. Issues get the same treatment where it makes sense.

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

Anything published under Drew's name (PR bodies, issues, README) must be shown to him word-for-word before pushing. His voice: terse, considerate, stoic, no strong adverbs, no em-dashes.

## Conventions

@.claude/rules/code-quality.md

That file holds only the Go and Bubble Tea specifics. The principles and voice rules are global and load automatically; don't copy them in here, that only creates drift.

## Commands

```sh
make all              # lint (gofmt + mod-tidy + golangci-lint) + test + build
make test             # go test -race -coverprofile ./...
make lint             # includes gofmt check and go.mod tidiness
make fmt-fix          # gofmt -w .
make install          # build to ~/.local/bin/zen-octo
go test ./internal/gh/ -run TestName   # single test
```

Run checks directly, never through a pipe that swallows exit codes. `make lint | tail` reports success on failure.

### Lint version pin

CI pins golangci-lint to match the local brew version (`.github/workflows/ci.yml`). Keep the pin current with the local version, or CI and local runs stop agreeing.

### Git hooks

`.githooks/pre-push` is tracked and rejects pushes to `main`. `git config core.hooksPath .githooks` wires it up; the SessionStart hook does this on every session so a fresh clone is covered. Untracked `.git/hooks/` files don't survive a clone, which is why the hook lives here instead.

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

A body of work big enough to need milestones gets its own finite epic project, named for what it delivers, completed and closed when it ships. **v1** is the current epic. An epic is a Linear Project, never a tracking issue. When an epic closes, follow-ups move to the matching bucket.

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

**This repo carries no copy of it.** It did until 2026-08-11, so that a session cloning this repo alone would still have the skill. Manual propagation is what actually happened instead: the copy drifted, shadowed the live one, and spent over a week prescribing a Copilot review request that never worked. A stale copy is worse than an absent skill, because nothing about it reads as stale. A session that cannot see the global skill should say so rather than follow a copy nobody maintains.

### Specs and plans

Scratch, never committed. `docs/` describes only what is true today. Durable context lives in Linear project descriptions and tickets.

## Architecture

`cmd/zen-octo` is the entrypoint (fang over cobra). Everything else lives in `internal/`.

Package boundaries that matter:

- **`internal/gh` is the only package that touches the network.** It returns domain types, never raw API structs, so everything above it is testable against a fake. Two transports, two seams: `graphQLDoer` for everything, `restDoer` for the diff alone, because GraphQL has no field carrying a patch.
- **`internal/store` owns fetched state and refresh timing.** Views read from it, they never fetch.
- **`internal/tui/*` packages never import each other sideways.** Shared widgets live in `internal/tui/comp`.

Writes are optimistic: apply locally, toast, reconcile on response, revert on error. Every write path needs the revert branch, not just the happy one.

The shell divides the frame top-down: `app` owns the terminal size, subtracts the status bar, and calls `SetSize` on the screen that has focus, which sizes its own panes. **No component reads terminal dimensions or counts chrome lines.** A test asserts the rendered frame never exceeds the size it was given.

Every scrollable region owns its own `bubbles/v2/viewport`. Scroll state never sits on the root model.

Key bindings live in `internal/tui/keys`, declared once with their help text. The help view renders from the same declarations, and tests hold that nothing declared goes unlisted and nothing listed is invented.

The braces are paragraph motion, the way they are in vim, and mean the same thing wherever the detail screen holds blocks: go to the next one. What a block is belongs to the tab, so the key walks the cards on the conversation and the files in a diff. They were two separate keys once, for one intention, and whichever the reader pressed was inert on the tab the other worked on. That gives tab and shift+tab back to the tab strip, where they do what they do on the list screen. `keys.Form` is the one place tab means something else, and it is its own map rather than part of `DetailMap`: a compose box or the merge form takes every key until it closes, so the two are never live together, and the braces a reader walks blocks with are text inside a textarea.

The rail is the exception, and the braces are dead on it. Its rows are a list of controls rather than blocks of prose, so it answers to the movement keys the way the file column does: `railDriving` sends `j` and `k` to the cursor, and taking the pane lands the cursor on its first control rather than waiting to be pressed once before it will say where the keys go. Two things fall out of it being a list with facts in it. The cursor hands the key back to the pane once it has nowhere left to go, because the last control has rows under it that are the reason a reader scrolls at all, and `g`, `G` and the page keys never move it, because those go to the ends of a pane and the rail's ends are past its last stop.

The status bar carries the hints on the left in every state. A toast or the refresh spinner lands on the right and wins a narrow line, which is the opposite of the readout that sits there otherwise: a toast may be the only account of a write that failed, and a key works whether or not it is on the line. `RenderMessage` is that priority, beside `Render`. The readout is the remaining budget and nothing else, shown only once it has run low enough to be worth reading. Neither screen names itself there: the list's section is the current tab in the top border and the detail's pull request is in its own header, so both were spending the line on something already on the screen, and spending it on the side a toast lands on.

The State row's menu is built from where the pull request sits and from what
GitHub says the viewer may do to it, never from the word on the row: state and
draft are independent fields, a closed draft reads as "Draft", and the detail
query asks for `viewerCanUpdate`, `viewerCanClose` and `viewerCanReopen` so a
menu item never opens a write that comes back refused. A row with nothing to
offer states a fact and the ring walks past it, the way an empty Checks section
does, but only once the detail has landed: before that nothing is known, which
is not nothing being allowed, and dropping the key early moves every rail stop
by one the moment the answer arrives. A state write refetches the whole detail
once it settles, because the Merge row, the check rollup and the timeline all
hang off a field the store cannot compute. It borrows no refresh leg doing it,
or the sync's summary toast lands behind the one that already said what
happened.

Every control on the details rail answers to one key. Enter opens what the focused row holds, as a centred modal built from `comp.Over` and `comp.Modal`, and `comp.Picker` is the list inside it. The picker declares no bindings of its own: a widget package cannot reach sideways into `keys`, so it exposes verbs and `prview` decides which key means which. While one is up it owns the keyboard, which is what `Capturing` tells the root, and the order in `pickerKey` is load-bearing: the keys that can never be text go first, then the filter claims every printable one, then movement takes what is left. A section is its picker: the rows already in it open the same modal as the add row under them, because the modal is where something comes off as well as goes on.

A tick in the Reviewers picker means a review is requested, never that somebody
is on the pull request. GitHub drops a reviewer from the requests the moment
they submit, and there is no call that un-submits one, so a reviewer who has
answered opens unchecked and ticking them asks again, which is what the
re-request button does in the browser. `Reviewer.Requested` is what says so, and
it is not the inverse of `State`: a re-requested reviewer holds a verdict and an
open request at once, the two connections overlap rather than partition on them,
and reading an empty state as "waiting" hides the request behind a key that can
only ask for it again. That write is the one place two calls are unavoidable:
the endpoint has no spelling meaning "these and nobody else", so the screen
sends a delta and the app cancels before it asks. Because it is a delta, the
picker has to list every outstanding request as well as the repository's own
users: `comp.Picker.Chosen` reports only ids it was given items for, so a
checked login with no item silently becomes a cancellation, and a review
requested of anyone past the hundredth assignable user would be dropped by a
reader who never saw them. A team requested for review is kept and never listed,
because `assignableUsers` holds none and a delta cannot remove what it could not
offer; `Reviewer.Team` is what the exclusion reads, rather than the slash in a
handle this client built itself. It is also the one failure path that refetches:
a write made of two calls can fail with the first applied, and reverting alone
would leave the rail claiming a request GitHub has already dropped.
Copilot answers to a different name in every direction and the two REST verbs
disagree with each other: `POST` takes `copilot-pull-request-reviewer[bot]` and
answers 200 while writing nothing to a bare `Copilot`; `DELETE` takes the bare
`Copilot` and 422s on the `[bot]` form, which it resolves to a Bot and then
rejects for not being a User; GraphQL reports `copilot-pull-request-reviewer`,
and that one is canonical everywhere above `internal/gh`. **The `POST` response
never lists the bot at all**, landed or not, so `requested_reviewers` cannot
tell a success from the silent no-op and reading it rejects every Copilot
request there is. That shipped once. The confirmation is a GraphQL
`reviewRequests` read, which is the only place a bot request is visible, and it
is what makes those two 200s distinguishable. Copilot cannot be discovered
either, so it is offered always. A reviewer write refetches the whole detail, since the endpoint reports
the outstanding requests alone and says nothing about who has already reviewed,
and requesting one rewrites the review decision the header renders. An assignee
write refetches nothing: it changes nothing the store cannot already see. The
Assignees rows are a control only while `viewerCanAssign` **and**
`viewerCanUpdate` both say so, because the permission to assign and the
permission to run the mutation that does it are different answers: a triage
collaborator is given the first and refused the second. There is no flag at all
for review requests, so those rows are ungated and the revert branch answers a
refusal.

The Base row is a control while `viewerCanUpdate` says so **and** the pull
request is not merged, which is two questions rather than one: the flag stays
true on a merged pull request, because its title and body are still editable,
and GitHub refuses the base change anyway. Its branches are the one picker whose
choices are a search rather than a cache. `refs` takes an `orderBy` and ignores
it on `refs/heads`, so alphabetical is the only order GitHub will apply, and it
pages before this client sees anything: sorting the page by each ref's
`committedDate` orders it and cannot choose it. **A branch outside the page is
reached by narrowing the search and never by scrolling**, which is what the
picker is built around. The filter is the search, going over the wire as
`refs(query:)`, a case-insensitive substring exactly the way `comp.Picker`
matches, debounced through the pair `armCommit` and `settleCommit` already use
so a word typed at speed costs one request. Thirty at a time, with what the
search left out named beside the title. The repository's default branch is
offered first and the current base is always offered, whatever the head is
called: a fork's head carries a name and not a repository, so a contributor's
`main` merging into this one matches the head filter, and a picker that opens
with nothing checked puts the cursor on row one and makes enter a retarget onto
whatever sorted first. Neither is pinned once something has been typed. A
retarget refetches the whole detail and marks the diff stale, and the diff waits
for the detail rather than going with it, because the changed-file count its
overflow line is measured against arrives with it. The row reads "Retargeting to
develop" while the write is out, and "Merging into develop" once it has landed
with nothing counted yet; those are two states rather than one because the
confirming refetch can fail, and a row that latched on the first would report a
finished write as in flight for the rest of the session. `gh.BehindUnknown` is
the count meanwhile, since zero is already spoken for: it means up to date.

The Merge row is the one control that opens a form rather than a picker: a
method, a commit message, a branch to delete, and a button. It is a control on a
clean pull request and on one whose checks are failing, because GitHub's own
button merges the second: a red check no rule requires is not a rule. Blocked
and behind go together and only under `viewerCanMergeAsAdmin`, because they are
the same refusal, a protection rule standing in the way and a flag that lifts
it; the form says `Bypasses branch protection on main` when it is doing that,
which is the one merge here overriding something somebody set on purpose.
Nothing lifts a conflict and nothing merges a draft. Nothing merges an unknown
either, but that one is a wait rather than an answer: GitHub computes
mergeability lazily and the query that reads it is what starts the computation,
so a pull request nothing has looked at recently opens on "Checking" and the row
is inert. One probe is armed for that, on the first landing in a session alone,
which is what keeps it to a single extra request rather than a loop.

**The commit message is GitHub's own, per method**, from
`viewerMergeHeadlineText` and `viewerMergeBodyText`: the repository decides
whether a squash title is the pull request's or its single commit's, and nothing
on this side reconstructs that. Switching method rewrites whichever field the
reader has not touched, because a merge commit and a squash want different
sentences. A rebase answers empty to both, which is GitHub saying it writes no
commit of its own, so the form drops the two fields from the render and from the
ring rather than sending text that gets discarded. `expectedHeadOid` is the
commit that was on screen, snapshotted when the form opens: a refetch landing
behind the modal must not change which commit gets merged, and a branch that
moved comes back refused in GitHub's own words rather than merged unseen.

Deleting the head branch is a second call and it cannot undo the first, so it
runs off the back of the merge's answer and its failure toasts alone. It is not
made at all where the repository sets `deleteBranchOnMerge`: GitHub deletes the
branch itself a moment later, and a call racing that fails on a ref already gone,
which is an error about a thing that worked. There the row is absent rather than
ticked. `deleteRef` takes the ref's node id and no name, which is why the detail
asks for `headRef { id }`; it is null once the branch is gone. Success says
nothing, because that toast lands a beat behind the merge's own and would take
the status bar off the more important of the two.

**`viewerCanDeleteHeadRef` does not answer whether the viewer may delete the
head branch.** It is false on every open pull request, for a repository
administrator as readily as for a stranger, and turns true the moment the pull
request closes. The only time a merge form can be open is while it is open, so a
row gated on that flag never appears once, in any session, for anybody. That
shipped as far as the runbook. There is no field that does answer it: `Ref` has
no viewer permission at all. So the row is ungated, on the same terms a review
request is, and the delete's own failure is what reports a refusal. The one case
refused up front is `isCrossRepository`, a head living in a fork, because
deleting a contributor's branch is worth declining without being asked to.

`MergeEdit` settles against `fieldState` rather than a field of its own. A merge
is a lifecycle move, so a close and a merge in flight together settle
last-held-wins the way two lifecycle writes do, and the fold marks the detail
`StateWriting` for free. It moves the state and leaves `mergeStateStatus` alone,
because the row reads the lifecycle first: a merged pull request says "Merged
into main" whatever GitHub last answered about what stands in the way.

A refused merge is the one revert here that refetches, because the refusal is
evidence about the screen rather than about the pull request. The commonest one
is the head having moved since the detail was fetched, and there the fetched row
is the very thing that lost the merge: putting "Ready to merge" straight back
says the branch is as it was and invites the same press again. `EditRevertedStale`
is what both it and the reviewer write go through, dropping the edit and marking
the fetch in flight stale so an answer asked for before the failure cannot be
believed. Every other write is all or nothing, and reverting one of those says
the pull request never moved, which needs no request to confirm.

Every write the rail makes has a timeline event behind it, and the detail query
asks for all of them: a write nobody can see happen reads as one that did not
land. They arrive one per label and one per person, so the conversation folds a
run of them back into the one line, on `TimelineItem.Subject`, which is the
single field carrying a label's name, a handle, or a branch. An event whose
subject GitHub nulled is dropped rather than rendered as a verb with nothing
after it. The fold is not the one a push uses: a run needs the same actor and
one minute, where a rebase written over a week is still one push. Two review
requests for the same person an hour apart are two things somebody did, and
folding on kind alone reads as "requested reviews from Copilot and Copilot",
which is what PR #20's own history produces.

Built so far: `cmd/zen-octo`, `internal/config`, `internal/gh`, `internal/store`, `internal/version`, and `internal/tui/{app,comp,keys,list,prview,theme}`. The rest lands milestone by milestone; see the **v1** project in Linear.

`internal/store` holds the viewer's login, asked for once at startup, pull request sections, one detail per pull request opened, one diff per pull request whose Files tab was opened, and one diff per commit the cursor settled on in the Commits tab. The two per-pull-request caches are keyed the same and filled separately: the diff costs a second request, so it waits until the tab is asked for. The commit cache is keyed by sha instead, because a commit's diff is the same wherever it is opened from. It follows the cursor on a debounce rather than on every keystroke: the cursor has to sit still for `commitSettleDelay` before its commit is asked for, so walking a long branch costs one request rather than one per commit passed through. Issue sections need their own domain type, query, and row shape, and land with ZNO-15.

Beside those it keeps one set of choices per repository, keyed by `owner/name`: the labels, the assignable users, the branches, and which merge methods the repository allows. They belong to the repository rather than to any pull request, so they outlive the screen that asked and are fetched once. `BeginRepoMeta` refuses one already loaded as well as one in flight; `InvalidateRepoMeta` is what lets a sync reach them.

It also holds the writes still in flight, keyed by pull request and folded in on the way out of `Detail`: a comment onto the timeline, a reply into the review thread it answers, a resolve over the thread it settles, and an `Edit` over the metadata it replaces. Beside the fetched detail rather than inside it: a refetch replaces a timeline wholesale, and one fetched before the mutation answered is not evidence the mutation failed. Written in, an optimistic comment would vanish on the next refresh with nothing to say why. A thread the refetch no longer carries has nowhere to hang a reply, and the reply waits out of sight rather than the store inventing a thread GitHub did not send.

Folding a reply clones both slice levels. A thread's comments are their own slice, still the held one after the threads are cloned, and a thread with spare capacity takes the append in place: a detail already handed out then changes under whoever is holding it, which on this screen is a rendered conversation. A resolve needs the outer clone alone, and it folds through the same one the reply used: cloning again from the held slice throws the reply away. It writes the state and never the permissions, because a locally flipped `CanUnresolve` offers a key that opens a write GitHub rejects. It marks the thread pending, and the key goes inert on a marked one: two resolves out at once settle in the order the responses arrive, which is not the order they were pressed.

An `Edit` settles by writing GitHub's answer into the held detail and then dropping itself, and the answer is stale only against a later write on the same field: `editField` is what keeps a label set landing mid-lifecycle-change from being thrown away. The reviewer panel is the exception, and `dropEdit` hands the write back for it. There is no answer worth taking, because the endpoint reports the outstanding requests and nothing about who has already reviewed, so the write's own optimistic panel is promoted into the held detail instead. Dropping it and waiting for the refetch would put the fetched panel back for the length of a round trip, which reads as the write undoing itself.

Code is highlighted from a Chroma style named by the theme (`Theme.Syntax`), overridable with `syntaxTheme` in config. `internal/tui/comp.Syntax` returns colored tokens rather than rendered text: Chroma's own terminal formatter writes resets that would tear a row's background open.

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
- **Scrolling the shortest distance is right wherever a box is involved, and wrong everywhere else.** A key that lands on a block is taking the reader somewhere, so the block goes to the top row. A box is different twice over. A character typed is not taking them anywhere, and hauling the page for it is the worse of the two wrongs. Opening one is worse still: the box sits under what it answers, so moving it to the top row scrolls the thread off the screen and leaves the reader writing a reply to something they can no longer see. The caret needs no arithmetic of its own, because the box is a fixed height and the textarea scrolls inside it, so a box in view is a caret in view.
- **A text input sized during a render is sized on a copy.** `View` is reached through value receivers all the way down, so a `SetWidth` there is thrown away with the copy, and the real widget keeps a width of zero: it renders from the first character, never scrolls, and every keystroke past the visible edge is invisible while the caret sits off the box. Size the fields when the thing opens and when the screen resizes, never while drawing.
- **A text input recomputes its visible window only when the caret leaves it.** Writing a longer value and then putting the caret inside the window the old one had leaves that window exactly where it was, so the box goes on showing as many characters as the short value did. Ending first and coming back is what forces the recompute. A fixture whose two values are the same length proves none of this.
- **A block that answers the line above it cannot go to the top row either.** The rule holds past the compose box. A review thread in the diff hangs under the code it was written against, so a jump that tops the card scrolls that code away and lands the reader on a comment about something they cannot see. Open a few lines above it instead, and never above the file's own heading, which reads as the wrong file until the eye finds the border.
