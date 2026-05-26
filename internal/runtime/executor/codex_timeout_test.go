package executor

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

func TestCodexResponseHeaderTimeoutDefaultsAndDisable(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want time.Duration
	}{
		{name: "nil config defaults", cfg: nil, want: 30 * time.Second},
		{name: "empty config disables", cfg: &config.Config{}, want: 0},
		{name: "explicit zero disables", cfg: &config.Config{SDKConfig: config.SDKConfig{CodexResponseHeaderTimeout: 0}}, want: 0},
		{name: "explicit positive", cfg: &config.Config{SDKConfig: config.SDKConfig{CodexResponseHeaderTimeout: 5}}, want: 5 * time.Second},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exec := NewCodexExecutor(tc.cfg)
			if got := exec.codexResponseHeaderTimeout(); got != tc.want {
				t.Fatalf("codexResponseHeaderTimeout() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEnsureCodexResponseHeaderTimeoutNoopWhenDisabled(t *testing.T) {
	client := &http.Client{Transport: &http.Transport{}}
	ensureCodexResponseHeaderTimeout(client, 0)

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", client.Transport)
	}
	if transport.ResponseHeaderTimeout != 0 {
		t.Fatalf("ResponseHeaderTimeout = %v, want 0", transport.ResponseHeaderTimeout)
	}

	ensureCodexResponseHeaderTimeout(client, 7*time.Second)
	transport, ok = client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport after enable = %T, want *http.Transport", client.Transport)
	}
	if transport.ResponseHeaderTimeout != 7*time.Second {
		t.Fatalf("ResponseHeaderTimeout = %v, want 7s", transport.ResponseHeaderTimeout)
	}
}

func TestWrapCodexFirstEventReaderSkipsWhenDisabled(t *testing.T) {
	body := io.NopCloser(&emptyReader{})
	if got := wrapCodexFirstEventReader(body, 0); got != body {
		t.Fatalf("disabled first event wrapper returned %T, want original body", got)
	}
	if got := wrapCodexFirstEventReader(body, time.Second); got == body {
		t.Fatalf("enabled first event wrapper returned original body, want wrapper")
	}
}

type emptyReader struct{}

func (*emptyReader) Read([]byte) (int, error) { return 0, io.EOF }
