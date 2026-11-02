<p align="center"><img src=".github/logo.png" alt="Pipetower" width="500"></p>

# pt — the Pipetower CLI

Run actions and workflows on your servers and follow them live, check server health and open
shared terminal sessions, from your terminal.

## Install

```sh
curl -fsSL https://pipetower.com/install-cli | bash
```

Linux and macOS (on Windows use WSL). It also sets up Tab completion. pt keeps itself up to date:
it offers new versions when it starts, or run `pt update`.

## Configure

Create an API token in Pipetower (**Account → API tokens**) and put it into `~/.pipetower`:

```sh
printf 'PIPETOWER_URL=https://app.pipetower.com\nPIPETOWER_TOKEN=<token>\n' > ~/.pipetower
chmod 600 ~/.pipetower
pt status
```

To limit a project folder to one of your teams, put a `.pipetower` with only the team into it
(and into its `.gitignore`):

```sh
PIPETOWER_TEAM=<team name>
```

## Usage

```sh
pt --help
```

## Development

```sh
make build     # bin/pt
make test
make lint
```

## Releasing

Versions follow [semantic versioning](https://semver.org). `make release VERSION=1.2.0` tags `v1.2.0`
on a clean, up-to-date `main`; GitHub Actions then publishes the release, and every pt offers it as
an update. A suffix like `1.3.0-rc.1` makes it a prerelease, which is never offered.
