// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

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
	URL              string     `yaml:"url"`
	TokenURL         string     `yaml:"token_url"`
	AuthorizationURL string     `yaml:"authorization_url,omitempty"`
	CurrentOrg       string     `yaml:"current_org,omitempty"`
	Auth             AuthConfig `yaml:"auth,omitempty"`
}

type AuthConfig struct {
	GrantType    string    `yaml:"grant_type,omitempty"`
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
	return filepath.Join(home, ".amctl", "config"), nil
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

	tmpFile, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmp := tmpFile.Name()
	defer os.Remove(tmp)
	if err := os.Chmod(tmp, 0600); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("chmod temp config: %w", err)
	}
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write config: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit config: %w", err)
	}
	return nil
}
