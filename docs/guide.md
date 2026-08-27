# Guide

zen-octo opens a pull request as four tabs and keeps you on the keyboard through
all of them: read it, discuss it, watch its CI, set its metadata, merge it.

The keymap is in [keys](keys.md), the config file in
[configuration](configuration.md), and installing it in [install](install.md).

## The list

You open on a list of pull requests, divided into sections. Each section is a
GitHub search query you wrote, so what is on screen is whatever you told it to
watch: yours, the ones wanting your review, the ones you are involved in.
`]` and `[` walk the sections, `⏎` opens a row, `s` refetches.

A section that has never answered shows a block rather than rows, and a failed
one shows its error. A reload keeps its rows: the spinner moves to the status
bar and the keys keep working on what is already there.

`/` opens a search bar over the section on screen. It narrows the rows as you
type, against the number, the repository, the title, the author and the head
branch, and it reads what is already fetched rather than asking GitHub. `⏎`
keeps the filter and hands the keyboard back, so `j` and `⏎` then work on what
is left. `esc` clears it. The bar stays on screen while a filter stands, with
what it matched counted against the section it narrowed. The tab counts are the
section's own either way.

A search reaches only what the section returned. Widening beyond that is a
change to the section's query in the config file.

`y` copies a pull request's link and `O` opens it in a browser. Both work on the
list and on every tab of the detail screen.

## The pull request

Four tabs, walked with `]` and `[`.

`tab` and `shift+tab` step the column that drives the pane: the file on Files,
the commit on Commits, the check on Checks. They work from the pane, so you move
through the diffs without leaving the one you are reading. The conversation has
no such column and the key does nothing there.

**Conversation** is the description and every comment and review under it, as
cards. `}` and `{` walk them.

**Files** is the diff. `}` and `{` walk the
hunks and the comments written against them, and `m` marks a file viewed. `|`
puts the two sides in two columns, where `h` and `l` step between them.

**Commits** lists them, walked whole rather than by hunk.

**Checks** is CI. `/` searches a job's log, `n` and `N` walk the matches, `f`
jumps to the first failure, and `r` reruns a job.

Whatever the tab, `}` and `{` mean the same thing: go to the next block. What a
block is belongs to the tab.

## Talking

`c` writes a comment. On Files it is scoped to what the cursor is on, so a
comment lands on the line you are reading rather than the file as a whole.

`r` replies to the focused card, `R` quotes it first. `e` edits your own, `D`
deletes one behind a confirm, and `x` resolves or unresolves a thread. `+` opens
GitHub's eight reactions over the block and toggles the one you pick.

`ctrl+⏎` posts. `ctrl+e` opens `$EDITOR` for anything longer than a line, and
what you write there comes back into the box.

`v` on a review comment shows it in the diff, which is the jump from the
conversation to the code it was written against.

Writes are optimistic. What you did appears immediately, and if the API refuses
it the change is reverted and a toast says so.

## The details rail

`d` opens a rail down the left carrying five fields: state, labels, reviewers,
assignees and the base branch.

Every row answers to `⏎`, which opens a picker as a centred modal. The picker
owns the keyboard while it is up: the keys that can never be text go first, the
filter claims every printable one, and movement takes what is left.

The rail is a list of controls rather than blocks of prose, so `j` and `k` walk
its rows and the braces are dead on it. Its cursor stops at each end rather than
wrapping.

`d` answers at every width the shell will draw, because the rail is where those
five writes live. What the width decides is where it lands: wide enough and it
is a column, narrower and it is painted over the conversation, so the writes
hold in a drawer beside an editor.

## Merging

The merge form offers the methods the repository allows, with the commit message
GitHub itself would use for each. It is not a message zen-octo invents: the
repository decides the headline and body per method, and the form shows what you
are actually about to write.

Where the head branch can be deleted the form offers that too, and where a
protection would be bypassed it says so before the button rather than after.

## Sizing

Under 56 by 23 the shell draws its size instead of a screen. The height is where
the merge form stops fitting; the width is where the rail stops landing whole.

Nothing is torn down under it, so a terminal dragged small and back is the one
it was with whatever was open still open. What is dropped is the keyboard,
everything but the ways out: a picker still holding a set would otherwise write
it on a blind `⏎`, and the merge form's button is one blind `⏎` from a merge.

## Colors

zen-octo takes its colors from your terminal. The hues are ANSI slots, so the
accent, the check marks and the diff markers are the ones your palette already
uses; the greys, the borders and the cursor line are blended from the background
your terminal reports when it starts, so they sit just above it whether it is
light or dark. Nothing needs configuring for this, and there is no theme to
pick.

Code is the exception. Syntax palettes are all fixed colors and none of them is
yours, so the diff is highlighted in GitHub's, paired light or dark against the
same background. [Configuration](configuration.md) covers overriding a color it
gets wrong, running against a translucent terminal, and choosing a different
palette for code.
