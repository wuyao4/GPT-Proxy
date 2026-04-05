package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/jchv/go-webview2"

	proxyshared "gptproxy/shared"
	"gptproxy/shared/controlui"
)

func runDesktop() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	app, err := proxyshared.NewApp(proxyshared.AppOptions{
		DefaultControlListen: "127.0.0.1:0",
	})
	if err != nil {
		return err
	}

	if err := app.StartControlServer(controlui.HandleIndex, false); err != nil {
		return err
	}
	defer func() {
		_ = app.StopProxy()
		_ = app.StopControlServer()
	}()

	controlURL := app.ControlAddr()
	if controlURL == "" {
		return fmt.Errorf("control panel url is empty")
	}

	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		DataPath:  desktopDataPath(),
		WindowOptions: webview2.WindowOptions{
			Title:  "Gpt Proxy",
			Width:  1440,
			Height: 960,
			Center: true,
		},
	})
	if w == nil {
		return fmt.Errorf("failed to create desktop webview; ensure Microsoft Edge WebView2 Runtime is installed")
	}
	defer w.Destroy()

	w.SetSize(1280, 840, webview2.HintMin)
	w.Navigate(controlURL)
	w.Run()
	return nil
}

func desktopDataPath() string {
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}

	path := filepath.Join(base, "gpt-proxy-desktop")
	if err := os.MkdirAll(path, 0o755); err != nil {
		return ""
	}
	return path
}
