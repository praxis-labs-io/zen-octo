# Configuration

zen-octo reads `~/.zen-octo/config.yml`. `zen-octo config-path` prints the exact
path, and `ZEN_OCTO_CONFIG_DIR` overrides the directory.

There is no config file until you write one. Everything below has a default, so
a missing file is a working app rather than a prompt.

## Sections

A section is one tab in the list, a title and a raw GitHub search query.
zen-octo passes the query through as typed, so anything GitHub's search
understands works here.

The key names come from [gh-dash](https://github.com/dlvhdr/gh-dash), where
Dolev Hadar worked this shape out first, so a gh-dash `prSections` block reads
here as written.

```yaml
prSections:
  - title: My PRs
    filters: "is:open is:pr author:@me"
  - title: Needs My Review
    filters: "is:open is:pr review-requested:@me"
  - title: Involved
    filters: "is:open is:pr involves:@me -author:@me"
  - title: Recently Closed
    filters: "is:pr is:closed author:@me closed:>={{since:24h}} sort:updated-desc"

issueSections:
  - title: My Issues
    filters: "is:open is:issue author:@me"
  - title: Assigned
    filters: "is:open is:issue assignee:@me"
```

Those are the shipped defaults. Replacing `prSections` replaces all of them.

### Time tokens

`{{since:24h}}` becomes the RFC 3339 instant that long ago, which is the only
date shape GitHub's search takes. Anything Go's `ParseDuration` reads works:
`30m`, `12h`, `168h`. A duration it cannot parse is left in the query as typed
rather than silently becoming something else.

Note the `sort:updated-desc` on the closed section above. The limit is applied
before the list re-sorts, so without a sort it is relevance that decides which
twenty come back.

## Defaults

```yaml
defaults:
  prsLimit: 20
  issuesLimit: 20
```

How many rows a section fetches. The ceiling is 100.

## Theme

The chrome is derived from your terminal rather than picked from a list. The
hues are ANSI slots, so they are whatever your palette maps them to; the
shades and surfaces are blended from the background your terminal reports when
zen-octo starts, so they sit just above it whether it is light or dark. There
is nothing to configure to get this.

```yaml
theme:
  accent: "#c4a7e7"
  error: "1"
transparent: false
syntaxTheme: ""
```

`theme` is a set of overrides for the colors it got wrong, every key optional
and layered over what was derived. A value is a hex like `"#c4a7e7"` or an ANSI
index like `"5"`, which is worth preferring: an index follows your palette where
a hex pins the color to itself. A key that is not a color, or a color that will
not parse, is reported with its name.

The keys are `text`, `accent`, `subtle`, `muted`, `inverted`, `success`,
`warning`, `error`, `actor`, `selectedBackground`, `addedBackground`,
`removedBackground`, `border`, `borderSubtle` and `borderMuted`.

`background` is the sixteenth and it does not work like the others. It is not a
color to paint — nothing ever paints the background — it is what zen-octo should
believe is already behind the page, and everything else is derived from it:

```yaml
theme:
  background: "#eff1f5"
```

Reach for it when your terminal cannot answer the query or answers it wrong.
`screen`, and some `tmux` and `ssh` setups, do not reply; without an answer
zen-octo paints no cursor line and no diff wash, and naming the background here
brings all of it back. It beats whatever the terminal reported, so it also
fixes an answer that was simply incorrect. One line does the shades, the
surfaces and the light-or-dark syntax pairing together, which is why it is
worth preferring over pinning half a dozen colors by hand.

`theme` used to be a name. If yours still says `theme: rose-pine-moon`, zen-octo
starts and says so rather than refusing; there is one theme now and it is yours.

### Transparency

`transparent: true` stops zen-octo painting the cursor line and the diff's green
and red washes, for a terminal running translucent where an opaque row is the
thing that spoils it. The background is never painted either way. What you give
up is real: a changed line is read as a block, and the bar in the leading cell
and the `+` and `−` markers are what carry it once the wash is gone.

This is also what happens when a terminal does not answer the background query
at all, since a surface guessed against an unknown background lands invisible
about as often as not. If that is your terminal and you wanted the surfaces,
name the background above rather than turning this on.

### Syntax

`syntaxTheme` is a separate question: the palette code is highlighted with.
Chroma's styles are all truecolor and none of them is your terminal, so this is
the one thing that cannot follow your palette. Left empty it pairs against your
background, `github-dark` on a dark terminal and `github` on a light one. Set it
to any Chroma style name to override that.

## A bad config

A section that fails to validate is reported with its section named. The app
draws a notice line rather than refusing to start, so a config you are still
editing does not lock you out of the tool you are editing it for.
