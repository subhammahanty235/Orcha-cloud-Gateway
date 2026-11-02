package config

import (
	"testing"
	"time"
)

func TestLoadDefaultsAreLoopback(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ListenAddr != "127.0.0.1:8081" {
		t.Errorf("ListenAddr = %q", cfg.ListenAddr)
	}
	if cfg.InferenceURL != "http://127.0.0.1:8080" {
		t.Errorf("InferenceURL = %q", cfg.InferenceURL)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("ORCHA_LISTEN_ADDR", "127.0.0.1:9000")
	t.Setenv("ORCHA_INFERENCE_URL", "http://127.0.0.1:9001")
	t.Setenv("ORCHA_UPSTREAM_TIMEOUT", "5s")
	t.Setenv("ORCHA_MAX_REQUEST_BYTES", "1024")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Config{
		ListenAddr:      "127.0.0.1:9000",
		InferenceURL:    "http://127.0.0.1:9001",
		UpstreamTimeout: 5 * time.Second,
		MaxRequestBytes: 1024,
	}
	if cfg != want {
		t.Errorf("Load = %+v, want %+v", cfg, want)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	tests := []struct{ name, key, value string }{
		{"relative url", "ORCHA_INFERENCE_URL", "127.0.0.1:8080"},
		{"unsupported scheme", "ORCHA_INFERENCE_URL", "ftp://127.0.0.1:8080"},
		{"unparseable timeout", "ORCHA_UPSTREAM_TIMEOUT", "soon"},
		{"zero timeout", "ORCHA_UPSTREAM_TIMEOUT", "0s"},
		{"unparseable size", "ORCHA_MAX_REQUEST_BYTES", "big"},
		{"negative size", "ORCHA_MAX_REQUEST_BYTES", "-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.key, tt.value)
			if _, err := Load(); err == nil {
				t.Errorf("Load accepted %s=%q", tt.key, tt.value)
			}
		})
	}
}
