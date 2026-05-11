package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	RoleSaaS = "saas"
	RoleLab  = "lab"
	RoleDev  = "dev"
)

type Config struct {
	AppRole  string         `yaml:"app_role"`
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Redis    RedisConfig    `yaml:"redis"`
	JWT      JWTConfig      `yaml:"jwt"`
}

type ServerConfig struct {
	Host                string `yaml:"host"`
	Port                int    `yaml:"port"`
	ReadTimeoutSeconds  int    `yaml:"read_timeout_seconds"`
	WriteTimeoutSeconds int    `yaml:"write_timeout_seconds"`
}

type DatabaseConfig struct {
	DSN                    string `yaml:"dsn"`
	MaxOpenConns           int    `yaml:"max_open_conns"`
	MaxIdleConns           int    `yaml:"max_idle_conns"`
	ConnMaxLifetimeMinutes int    `yaml:"conn_max_lifetime_minutes"`
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type JWTConfig struct {
	Issuer   string `yaml:"issuer"`
	Secret   string `yaml:"secret"`
	TTLHours int    `yaml:"ttl_hours"`
}

func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal([]byte(os.ExpandEnv(string(raw))), &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}

	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) validate() error {
	switch c.AppRole {
	case RoleSaaS, RoleLab, RoleDev:
	default:
		return fmt.Errorf("invalid app_role %q", c.AppRole)
	}

	if strings.TrimSpace(c.Server.Host) == "" {
		return errors.New("server.host is required")
	}
	if c.Server.Port <= 0 {
		return errors.New("server.port must be positive")
	}
	if strings.TrimSpace(c.Database.DSN) == "" {
		return errors.New("database.dsn is required")
	}
	if strings.TrimSpace(c.Redis.Addr) == "" {
		return errors.New("redis.addr is required")
	}
	if strings.TrimSpace(c.JWT.Secret) == "" {
		return errors.New("jwt.secret is required")
	}
	if strings.TrimSpace(c.JWT.Issuer) == "" {
		return errors.New("jwt.issuer is required")
	}

	return nil
}

func (c *Config) applyDefaults() {
	if c.AppRole == "" {
		c.AppRole = RoleDev
	}
	if c.Server.Host == "" {
		c.Server.Host = "0.0.0.0"
	}
	if c.Server.Port == 0 {
		c.Server.Port = 18080
	}
	if c.Server.ReadTimeoutSeconds == 0 {
		c.Server.ReadTimeoutSeconds = 15
	}
	if c.Server.WriteTimeoutSeconds == 0 {
		c.Server.WriteTimeoutSeconds = 15
	}
	if c.Database.MaxOpenConns == 0 {
		c.Database.MaxOpenConns = 10
	}
	if c.Database.MaxIdleConns == 0 {
		c.Database.MaxIdleConns = 5
	}
	if c.Database.ConnMaxLifetimeMinutes == 0 {
		c.Database.ConnMaxLifetimeMinutes = 30
	}
	if c.JWT.TTLHours == 0 {
		c.JWT.TTLHours = 72
	}

	// Local developer fallbacks keep the first slice easy to boot with a local stack.
	if c.AppRole == RoleDev {
		if strings.TrimSpace(c.Database.DSN) == "" {
			c.Database.DSN = "host=localhost port=5432 user=postgres password=postgres dbname=quantsaas sslmode=disable"
		}
		if strings.TrimSpace(c.Redis.Addr) == "" {
			c.Redis.Addr = "localhost:6379"
		}
		if strings.TrimSpace(c.JWT.Secret) == "" {
			c.JWT.Secret = "dev-secret-change-me"
		}
		if strings.TrimSpace(c.JWT.Issuer) == "" {
			c.JWT.Issuer = "quantsaas-dev"
		}
	}
}

func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}
