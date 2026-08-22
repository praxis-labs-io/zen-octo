# zen-octo

A pull request, end to end, without opening a browser. Read it, discuss it,
watch its CI, fix its metadata, merge it.

A terminal client for GitHub, built for the part of the day spent in other
people's branches.

Needs the [GitHub CLI](https://cli.github.com) authenticated. zen-octo rides on
`gh`'s token rather than asking for one of its own.

![The pull request list, in sections you defined](docs/images/pull-request-list.png)

You open on a list divided into sections, each one a GitHub search query you
wrote. What is on screen is whatever you told it to watch.

![A pull request's conversation, with the details rail beside it](docs/images/conversation-and-rail.png)

`⏎` opens one. The rail down the left is the only route to five writes: state,
labels, reviewers, assignees and the base branch. It answers at every width the
shell will draw, so a client in a drawer beside an editor is not read-only.

![The diff, with a review comment against the line it was written on](docs/images/review-comments.png)

`]` and `[` walk the four tabs. On Files the tree is beside the diff, review
comments sit against the lines they were written on, and `c` adds one scoped to
the line under the cursor. `\|` splits the diff into two columns, and `?` lists
every key.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/praxis-labs-io/zen-octo/main/install.sh | sh
```

Downloads the binary for macOS or Linux, on arm64 or amd64, and needs no Go.
Windows takes the `.zip` off the
[releases page](https://github.com/praxis-labs-io/zen-octo/releases), the
installer being a POSIX script.

From a clone, which is what you want if you intend to change anything:

```sh
git clone https://github.com/praxis-labs-io/zen-octo.git
cd zen-octo
make install
```

That one needs Go 1.26.6 or later. [Install](docs/install.md) covers the
requirements, the config path and upgrading.

## A first run

```sh
zen-octo
```

You land on a list of pull requests, divided into sections you defined: yours,
the ones wanting your review, the ones you are involved in. Each section is a
GitHub search query, so the list is whatever you told it to watch.

`⏎` opens one. Four tabs from there, walked with `]` and `[`: the conversation,
the files, the commits, and the checks. `}` and `{` walk the blocks on whichever
tab you are on, which means cards on the conversation and hunks on the files.

`c` comments, `r` replies, `+` reacts, `x` resolves a thread. `d` opens the
details rail, which is the only route to the five things you would otherwise
open a browser for: state, labels, reviewers, assignees and the base branch.

`?` lists every key without leaving the app.

## Documentation

- [Guide](docs/guide.md) — the list, the four tabs, the rail, merging
- [Keys](docs/keys.md) — every binding, generated from the same declarations the
  help overlay renders from
- [Configuration](docs/configuration.md) — sections, search queries, themes
- [Install](docs/install.md) — requirements, `config-path`, upgrading
- [Contributing](docs/CONTRIBUTING.md) — the checks, the boundaries, the test
  conventions

## License

MIT. See [LICENSE](LICENSE).
