package config

import (
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

	return &cfg, nil
}
