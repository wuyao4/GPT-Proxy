package main

import (
	"io"
	"strings"
	"testing"
)

func TestParseRunOptionsDefaults(t *testing.T) {
	opts, err := parseRunOptions(nil)
	if err != nil {
		t.Fatalf("parse default options: %v", err)
	}

	if opts.port != 0 {
		t.Fatalf("unexpected default port: %d", opts.port)
	}
	if opts.upstream != "" {
		t.Fatalf("unexpected default upstream: %q", opts.upstream)
	}
}

func TestParseRunOptionsValues(t *testing.T) {
	opts, err := parseRunOptions([]string{
		"-upstream", " https://example.com/v1/responses ",
		"-port", "3000",
		"-listen-host", "0.0.0.0",
		"-display-host", "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("parse options: %v", err)
	}

	if opts.port != 3000 {
		t.Fatalf("unexpected port: %d", opts.port)
	}
	if opts.upstream != "https://example.com/v1/responses" {
		t.Fatalf("unexpected upstream: %q", opts.upstream)
	}
	if opts.listenHost != "0.0.0.0" {
		t.Fatalf("unexpected listen host: %q", opts.listenHost)
	}
	if opts.displayHost != "127.0.0.1" {
		t.Fatalf("unexpected display host: %q", opts.displayHost)
	}
}

func TestResolveCLIUpstreamTargetDefaultMode(t *testing.T) {
	host, mode, err := resolveCLIUpstreamTarget("https://api.openai.com/v1/responses")
	if err != nil {
		t.Fatalf("resolve cli default target: %v", err)
	}

	if mode != "default" {
		t.Fatalf("unexpected mode: %s", mode)
	}
	if host != "https://api.openai.com/v1/responses" {
		t.Fatalf("unexpected host: %s", host)
	}
}

func TestResolveCLIUpstreamTargetCustomMode(t *testing.T) {
	host, mode, err := resolveCLIUpstreamTarget("https://relay.example.com/openai/responses")
	if err != nil {
		t.Fatalf("resolve cli custom target: %v", err)
	}

	if mode != "custom" {
		t.Fatalf("unexpected mode: %s", mode)
	}
	if host != "https://relay.example.com/openai/responses" {
		t.Fatalf("unexpected host: %s", host)
	}
}

func TestPromptInteractiveOptions(t *testing.T) {
	opts, ok, err := promptInteractiveOptions(
		strings.NewReader("1\nhttps://example.com/v1/responses\n0.0.0.0\n3000\n"),
		io.Discard,
		runOptions{},
	)
	if err != nil {
		t.Fatalf("prompt interactive options: %v", err)
	}
	if !ok {
		t.Fatalf("expected start")
	}
	if opts.upstream != "https://example.com/v1/responses" {
		t.Fatalf("unexpected upstream: %q", opts.upstream)
	}
	if opts.listenHost != "0.0.0.0" {
		t.Fatalf("unexpected listen host: %q", opts.listenHost)
	}
	if opts.displayHost != "127.0.0.1" {
		t.Fatalf("unexpected display host: %q", opts.displayHost)
	}
	if opts.port != 3000 {
		t.Fatalf("unexpected port: %d", opts.port)
	}
}

func TestApplyOptionDefaultsUsesLocalDisplayHostForWildcardBind(t *testing.T) {
	opts := applyOptionDefaults(runOptions{listenHost: "0.0.0.0"})

	if opts.listenHost != "0.0.0.0" {
		t.Fatalf("unexpected listen host: %q", opts.listenHost)
	}
	if opts.displayHost != "127.0.0.1" {
		t.Fatalf("unexpected display host: %q", opts.displayHost)
	}
}

func TestApplyOptionDefaultsKeepsExplicitDisplayHost(t *testing.T) {
	opts := applyOptionDefaults(runOptions{
		listenHost:  "0.0.0.0",
		displayHost: "192.168.1.10",
	})

	if opts.displayHost != "192.168.1.10" {
		t.Fatalf("unexpected display host: %q", opts.displayHost)
	}
}
