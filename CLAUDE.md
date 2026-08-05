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

Feature-complete work ships via the `ship-feature` skill at `.claude/skills/ship-feature/SKILL.md`: `make all` green, push, draft PR, Copilot + `/code-review`, triage with no tech debt, push then mark ready as separate actions. Manual invocation only.

**That file is a copy, and the copy is deliberate.** The source of truth lives in Drew's global skills; every repo carries a real copy rather than a symlink, because a cloud session clones this repo alone and a link into a sibling checkout would dangle. Propagation is manual. Never edit the copy here: the next copy-out discards the change silently. Edit the source, then copy it in.

### Specs and plans

Scratch, never committed. `docs/` describes only what is true today. Durable context lives in Linear project descriptions and tickets.

## Architecture

`cmd/zen-octo` is the entrypoint (fang over cobra). Everything else lives in `internal/`.

Package boundaries that matter:

- **`internal/gh` is the only package that touches the network.** It returns domain types, never raw GraphQL structs, so everything above it is testable against a fake.
- **`internal/store` owns fetched state and refresh timing.** Views read from it, they never fetch.
- **`internal/tui/*` packages never import each other sideways.** Shared widgets live in `internal/tui/comp`.

Writes are optimistic: apply locally, toast, reconcile on response, revert on error. Every write path needs the revert branch, not just the happy one.

The shell divides the frame top-down: `app` owns the terminal size, subtracts the status bar, and calls `SetSize` on the screen that has focus, which sizes its own panes. **No component reads terminal dimensions or counts chrome lines.** A test asserts the rendered frame never exceeds the size it was given.

Every scrollable region owns its own `bubbles/v2/viewport`. Scroll state never sits on the root model.

Key bindings live in `internal/tui/keys`, declared once with their help text. The help view renders from the same declarations, and tests hold that nothing declared goes unlisted and nothing listed is invented.

Built so far: `cmd/zen-octo`, `internal/config`, `internal/gh`, `internal/store`, `internal/version`, and `internal/tui/{app,comp,keys,list,prview,theme}`. The rest lands milestone by milestone; see the **v1** project in Linear.

`internal/store` holds only pull request sections today. Issue sections need their own domain type, query, and row shape, and land with ZNO-15.

## Rendering traps

Each of these looks like working code and produces a broken frame.

- **Every styled cell ends in a full SGR reset**, which clears the background along with the foreground. A row background has to be set per cell; wrapping a joined row paints only the first one.
- **`lipgloss.Canvas.Compose` ignores a layer's position** and draws every layer at the origin. Compositing an overlay needs `lipgloss.NewCompositor`.
- **`Style.Width` wraps before it clips.** Truncating to a column width means clipping explicitly first, or one long title becomes two rows.
- **`viewport.EnsureVisible` is not a scroll-to-cursor.** It acts only once the line is already outside the window, then puts it on the top row. A cursor moving down a row at a time jumps a whole page and then sits still for the rest of it. Move the offset by hand.
- **A pane clips overflow silently.** A row wider than the pane loses its trailing columns mid-cell with no ellipsis, and a width test still passes because the pane fills its line. The row has to fit before the pane sees it.
- **A viewport offset is a line, and a row is not.** Once rows are two lines and group headers are one, scroll arithmetic that lands on the row it wants opens the window on that row's second line with its title cut off above. Round the offset up to the next item boundary. A test at an even content height proves nothing: the arithmetic lands on boundaries by accident there.
