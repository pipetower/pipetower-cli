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

Create an API token in Pipetower (**Account → API tokens**) and add the instance as environment:

```sh
pt env add        # asks for the token, hidden
pt status
```

This writes `~/.pipetower`, a JSON file with the environment `app.pipetower.com`, readable by you only.
In a script, pass the token with `--token`. A second instance, e.g. a local one, is another
environment: `pt env add --url http://pipetower.localhost --team Ops` adds `pipetower.localhost`.

```json
{
  "environment": "app.pipetower.com",
  "environments": {
    "app.pipetower.com":   {"url": "https://app.pipetower.com", "token": "<token>"},
    "pipetower.localhost": {"url": "http://pipetower.localhost", "token": "<token>", "team": "Ops"}
  }
}
```

`environment` is the active one, `pt env list` shows them and `pt env use pipetower.localhost` switches. A `team`
limits all commands to that team while the environment is active: `pt team use Ops` sets it,
`pt team use --all` lifts it, `--team` and `--all-teams` override it for one command.

A project folder can carry a `.pipetower` with only `{"environment": "local", "team": "Ops"}` (put it
into `.gitignore`), the environments then come from `~/.pipetower`. `PIPETOWER_CONFIG` names another
file to use instead of `~/.pipetower`. In CI, `PIPETOWER_URL` and `PIPETOWER_TOKEN` replace the file,
`PIPETOWER_ENV` and `PIPETOWER_TEAM` choose environment and team.

## Usage

```sh
pt --help
```

Actions, workflows and templates are described by definition files (YAML), the same files the
export in Pipetower writes. `pt` prints, checks and applies them:

```sh
pt action example > deploy.yml          # a definition with every option, to start from
pt action apply -f deploy.yml --dry-run # check it against your team
pt action apply -f deploy.yml           # create it, or replace the action of that name
pt action export "Deploy App"           # print the definition of an existing action
pt action delete "Deploy App"
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
