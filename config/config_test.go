package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateRejectsBadConfigs(t *testing.T) {

	base := `port: 8000
health_check_interval: 5s
max_retries: 3
backends: ["http://localhost:8080"]
`
	cases := map[string]string{
		"negative retries":  "port: 8000\nhealth_check_interval: 5s\nmax_retries: -1\nbackends: [\"http://x:1\"]\n",
		"zero interval":     "port: 8000\nhealth_check_interval: 0s\nmax_retries: 3\nbackends: [\"http://x:1\"]\n",
		"missing interval":  "port: 8000\nmax_retries: 3\nbackends: [\"http://x:1\"]\n",
		"no backends":       "port: 8000\nhealth_check_interval: 5s\nmax_retries: 3\nbackends: []\n",
		"bad backend URL":   "port: 8000\nhealth_check_interval: 5s\nmax_retries: 3\nbackends: [\"not-a-url\"]\n",
		"port out of range": "port: 0\nhealth_check_interval: 5s\nmax_retries: 3\nbackends: [\"http://x:1\"]\n",
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadConfig(path); err == nil {
				t.Fatalf("expected %s to be rejected, got nil error", name)
			}
		})
	}

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(base), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	if cfg.Port != 8000 || cfg.MaxRetries != 3 || len(cfg.Backends) != 1 {
		t.Fatalf("unexpected parse: %+v", cfg)
	}
}
