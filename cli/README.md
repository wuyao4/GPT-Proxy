# Gpt Proxy CLI

Standalone CLI build. It starts the local proxy, prints the proxy address, and keeps streaming logs in the terminal. It does not include the browser control panel.

## Features

- Start directly with command-line flags
- Enter interactive mode when no arguments are provided
- Prompt only for the upstream URL when flags are used but `-upstream` is omitted
- Print the proxy base URL and available routes after startup
- Keep streaming logs until `Ctrl+C`

Current proxy routes:

- `GET /v1/models`
- `POST /v1/responses`
- `POST /v1/messages`
- `POST /v1/chat/completions`

## Run

From `cli/`:

```powershell
go run .
```

Without arguments the CLI enters interactive mode and asks for:

- startup option
- upstream URL
- local listen host
- local port

Direct startup example:

```powershell
go run . -upstream https://api.openai.com -host 127.0.0.1 -port 3000
```

If you pass some flags but omit `-upstream`, the CLI will prompt only for the upstream URL and then continue startup.

## Flags

- `-upstream`
  - Upstream `responses` URL or an OpenAI-compatible root URL
  - Example: `https://api.openai.com/v1/responses`
  - Root URLs such as `https://api.openai.com` also work
- `-host` / `-listen-host`
  - Local proxy listen host
  - Default: `127.0.0.1`
- `-display-host`
  - Host name printed to the terminal after startup
  - Defaults to the listen host
  - Falls back to `127.0.0.1` when listening on `0.0.0.0`, `::`, or `[::]`
- `-port`
  - Local proxy port
  - `0` or omitted means a random free port

## Examples

Fixed port:

```powershell
go run . -upstream https://api.openai.com -port 3000
```

Listen on all interfaces but still print a local address:

```powershell
go run . -upstream https://api.openai.com -host 0.0.0.0 -port 3000
```

Override the printed host:

```powershell
go run . -upstream https://api.openai.com -host 0.0.0.0 -display-host 192.168.1.10 -port 3000
```

## Build

From `cli/`:

```powershell
go build -o gpt-proxy-cli.exe .
```

## Test

From `cli/`:

```powershell
go test ./...
```

## Notes

- This build is intended for terminal-first workflows.
- Shared proxy behavior lives in `../shared/`, not in the CLI module itself.
