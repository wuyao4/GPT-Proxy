package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	proxyshared "gptproxy/shared"
)

type runOptions struct {
	upstream    string
	port        int
	listenHost  string
	displayHost string
	protocol    string
}

func parseRunOptions(args []string) (runOptions, error) {
	fs := flag.NewFlagSet("gpt-proxy-cli", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	opts := runOptions{}
	fs.StringVar(&opts.upstream, "upstream", "", "upstream OpenAI responses url or host")
	fs.IntVar(&opts.port, "port", 0, "proxy port, 0 means random available port")
	fs.StringVar(&opts.listenHost, "listen-host", "", "proxy listen host")
	fs.StringVar(&opts.listenHost, "host", "", "proxy listen host")
	fs.StringVar(&opts.displayHost, "display-host", "", "proxy display host")
	fs.StringVar(&opts.protocol, "protocol", "", "upstream protocol: responses or chat_completions (default: responses)")
	if err := fs.Parse(args); err != nil {
		return runOptions{}, err
	}

	opts.upstream = strings.TrimSpace(opts.upstream)
	opts.listenHost = strings.TrimSpace(opts.listenHost)
	opts.displayHost = strings.TrimSpace(opts.displayHost)
	opts.protocol = strings.TrimSpace(opts.protocol)
	return opts, nil
}

func runCLI(args []string, stdin io.Reader, stdout io.Writer) error {
	opts, err := parseRunOptions(args)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		var ok bool
		opts, ok, err = promptInteractiveOptions(stdin, stdout, opts)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
	} else {
		opts = applyOptionDefaults(opts)
		if opts.upstream == "" {
			reader := bufio.NewReader(stdin)
			upstream, err := promptRequired(reader, stdout, "Upstream request URL")
			if err != nil {
				return err
			}
			opts.upstream = upstream
		}
	}

	host, hostMode, err := resolveCLIUpstreamTarget(opts.upstream)
	if err != nil {
		return err
	}

	// 标准化协议参数
	protocol := normalizeProtocol(opts.protocol)

	app, err := proxyshared.NewApp(proxyshared.AppOptions{
		DefaultControlListen: "127.0.0.1:0",
		DefaultProxyBindHost: opts.listenHost,
		DefaultDisplayHost:   opts.displayHost,
	})
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.StartProxy(ctx, host, "", opts.port, hostMode, protocol); err != nil {
		return err
	}
	stopped := false
	defer func() {
		if !stopped {
			_ = app.StopProxy()
		}
	}()

	status := app.SnapshotStatus()
	fmt.Fprintln(stdout, "Proxy started")
	fmt.Fprintf(stdout, "Base URL: %s\n", status.ProxyBaseURL)
	for _, route := range status.Routes {
		fmt.Fprintf(stdout, "%s: %s\n", route.Name, route.URL)
	}
	fmt.Fprintln(stdout, "Logs:")

	for _, line := range app.Logger().Snapshot() {
		fmt.Fprintln(stdout, line)
	}

	updates, cancel := app.Logger().Subscribe()
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for line := range updates {
			fmt.Fprintln(stdout, line)
		}
	}()

	<-ctx.Done()
	if err := app.StopProxy(); err != nil {
		return err
	}
	stopped = true
	cancel()
	<-done
	return nil
}

func applyOptionDefaults(opts runOptions) runOptions {
	if opts.listenHost == "" {
		opts.listenHost = "127.0.0.1"
	}
	if opts.displayHost == "" {
		opts.displayHost = defaultDisplayHost(opts.listenHost)
	}
	if opts.protocol == "" {
		opts.protocol = "responses"
	}
	return opts
}

func promptInteractiveOptions(stdin io.Reader, stdout io.Writer, opts runOptions) (runOptions, bool, error) {
	opts = applyOptionDefaults(opts)
	reader := bufio.NewReader(stdin)

	fmt.Fprintln(stdout, "CLI options")
	fmt.Fprintln(stdout, "1. Start proxy")
	fmt.Fprintln(stdout, "2. Exit")

	choice, err := promptWithDefault(reader, stdout, "Select option", "1")
	if err != nil {
		return runOptions{}, false, err
	}

	switch strings.TrimSpace(choice) {
	case "", "1":
	case "2":
		return opts, false, nil
	default:
		return runOptions{}, false, fmt.Errorf("unknown option %q", choice)
	}

	upstream, err := promptRequired(reader, stdout, "Upstream request URL")
	if err != nil {
		return runOptions{}, false, err
	}

	protocolChoice, err := promptWithDefault(reader, stdout, "Upstream protocol (responses/chat_completions)", opts.protocol)
	if err != nil {
		return runOptions{}, false, err
	}

	listenHost, err := promptWithDefault(reader, stdout, "Listen host", opts.listenHost)
	if err != nil {
		return runOptions{}, false, err
	}
	portText, err := promptWithDefault(reader, stdout, "Port", "")
	if err != nil {
		return runOptions{}, false, err
	}

	opts.upstream = upstream
	opts.protocol = normalizeProtocol(protocolChoice)
	opts.listenHost = strings.TrimSpace(listenHost)
	opts.displayHost = defaultDisplayHost(opts.listenHost)
	opts.port = 0

	if strings.TrimSpace(portText) != "" {
		port, err := strconv.Atoi(strings.TrimSpace(portText))
		if err != nil {
			return runOptions{}, false, fmt.Errorf("invalid port: %w", err)
		}
		opts.port = port
	}

	return opts, true, nil
}

func resolveCLIUpstreamTarget(raw string) (string, string, error) {
	normalized, err := proxyshared.NormalizeAbsoluteURL(raw)
	if err != nil {
		return "", "", err
	}

	parsed, err := url.Parse(normalized)
	if err != nil {
		return "", "", fmt.Errorf("invalid upstream url: %w", err)
	}

	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case path == "", path == "/v1", strings.HasSuffix(path, "/v1/responses"):
		return normalized, "default", nil
	default:
		return normalized, "custom", nil
	}
}

func promptRequired(reader *bufio.Reader, stdout io.Writer, label string) (string, error) {
	value, err := promptWithDefault(reader, stdout, label, "")
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", strings.ToLower(label))
	}
	return value, nil
}

func promptWithDefault(reader *bufio.Reader, stdout io.Writer, label, defaultValue string) (string, error) {
	if defaultValue == "" {
		fmt.Fprintf(stdout, "%s: ", label)
	} else {
		fmt.Fprintf(stdout, "%s [%s]: ", label, defaultValue)
	}

	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultValue, nil
	}
	return line, nil
}

func defaultDisplayHost(listenHost string) string {
	switch strings.TrimSpace(listenHost) {
	case "", "0.0.0.0", "::", "[::]":
		return "127.0.0.1"
	default:
		return strings.TrimSpace(listenHost)
	}
}

func normalizeProtocol(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case "chat_completions", "chatcompletions", "chat":
		return "chat_completions"
	case "responses", "response", "":
		return "responses"
	default:
		return "responses"
	}
}
