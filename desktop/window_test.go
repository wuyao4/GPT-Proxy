package main

import "testing"

func TestDesktopAppOptions(t *testing.T) {
	opts := desktopAppOptions()

	if opts.DefaultProxyBindHost != "0.0.0.0" {
		t.Fatalf("unexpected default proxy bind host: %q", opts.DefaultProxyBindHost)
	}
	if opts.DefaultDisplayHost != "127.0.0.1" {
		t.Fatalf("unexpected default display host: %q", opts.DefaultDisplayHost)
	}
	if opts.DefaultControlListen != "127.0.0.1:0" {
		t.Fatalf("unexpected default control listen: %q", opts.DefaultControlListen)
	}
}
