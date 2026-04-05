# Gpt Proxy Web

Browser-based control panel for testing the upstream endpoint, starting and stopping the proxy, and reading live logs.

## Features

- Local browser control panel
- Upstream `responses` compatibility test
- Start the local proxy and show the proxy address
- Stream runtime logs in real time
- Automatically open the browser after startup

Current proxy routes:

- `GET /v1/models`
- `POST /v1/responses`
- `POST /v1/messages`
- `POST /v1/chat/completions`

## Run

From `web/`:

```powershell
go run .
```

Default behavior:

- The control panel listens on a random local port
- The browser opens automatically after startup
- You can test the upstream connection and start the proxy from the page

## Environment Variables

- `CONTROL_ADDR`
  - Control panel listen address
  - Default: `127.0.0.1:0`
- `PROXY_BIND_HOST`
  - Proxy listen host
  - Default: `127.0.0.1`
- `DISPLAY_HOST`
  - Host name shown to the user for proxy URLs
  - Defaults to `PROXY_BIND_HOST`
- `HTTP_TIMEOUT_SECONDS`
  - Upstream request timeout in seconds
  - Default: `60`

## Build

From `web/`:

```powershell
go build -o gpt-proxy-web.exe .
```

## Test

From `web/`:

```powershell
go test ./...
```

## Notes

- The Web build and Desktop build share the same embedded control UI from `../shared/controlui/`.
- Shared proxy behavior lives in `../shared/`.
