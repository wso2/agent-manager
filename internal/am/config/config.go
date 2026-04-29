package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Path            string              `yaml:"-"`
	CurrentInstance string              `yaml:"current_instance"`
	Instances       map[string]Instance `yaml:"instances"`
}

type Instance struct {
	URL        string     `yaml:"url"`
	TokenURL   string     `yaml:"token_url"`
	CurrentOrg string     `yaml:"current_org,omitempty"`
	Auth       AuthConfig `yaml:"auth,omitempty"`
}

type AuthConfig struct {
	ClientID     string    `yaml:"client_id,omitempty"`
	ClientSecret string    `yaml:"client_secret,omitempty"`
	AccessToken  string    `yaml:"access_token,omitempty"`
	RefreshToken string    `yaml:"refresh_token,omitempty"`
	ExpiresAt    time.Time `yaml:"expires_at,omitempty"`
}

func (c *Config) Current() (*Instance, error) {
	if c.CurrentInstance == "" {
		return nil, fmt.Errorf("no instance selected")
	}
	instance, ok := c.Instances[c.CurrentInstance]
	if !ok {
		return nil, fmt.Errorf("current instance %q not found in config", c.CurrentInstance)
	}
	return &instance, nil
}

func (c *Config) AddInstance(name string, inst Instance) {
	if c.Instances == nil {
		c.Instances = map[string]Instance{}
	}
	c.Instances[name] = inst
	c.CurrentInstance = name
}

func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &Config{Path: path}, nil
		}
		return nil, fmt.Errorf("open config %s: %w", path, err)
	}
	defer f.Close()

	var cfg Config
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode config %s: %w", path, err)
	}
	cfg.Path = path
	return &cfg, nil
}

func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".am", "config"), nil
}

func (c *Config) Save() error {
	return Save(c.Path, *c)
}

func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit config: %w", err)
	}
	return nil
}
