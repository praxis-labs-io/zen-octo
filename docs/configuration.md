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
  accent: "#8839ef"
  error: "1"
transparent: false
syntaxTheme: ""
```

`theme` is a set of overrides for the colors it got wrong, every key optional
and layered over what was derived. A value is a hex like `"#c4a7e7"` or an ANSI
index from `"0"` to `"255"`, which is worth preferring: an index follows your
palette where a hex pins the color to itself. A key that is not a color, or a
value outside that range, is named on the notice line and the whole set is left
unapplied rather than half of it.

The keys are `text`, `accent`, `subtle`, `muted`, `inverted`, `success`,
`warning`, `error`, `actor`, `selectedBackground`, `addedBackground`,
`removedBackground`, `border`, `borderSubtle` and `borderMuted`.

`background` and `foreground` are the last two and they do not work like the
others. They are not layered over a derived color; they are what everything else
is derived *from*. zen-octo paints the background, and the shades travel from it
toward the foreground:

```yaml
theme:
  background: "#eff1f5"
  foreground: "#4c4f69"
```

Two reasons to reach for them. The first is wanting zen-octo to look different
from the terminal you run it in — a dark client in a light terminal, say. The
second is a terminal that cannot answer the background query: `screen`, and some
`tmux` and `ssh` setups, do not reply, and without an answer zen-octo paints no
cursor line and no diff wash. Naming them brings all of that back.

They beat whatever the terminal reported, so they also fix an answer that was
simply wrong. One line moves the shades, the surfaces and the light-or-dark
syntax pairing together, which is why it is worth preferring over pinning half a
dozen colors by hand.

Naming only the background is fine and usually enough. Where it flips the page
from light to dark or back, the foreground the terminal reported is left on the
wrong side of it, and zen-octo falls back to deriving the greys against plain
black or white rather than building an unreadable ladder between two colors that
are now both dark. Naming the foreground too is how you get the harmony back.

`theme` used to be a name. If yours still says `theme: rose-pine-moon`, zen-octo
starts and says so rather than refusing; there is one theme now and it is yours.

### Transparency

By default zen-octo paints a background: the one your terminal reported, or the
one you named above. Painting the reported one changes nothing you can see, and
it is what keeps every shade sitting on exactly the base it was derived against.

`transparent: true` is the opt-out. It is for a terminal running translucent,
where a window painted over your wallpaper is the thing that spoils the effect,
and it withholds exactly that: the background, and nothing else. The cursor line
and the green and red diff washes are painted as usual. They are one row each
rather than a window, and each of them says something — where you are, what was
added, what was taken away.

It said "paint nothing" through v0.2.0 and took those with it. That left the
pull request list, the pickers, the file, commit and check columns and the merge
form with no cursor at all, which is a client you navigate by memory for a
translucency they were never obscuring.

The two combine. Naming a background under `transparent: true` means "derive
against this, do not paint it" — which is the answer for a translucent terminal
that also cannot answer the query.

A terminal that never answers and names nothing paints nothing either, since a
surface guessed against an unknown background lands invisible about as often as
not. That one still costs you the cursor outside the diff and the rail, and
naming a `background` is the fix: it restores every surface.

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
