# Install

## Requirements

- **[GitHub CLI](https://cli.github.com), authenticated.** zen-octo rides on
  `gh`'s token rather than asking for one of its own. If `gh auth status` is
  happy, so is zen-octo. Where a scope is missing it says which and prints the
  `gh auth refresh` line to fix it.
- **Nothing else**, for a released binary. Everything is pure Go and statically
  linked, so there is no libc to match.
- **Go 1.26.6 or later**, only if you are building it yourself.
- A terminal at least **56 by 23**. Under that the shell draws its size instead
  of a screen, so a drawer beside an editor still works and a window smaller
  than the merge form does not pretend to.

Releases carry macOS, Linux and Windows, each on arm64 and amd64.

## Install

macOS and Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/praxis-labs-io/zen-octo/main/install.sh | sh
```

Windows:

```powershell
irm https://raw.githubusercontent.com/praxis-labs-io/zen-octo/main/install.ps1 | iex
```

Both download the binary for your machine, check it against the `checksums.txt`
the release publishes, and install nothing that doesn't match. `install.sh` puts
it in `~/.local/bin` and `install.ps1` in `%LOCALAPPDATA%\Programs\zen-octo`.

`INSTALL_DIR` overrides where it lands, and `VERSION` pins a release, as
`VERSION=v0.2.0`.

### From a clone

```sh
git clone https://github.com/praxis-labs-io/zen-octo.git
cd zen-octo
make install
```

That builds this tree into `~/.local/bin/zen-octo`. Run it again after every
change or you keep running the old binary.

### PATH

If the installer says `~/.local/bin` isn't on your `PATH`:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

On Windows:

```powershell
[Environment]::SetEnvironmentVariable('Path', "$env:PATH;$env:LOCALAPPDATA\Programs\zen-octo", 'User')
```

Then open a new terminal. Neither installer edits `PATH` for you.

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

Re-run the installer, or from a clone:

```sh
git pull
make install
```

Nothing checks for updates and nothing phones home.
