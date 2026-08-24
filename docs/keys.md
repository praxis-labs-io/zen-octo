# Keys

`?` lists every binding without leaving the app. This page is the same set,
written out.

Every binding is declared once in `internal/tui/keys/keys.go` with its help text
attached, so the overlay and this page cannot drift from what `Update` matches
on.

The keymap is shared with zen-review by convention, so learning one tool teaches
you the other, and each still ships on its own schedule.

## Everywhere

| Key | Does |
| --- | --- |
| `?` | help |
| `q` | quit |
| `ctrl+c` | quit from anywhere |

`q` and `ctrl+c` are separate because the compose pane takes plain letters as
text. While a box is open `q` types a `q`, and `ctrl+c` still quits: one way out
has to work from everywhere.

## The list

| Key | Does |
| --- | --- |
| `k` `↑` | up |
| `j` `↓` | down |
| `g` `G` | top, bottom |
| `pgup` `pgdn` | page |
| `ctrl+u` `ctrl+d` | half page |
| `]` | next section |
| `[` | previous section |
| `⏎` | open |
| `s` | sync |
| `y` | copy link |
| `O` | open in browser |
| `/` | search |
| `esc` | clear the search |

`/` narrows the section on screen against the number, the repository, the title,
the author and the head branch. It reads what is already fetched, so it costs no
request and reaches nothing the section did not return. `⏎` hands the keyboard
back and leaves the filter standing; `esc` clears it. Every other key on the
screen is a character while the bar is open.

## The pull request

The same movement keys serve every pane. Focus decides what they move.

### Moving

| Key | Does |
| --- | --- |
| `k` `↑` / `j` `↓` | up, down |
| `g` `G` | top, bottom |
| `pgup` `pgdn` | page |
| `ctrl+u` `ctrl+d` | half page |
| `]` `[` | next, previous tab |
| `tab` `shift+tab` | next, previous in the column |
| `}` `{` | next, previous block |
| `h` `←` / `l` `→` | pane left, pane right |
| `1` `2` `3` | focus a pane by its badge |
| `esc` | back |

### Files

| Key | Does |
| --- | --- |
| `m` | mark viewed |
| `\|` | side by side |
| `v` | show in the diff |

### Talking

| Key | Does |
| --- | --- |
| `c` | comment |
| `r` | reply, or rerun a job |
| `R` | quote reply |
| `e` | edit |
| `D` | delete |
| `x` | resolve or unresolve |
| `+` | react |
| `ctrl+⏎` | post |
| `ctrl+e` | open `$EDITOR` |

### Checks

| Key | Does |
| --- | --- |
| `/` | search the log |
| `n` `N` | next, previous match |
| `f` | first failure |

### The rest

| Key | Does |
| --- | --- |
| `d` | details rail |
| `space` | expand |
| `⏎` | open, or press |
| `s` | sync |
| `y` | copy link |
| `O` | open in browser |

## In a form

| Key | Does |
| --- | --- |
| `tab` `shift+tab` | next, previous field |
| `space` | check or uncheck |

A picker owns the keyboard while it is up. The order it reads keys in is
deliberate: the ones that can never be text go first, then the filter claims
every printable one, then movement takes what is left.

## Notes

`d` answers at every width the shell will draw, because the details rail is
where five writes live: state, labels, reviewers, assignees and the base
branch. What the width decides is where the rail lands, not whether it comes.

The rail is a list of controls rather than blocks of prose, so `j` and `k` walk
its rows and the braces are dead on it. Its cursor stops at each end rather than
wrapping, and `g`, `G` and the page keys go to the ends of the pane rather than
the ends of the list.

The concepts behind all of this are in [the guide](guide.md), and the config
file in [configuration](configuration.md).
