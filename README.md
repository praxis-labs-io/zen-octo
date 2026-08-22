# zen-octo

A pull request, end to end, without opening a browser. Read it, discuss it,
watch its CI, fix its metadata, merge it.

A terminal client for GitHub, built for the part of the day spent in other
people's branches.

## Install

Needs the [GitHub CLI](https://cli.github.com) authenticated, and Go 1.26.6 or
later to build. zen-octo rides on `gh`'s token rather than asking for one of its
own.

```sh
git clone https://github.com/praxis-labs-io/zen-octo.git
cd zen-octo
make install
```

That puts the binary in `~/.local/bin`. [Install](docs/install.md) covers the
requirements and upgrading.

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
