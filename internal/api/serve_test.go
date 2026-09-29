package api

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServeGracefulShutdown(t *testing.T) {
	logger, _ := jsonLogger()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()

	started := make(chan struct{})
	release := make(chan struct{})
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		_, _ = io.WriteString(w, "done")
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- Serve(ctx, ln, h, logger) }()

	// Start a request that stays in flight until released.
	type result struct {
		body string
		err  error
	}
	got := make(chan result, 1)
	go func() {
		resp, err := http.Get("http://" + addr + "/")
		if err != nil {
			got <- result{err: err}
			return
		}
		defer func() { _ = resp.Body.Close() }()
		b, err := io.ReadAll(resp.Body)
		got <- result{body: string(b), err: err}
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("request never reached the handler")
	}

	cancel()

	// Shutdown waits for the in-flight request, so Serve must not return yet.
	select {
	case err := <-served:
		t.Fatalf("Serve returned %v before the in-flight request finished", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	select {
	case r := <-got:
		if r.err != nil || r.body != "done" {
			t.Errorf("in-flight request = %q, %v; want \"done\", nil", r.body, r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("in-flight request never completed")
	}
	select {
	case err := <-served:
		if err != nil {
			t.Errorf("Serve = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after shutdown")
	}

	// The listener is closed, so new connections are refused.
	if conn, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
		_ = conn.Close()
		t.Error("new connection accepted after shutdown")
	}
}
