# wako

[English](README.md) | [简体中文](README_ZH.md)

A small CLI that remembers how to start your services.

Register a service once — the command and the directory it runs in — then
start, stop, restart and attach to it from anywhere with one command.

```console
$ wako add web pnpm dev
added service "web": pnpm dev (in /Users/you/code/web)

$ wako start web
wako: "web" running as "web": pnpm dev (in /Users/you/code/web)
...
```

## Features

- **One command to start anything** — wako remembers the command and the
  working directory, so you never `cd` around again.
- **Foreground or background** — stay attached to the output, or detach and
  let the service keep running.
- **Attach and detach at any time** — `wako resume` reattaches to a running
  service; `Ctrl+C` detaches without stopping it.
- **Named runs** — the same service can run more than once; each run gets a
  unique name (`web`, `web-1`, `web-2`, ...) or one you choose with `-n`.
- **No daemon** — each run is owned by its own tiny supervisor process.
- **Cross-platform** — works on macOS, Linux and Windows.

## Install

Requires [Go](https://go.dev) 1.26 or later.

```sh
git clone https://github.com/liuenzuo666/wako.git
cd wako
go install .
```

Make sure `$(go env GOPATH)/bin` is in your `PATH`. Alternatively, build a
binary wherever you like:

```sh
go build -o wako .
```

## Usage

### Register a service

Run `wako add` from the directory the service lives in:

```sh
cd ~/code/web
wako add web pnpm dev
```

The command is everything after the name, so no quoting is needed:

```sh
wako add api go run ./cmd/server --port 8080
```

Use `-r` to replace an existing service, and `wako remove <name>` to delete
one.

### Start a service

```sh
wako start web        # foreground: output streams to your terminal
wako start -d web     # background: keeps running after wako exits
```

While attached, `Ctrl+C` stops the service. Detached runs keep going; attach
to one again with:

```sh
wako resume web
```

`Ctrl+C` while resuming detaches without stopping the service.

### Run the same service more than once

Each run gets a unique name: the service name, or `<service>-1`,
`<service>-2`, ... when it is taken. Pick your own with `-n`:

```sh
wako start -d web          # run "web"
wako start -d web          # run "web-1"
wako start -d -n web2 web  # run "web2"
```

### Manage runs

```sh
wako ps              # list running services and their run names
wako stop web-1      # stop one run
wako stop --all      # stop everything
wako restart web     # stop "web" and start it again
wako restart -d web  # restart in the background
```

`restart` uses the service's current command and directory, so changes made
with `wako add -r` take effect on the next restart.

### Manage services

```sh
wako list      # list registered service names
wako list -a   # also show commands and directories
```

## Commands

| Command | Description |
| --- | --- |
| `wako add [-r] <name> <command>` | Register a service (remembers the current directory) |
| `wako remove <name>` | Remove a service (`rm`) |
| `wako start [-d] [-n <run name>] <name>` | Start a service |
| `wako stop [--all] [run name]` | Stop a running service, or all of them |
| `wako restart [-d] [-n <run name>] <name>` | Restart a service with its current command |
| `wako resume <run name>` | Attach to a running service (`Ctrl+C` detaches) |
| `wako ps` | List running services (`runs`) |
| `wako list [-a]` | List services; `-a` also shows commands and directories (`ls`) |
| `wako help` | Show help |

## How it works

`wako start` claims a run name and spawns a hidden supervisor process
(`wako __supervise -n <run name>`), which runs the command in its own process
group and owns the run for its whole lifetime. The supervisor keeps the last
256 KB of output in memory, so attaching a moment late still shows the
startup banner.

Clients talk to the supervisor over a control socket — a Unix socket on
macOS and Linux, a loopback TCP port on Windows. The first frame must
contain a random token stored in the run's state file, so only processes
that can read that file (mode `0600`) may attach or stop a run.

When the service exits, the supervisor lingers briefly to hand the exit code
to a client that is about to attach, cleans up the run's state and socket,
and exits. There is no daemon: stopping all runs leaves nothing behind.

## Files

| Path | Purpose |
| --- | --- |
| `~/.wako/config.json` | Registered services |
| `~/.wako/runs/*.json` | State of live runs (deleted when they end) |
| `~/.wako/runs/*.sock` | Control sockets (Unix) |

Set `WAKO_CONFIG` to override the config file location, which also moves the
`runs` directory next to it (useful for tests).

## Development

```sh
gofmt -l .
go vet ./...
go build ./...
```

## License

[MIT](LICENSE)
