package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Port                int           `yaml:"port"`
	HealthCheckInterval time.Duration `yaml:"health_check_interval"`
	MaxRetries          int           `yaml:"max_retries"`
	Backends            []string      `yaml:"backends"`
}

// Validate rejects configs that parse but would fail at runtime: a zero
// interval panics time.NewTicker, a negative retry count makes the attempt
// loop unreachable, and an empty backend list 503s every request.
func (c *Config) Validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port must be 1-65535, got %d", c.Port)
	}
	if c.HealthCheckInterval <= 0 {
		return fmt.Errorf("health_check_interval must be > 0, got %v", c.HealthCheckInterval)
	}
	if c.MaxRetries < 0 {
		return fmt.Errorf("max_retries must be >= 0, got %d", c.MaxRetries)
	}
	if len(c.Backends) == 0 {
		return errors.New("at least one backend is required")
	}
	for _, b := range c.Backends {
		u, err := url.Parse(b)
		if err != nil {
			return fmt.Errorf("backend %q: %w", b, err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("backend %q: scheme must be http or https", b)
		}
		if u.Host == "" {
			return fmt.Errorf("backend %q: missing host", b)
		}
	}
	return nil
}

func LoadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cfg Config
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}

	return &cfg, nil
}
