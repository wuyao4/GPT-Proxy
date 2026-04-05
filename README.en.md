# GPT Proxy

[中文说明](./README.md)

`GPT Proxy` is a Go-based local proxy that exposes OpenAI-style and Claude-style endpoints while forwarding requests to an upstream OpenAI-compatible `responses` API.

This repository is split into three independent deliverables:

- `web/`: browser-based control panel
- `cli/`: standalone command-line build
- `desktop/`: Windows desktop build powered by WebView2

All three builds reuse the same proxy core from `shared/`.

## What It Does

The proxy currently exposes:

- `GET /v1/models`
- `POST /v1/responses`
- `POST /v1/messages`
- `POST /v1/chat/completions`

Typical use cases:

- point local tools at a single local endpoint
- test an upstream OpenAI-compatible service before wiring clients to it
- expose OpenAI-style and Claude-style routes from the same local process
- run the proxy from a browser UI, a CLI, or a desktop app depending on deployment needs

## Repository Layout

```text
project/
  cli/       Standalone CLI app
  desktop/   Windows desktop app using WebView2
  shared/    Shared proxy core, app lifecycle, logs, and shared control UI
  web/       Browser-based control panel
```

`shared/` exists to avoid duplicating proxy behavior in three separate apps. It contains:

- proxy routing and protocol adaptation
- app lifecycle management for starting and stopping the proxy
- log streaming and status snapshots
- the shared control-surface frontend used by both `web` and `desktop`

## Version Overview

### Web

Use `web/` when you want a browser control panel that automatically opens and lets you test the upstream target, start the proxy, and inspect logs.

See: [web/README.md](/D:/App/tool/Go/project/web/README.md)

### CLI

Use `cli/` when you want a lightweight terminal-only build. It supports direct flags and an interactive mode when startup parameters are omitted.

See: [cli/README.md](/D:/App/tool/Go/project/cli/README.md)

### Desktop

Use `desktop/` on Windows when you want the same control surface as the web build but inside a desktop window, without depending on an external browser window.

See: [desktop/README.md](/D:/App/tool/Go/project/desktop/README.md)

## Quick Start

### Web

```powershell
cd web
go run .
```

### CLI

```powershell
cd cli
go run .
```

Direct CLI startup example:

```powershell
go run . -upstream https://api.openai.com -host 127.0.0.1 -port 3000
```

### Desktop

```powershell
cd desktop
go run .
```

## Build

Build each deliverable separately:

```powershell
cd web
go build -o gpt-proxy-web.exe .
```

```powershell
cd cli
go build -o gpt-proxy-cli.exe .
```

```powershell
cd desktop
go build -buildvcs=false -o gpt-proxy-desktop.exe .
```

## Test

Run tests per module:

```powershell
cd shared
go test ./...
```

```powershell
cd cli
go test ./...
```

```powershell
cd web
go test ./...
```

```powershell
cd desktop
go test ./...
```

## Runtime Notes

- `desktop/` requires Microsoft Edge WebView2 Runtime on Windows.
- `web/` and `desktop/` share the same embedded control UI from `shared/controlui/`.
- `cli/` does not include the control panel. It starts the proxy and prints the proxy address directly to the terminal.

## Common Configuration

Shared environment variables used by the app core:

- `CONTROL_ADDR`
  - Control panel listen address
  - Default: `127.0.0.1:0`
- `PROXY_BIND_HOST`
  - Proxy listen host
  - Default: `127.0.0.1`
- `DISPLAY_HOST`
  - Host shown to users in proxy URLs
  - Defaults to the proxy bind host
- `HTTP_TIMEOUT_SECONDS`
  - Upstream request timeout
  - Default: `60`

## Troubleshooting

### Go module cache permission errors

If you see errors like:

```text
go: could not create module cache
```

check your Go environment first. A broken `GOPATH` that points into `C:\Program Files\Go\bin` can cause permission failures. A normal setup should use a writable user directory such as:

```powershell
go env -w GOPATH=C:\Users\<your-user>\go
```

### Desktop window fails to start

If the desktop build cannot create its window, verify that WebView2 Runtime is installed and working.

## Notes

- The repository is intentionally split so `web`, `cli`, and `desktop` can be packaged independently.
- Behavioral changes to proxy routing should usually be made in `shared/`, not duplicated in app-specific folders.
