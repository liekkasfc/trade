package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	SaaSURL  string         `yaml:"saas_url"`
	Email    string         `yaml:"email"`
	Password string         `yaml:"password"`
	Exchange ExchangeConfig `yaml:"exchange"`
}

type ExchangeConfig struct {
	Name       string `yaml:"name"`
	APIKey     string `yaml:"api_key"`
	SecretKey  string `yaml:"secret_key"`
	Passphrase string `yaml:"passphrase"`
	Sandbox    bool   `yaml:"sandbox"`
}

func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	if strings.TrimSpace(c.SaaSURL) == "" {
		return errors.New("saas_url is required")
	}
	if strings.TrimSpace(c.Email) == "" {
		return errors.New("email is required")
	}
	if strings.TrimSpace(c.Password) == "" {
		return errors.New("password is required")
	}
	if strings.TrimSpace(c.Exchange.Name) == "" {
		return errors.New("exchange.name is required")
	}
	if strings.TrimSpace(c.Exchange.APIKey) == "" {
		return errors.New("exchange.api_key is required")
	}
	if strings.TrimSpace(c.Exchange.SecretKey) == "" {
		return errors.New("exchange.secret_key is required")
	}
	if strings.TrimSpace(c.Exchange.Passphrase) == "" {
		return errors.New("exchange.passphrase is required")
	}
	return nil
}
