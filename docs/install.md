# Install

## Requirements

- **[GitHub CLI](https://cli.github.com), authenticated.** zen-octo rides on
  `gh`'s token rather than asking for one of its own. If `gh auth status` is
  happy, so is zen-octo. Where a scope is missing it says which and prints the
  `gh auth refresh` line to fix it.
- **Go 1.26.6 or later**, to build it.
- A terminal at least **56 by 23**. Under that the shell draws its size instead
  of a screen, so a drawer beside an editor still works and a window smaller
  than the merge form does not pretend to.

## Install

From a clone:

```sh
git clone https://github.com/praxis-labs-io/zen-octo.git
cd zen-octo
make install
```

That builds this tree into `~/.local/bin/zen-octo`. Run it again after every
change or you keep running the old binary.

If `~/.local/bin` is not on your `PATH`:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

## Running it

```sh
zen-octo
```

You land on the pull request list, in the first section your config declares.

There is one subcommand:

```sh
zen-octo config-path
```

It prints where config is read from, which is `~/.zen-octo/config.yml` unless
`ZEN_OCTO_CONFIG_DIR` says otherwise. [Configuration](configuration.md) covers
what goes in it.

`zen-octo --version` says what you are running.

## Upgrading

```sh
git pull
make install
```

Nothing checks for updates and nothing phones home.
