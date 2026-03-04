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

package openbao

import (
	"errors"

	vault "github.com/hashicorp/vault/api"

	secretmanagersvc "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/secretmanagersvc"
)

const (
	// ProviderName is the name used to register this provider.
	ProviderName = "openbao"

	// DefaultVersion is the default KV secrets engine version.
	DefaultVersion = "v2"

	// ManagedByValue is the value used for the managed-by metadata.
	ManagedByValue = "amp-secret-manager"
)

// Provider implements the secretmanagersvc.Provider interface for OpenBao/Vault.
type Provider struct{}

// Ensure Provider implements the interface.
var _ secretmanagersvc.Provider = &Provider{}

// NewProvider creates a new OpenBao provider instance.
func NewProvider() secretmanagersvc.Provider {
	return &Provider{}
}

// Capabilities returns the provider's capabilities.
func (p *Provider) Capabilities() secretmanagersvc.StoreCapabilities {
	return secretmanagersvc.StoreCapabilityReadWrite
}

// NewClient creates a new SecretsClient for OpenBao.
func (p *Provider) NewClient(config *secretmanagersvc.StoreConfig) (secretmanagersvc.SecretsClient, error) {
	if err := p.ValidateConfig(config); err != nil {
		return nil, err
	}

	cfg := vault.DefaultConfig()
	cfg.Address = config.OpenBao.Server

	vaultClient, err := vault.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	vaultClient.SetToken(config.OpenBao.Auth.Token)

	version := config.OpenBao.Version
	if version == "" {
		version = DefaultVersion
	}

	return &Client{
		client:  vaultClient,
		path:    config.OpenBao.Path,
		version: version,
	}, nil
}

// ValidateConfig validates the OpenBao configuration.
func (p *Provider) ValidateConfig(config *secretmanagersvc.StoreConfig) error {
	if config == nil {
		return errors.New("config is required")
	}
	if config.OpenBao == nil {
		return errors.New("openbao config is required")
	}
	if config.OpenBao.Server == "" {
		return errors.New("openbao server is required")
	}
	if config.OpenBao.Path == "" {
		return errors.New("openbao path is required")
	}
	if config.OpenBao.Auth == nil {
		return errors.New("openbao auth is required")
	}
	if config.OpenBao.Auth.Token == "" {
		return errors.New("openbao auth token is required")
	}
	return nil
}

func init() {
	secretmanagersvc.Register(ProviderName, NewProvider())
}
