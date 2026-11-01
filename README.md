# pt — the Pipetower CLI

Run actions and workflows on your servers and follow every step live, check server health,
and open shared terminal sessions — from your terminal.

```
pt workflow run "Deploy App" --server prod -v branch=main
pt server view prod --watch
pt terminal open prod
```

## Install

```sh
# macOS / Linux
brew install --cask pipetower/tap/pt
# or
curl -fsSL https://raw.githubusercontent.com/pipetower/pipetower-cli/main/install.sh | sh
# or, with Go
go install github.com/pipetower/pipetower-cli/cmd/pt@latest
```

`.deb`, `.rpm` and Windows builds are attached to every [release](https://github.com/pipetower/pipetower-cli/releases).

## Configure

Create an API token in Pipetower (**Account → API tokens**) and put it into a `.pipetower` file:

```sh
printf 'PIPETOWER_URL=https://app.pipetower.com\nPIPETOWER_TOKEN=<token>\n' > ~/.pipetower
chmod 600 ~/.pipetower
pt status
```

The token is taken from the first of these places which has one:

1. the environment: `PIPETOWER_TOKEN` (handy in CI)
2. a `.pipetower` file in the current directory or one of its parents
3. `~/.pipetower`

The URL always comes from the same place as the token, so that a `.pipetower` file of a cloned
repository can never send your token to another server. Only `PIPETOWER_URL` from your environment
overrides it. A `.pipetower` in a project directory with its own token can point to another
instance, e.g. your local development instance.

pt only uses files which belong to you, accepts `http://` (and a `ws://` real-time gateway) only for
local instances (`localhost`, `*.localhost`, loopback addresses), and warns when a file is readable by
others, lies unignored in a git repository, or is ignored because it has a URL but no token.

## Commands

Every group is a singular noun, every verb means the same everywhere, and there is exactly one
command for each task (no aliases).

| Command | |
|---|---|
| `pt action list \| view \| run` | actions: `run <action> -s <server>…` runs one and follows it live |
| `pt workflow list \| view \| run` | workflows, the same as actions |
| `pt run list \| view \| watch \| cancel` | runs: filter with `--server`, `--action`, `--workflow`, `--pipe`, `--status`; `view --log` prints the log |
| `pt server list \| view \| connect \| disconnect` | servers and their health, `view --watch` keeps the dashboard live |
| `pt variable list` | your variables, or with `--server` / `--team` those of a server or team |
| `pt terminal list \| open \| join` | shared terminal sessions |
| `pt pipe list \| view` · `pt template list \| view` · `pt team list \| view` | |
| `pt status` | check configuration, token and the real-time gateway |
| `pt api <method> <path> [-d json]` | raw authenticated API request |

Running takes `-s <server>` (repeatable), `-v name=value` (missing variables are asked for),
`--detach`, `--full` (complete output instead of the step tree) and `--quiet`. Every `view`
accepts `--web` to open the item in the web app, every list of named items `--search`.
`pt version` prints the version.

Servers can be given by name, id or `tag:<tag>` (all servers with that tag).

In a terminal, everything you leave out is asked for: `pt server view`, `pt workflow run` or
`pt terminal join` without arguments open a picker which you narrow down by typing (fuzzy, also
by host or tag); running asks for the servers (Tab selects several) and the missing variables.
Ambiguous names open the same picker. What you give as argument or flag is never asked for, and
in scripts, CI or with `--no-input` nothing is asked at all: missing values fail with a usage error.

Global flags: `-o table|json|yaml`, `--no-input`, `--no-color` (or `NO_COLOR`), `--team`, `-h` (help).

### Live runs

`pt action run` and `pt workflow run` show a live tree of every server, workflow step and action
step with the tail of the running step's output. `Ctrl+C` asks whether to terminate the run or to
detach from it (`pt run watch <run>` follows it again). When the output is redirected, pt writes a
line-oriented log instead — still in real time.

### Terminal sessions

`pt terminal open <server>` opens a shared session: teammates can watch it or get control from the web
or with `pt terminal join <session>`. After Enter, type `~.` to detach, `~c` to request control,
`~e` to end the session, `~?` for help.

### Exit codes

| | |
|---|---|
| 0 | success (run successful or skipped) |
| 1 | run failed, partially failed, canceled or not allowed |
| 2 | usage error |
| 3 | configuration or authentication error |
| 4 | API error or not found |
| 130 | interrupted, terminated or detached |

## Development

```sh
make build     # bin/pt
make test      # with the race detector
make lint      # golangci-lint, same version as CI
make fmt       # gofmt and goimports through golangci-lint
make golden    # update the golden files after an intended change of a view, then review the diff
make release-snapshot   # goreleaser, all platforms into dist/
```

Packages: `cmd/pt` (entry), `internal/cli` (commands), `internal/api` (REST client),
`internal/gateway` (WebSocket client), `internal/config` (.pipetower discovery),
`internal/resolve` (names → resources), `internal/prompt` (pickers and questions),
`internal/runview` (live runs), `internal/serverview` (dashboards), `internal/terminal` (sessions),
`internal/present` (statuses, run rows), `internal/ui` (generic styles, tables, formatting),
`internal/fault` (errors and exit codes), `internal/browser` (opens the web app),
`internal/debounce` (coalesces bursts of changes into one refetch), `internal/version` (build information).
