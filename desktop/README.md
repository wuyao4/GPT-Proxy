# Gpt Proxy Desktop

Windows desktop build powered by WebView2. It reuses the same control surface as the Web build instead of maintaining a separate native form UI.

## Features

- Reuses the Web control surface inside a desktop window
- Test the upstream endpoint, choose `responses` or `chat_completions` upstream mode, start and stop the proxy, and read logs in one place
- Shows the proxy base URL and available routes after startup
- Does not open an external browser window
- Stops the local control server and proxy when the window closes

Current proxy routes:

- `GET /v1/models`
- `POST /v1/responses`
- `POST /v1/messages`
- `POST /v1/chat/completions`

## Run

From `desktop/`:

```powershell
go run .
```

## Requirements

- Windows 10 or Windows 11
- Microsoft Edge WebView2 Runtime

Most modern Windows installs already include WebView2 Runtime. If the desktop app cannot create its window, install or repair that runtime first.

## Build

From `desktop/`:

```powershell
go build -ldflags "-H windowsgui" -o gpt-proxy-desktop.exe .
```

The `-H windowsgui` flag suppresses the black console window on startup. Errors are shown via a Windows message box instead of stderr.

## Test

From `desktop/`:

```powershell
go test ./...
```

## Notes

- The Desktop build and Web build share the same embedded control UI from `../shared/controlui/`.
- Shared proxy behavior lives in `../shared/`.
- `chat_completions` upstream mode adds a `responses -> chat/completions -> responses` bridge for text requests.
