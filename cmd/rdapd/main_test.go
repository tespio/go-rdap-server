package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return p
}

func TestRunMissingConfig(t *testing.T) {
	quit := make(chan os.Signal, 1)
	err := run(filepath.Join(t.TempDir(), "nope.yaml"), zap.NewNop(), quit)
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestRunInvalidConfig(t *testing.T) {
	p := writeConfig(t, "rdap:\n  base_url: \"https://rdap.example.com\"\n  mode: \"bogus\"\n")
	quit := make(chan os.Signal, 1)
	err := run(p, zap.NewNop(), quit)
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestRunUnsupportedStore(t *testing.T) {
	p := writeConfig(t, `
rdap:
  base_url: "https://rdap.example.com"
storage:
  driver: "bogus"
`)
	quit := make(chan os.Signal, 1)
	err := run(p, zap.NewNop(), quit)
	if err == nil {
		t.Fatal("expected error for unsupported store")
	}
}

func TestRunStartsAndShutsDown(t *testing.T) {
	// Metrics disabled so metricsSrv is nil — also exercises the nil-guard on
	// metrics shutdown. Port 0 binds an ephemeral port so the test never
	// collides with a running instance.
	p := writeConfig(t, `
server:
  host: "127.0.0.1"
  port: 0
rdap:
  mode: "registrar"
  base_url: "https://rdap.example.com"
storage:
  driver: "memory"
metrics:
  enabled: false
rate_limiting:
  enabled: false
`)
	quit := make(chan os.Signal, 1)

	done := make(chan error, 1)
	go func() {
		done <- run(p, zap.NewNop(), quit)
	}()

	// Let the servers start, then signal shutdown.
	time.Sleep(500 * time.Millisecond)
	quit <- os.Interrupt

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run did not shut down in time")
	}
}

func TestRunWithWhoisGateway(t *testing.T) {
	// Enabling the WHOIS gateway must not break startup/shutdown, and the
	// gateway must answer a real WHOIS query.
	p := writeConfig(t, `
server:
  host: "127.0.0.1"
  port: 0
rdap:
  mode: "registrar"
  base_url: "https://rdap.example.com"
storage:
  driver: "memory"
metrics:
  enabled: false
rate_limiting:
  enabled: false
whois:
  enabled: true
  port: 14343
`)
	quit := make(chan os.Signal, 1)
	done := make(chan error, 1)
	go func() { done <- run(p, zap.NewNop(), quit) }()

	// Wait for the WHOIS listener, then run a real query.
	var ok bool
	for i := 0; i < 20; i++ {
		conn, err := net.Dial("tcp", "127.0.0.1:14343")
		if err == nil {
			conn.SetDeadline(time.Now().Add(2 * time.Second))
			conn.Write([]byte("example.com\r\n"))
			buf := make([]byte, 1024)
			n, _ := conn.Read(buf)
			conn.Close()
			if n > 0 && strings.Contains(string(buf[:n]), "Domain Name: example.com") {
				ok = true
			}
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ok {
		quit <- os.Interrupt
		<-done
		t.Fatal("WHOIS gateway did not answer a domain query")
	}

	quit <- os.Interrupt
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run did not shut down in time")
	}
}
