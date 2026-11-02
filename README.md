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
curl -fsSL https://pipetower.com/install-cli | bash
```

pt runs on Linux and macOS (amd64 and arm64; on Windows use WSL). The script downloads the latest
[release](https://github.com/pipetower/pipetower-cli/releases), verifies its checksum and installs
`pt` to `/usr/local/bin`, or to `~/.local/bin` without write access there. `PT_INSTALL_DIR` chooses
another directory, `PT_VERSION=1.2.0` a specific version.

### Updates

Once a day, when pt starts in a terminal, it looks for a new version and offers to install it. `pt update`
updates at any time. The new version takes effect when pt is started again. Scripts and CI
(no terminal, or `--no-input`) are never asked; `PT_NO_UPDATE_CHECK=1` turns the check off.

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
| `pt update` | update pt to the latest version |

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

Local builds are development builds (`dev-<git describe>`), which never look for updates.

Packages: `cmd/pt` (entry), `internal/cli` (commands), `internal/api` (REST client),
`internal/gateway` (WebSocket client), `internal/config` (.pipetower discovery),
`internal/resolve` (names → resources), `internal/prompt` (pickers and questions),
`internal/runview` (live runs), `internal/serverview` (dashboards), `internal/terminal` (sessions),
`internal/present` (statuses, run rows), `internal/ui` (generic styles, tables, formatting),
`internal/fault` (errors and exit codes), `internal/browser` (opens the web app),
`internal/debounce` (coalesces bursts of changes into one refetch), `internal/update` (finds and installs
new releases), `internal/version` (build information).

### Releasing

Versions follow [semantic versioning](https://semver.org). `make release VERSION=1.2.0` tags `v1.2.0` on an
up-to-date, clean `main` and pushes the tag; the release workflow then builds the archives with goreleaser
and publishes them with `checksums.txt` and the changelog as a GitHub release. From then on the install
script installs it, and every pt offers it as an update. A version with a suffix (`1.3.0-rc.1`) becomes a
prerelease, which only `PT_VERSION=1.3.0-rc.1` installs.

The install script itself is served by pipetower.com (`resources/scripts/install-cli.sh` in its repository).
