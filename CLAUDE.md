# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test

This project uses [devenv](https://devenv.sh/) to manage the Go toolchain. All commands must be run through devenv:

```sh
devenv shell -- go test ./...        # run all tests
devenv shell -- go test -run TestName ./...  # run a single test
devenv shell -- go build -o redirector .     # build binary
devenv up                            # run the server locally
```

## Architecture

Single-file Go HTTP server (`main.go`) that redirects requests based on `.txt` files on disk.

- **Route loading**: `loadRoutes()` walks `REDIRECT_DIR`, turning file paths into route keys (e.g., `go/github.txt` → `/go/github`) and file contents into redirect targets.
- **Hot reload**: A background goroutine reloads routes from disk every 100ms using `atomic.Pointer` for lock-free swaps.
- **Redirect handler**: `redirectHandler()` validates the request path (alphanumeric, dashes, underscores only; max 64 chars), looks it up in the route map, prepends `https://` if no scheme is present, and issues a 301 redirect.
- **Environment**: `REDIRECT_DIR` (required) and `PORT` (default `8080`).

The server is designed to sit behind an nginx reverse proxy that strips a `/go/` prefix.
