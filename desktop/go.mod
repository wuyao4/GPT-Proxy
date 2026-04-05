module gptproxy/desktop

go 1.21

require (
	github.com/jchv/go-webview2 v0.0.0-20260205173254-56598839c808
	gptproxy/shared v0.0.0
)

require (
	github.com/jchv/go-winloader v0.0.0-20250406163304-c1995be93bd1 // indirect
	golang.org/x/sys v0.20.0 // indirect
)

replace gptproxy/shared => ../shared
