// Package config loads gateway settings from environment variables.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

// Defaults keep both the gateway and its upstream on loopback. The upstream
// timeout allows for a full 2048-token context at the ~8.8 tokens/sec baseline
// measured on the current VM.
const (
	defaultListenAddr      = "127.0.0.1:8081"
	defaultInferenceURL    = "http://127.0.0.1:8080"
	defaultUpstreamTimeout = 300 * time.Second
	defaultMaxRequestBytes = 1 << 20 // 1 MiB
)

// Config holds the gateway's runtime settings.
type Config struct {
	ListenAddr      string
	InferenceURL    string
	UpstreamTimeout time.Duration
	MaxRequestBytes int64
}

// Load reads configuration from the environment, applying defaults for unset
// variables and rejecting invalid values.
func Load() (Config, error) {
	cfg := Config{
		ListenAddr:      defaultListenAddr,
		InferenceURL:    defaultInferenceURL,
		UpstreamTimeout: defaultUpstreamTimeout,
		MaxRequestBytes: defaultMaxRequestBytes,
	}

	if v := os.Getenv("ORCHA_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}

	if v := os.Getenv("ORCHA_INFERENCE_URL"); v != "" {
		cfg.InferenceURL = v
	}
	u, err := url.Parse(cfg.InferenceURL)
	if err != nil {
		return Config{}, fmt.Errorf("ORCHA_INFERENCE_URL: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return Config{}, fmt.Errorf("ORCHA_INFERENCE_URL: %q must be an absolute http(s) URL", cfg.InferenceURL)
	}

	if v := os.Getenv("ORCHA_UPSTREAM_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("ORCHA_UPSTREAM_TIMEOUT: %w", err)
		}
		if d <= 0 {
			return Config{}, fmt.Errorf("ORCHA_UPSTREAM_TIMEOUT: %q must be positive", v)
		}
		cfg.UpstreamTimeout = d
	}

	if v := os.Getenv("ORCHA_MAX_REQUEST_BYTES"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("ORCHA_MAX_REQUEST_BYTES: %w", err)
		}
		if n <= 0 {
			return Config{}, fmt.Errorf("ORCHA_MAX_REQUEST_BYTES: %q must be positive", v)
		}
		cfg.MaxRequestBytes = n
	}

	return cfg, nil
}
