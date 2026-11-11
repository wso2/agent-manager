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

// Package inhouse is the agent manager's secret store: values are encrypted with
// the in-house vault and kept in the agent manager database, and each secret is
// published as an OpenChoreo SecretReference that the in-house ClusterSecretStore
// resolves through the agent manager's internal API.
package inhouse

import (
	"errors"

	secretmanagersvc "github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc/vault"
)

// ProviderName is the name of the in-house secret store provider.
const ProviderName = "inhouse"

// Provider implements secretmanagersvc.Provider for the in-house secret store.
type Provider struct{}

var _ secretmanagersvc.Provider = (*Provider)(nil)

// NewProvider creates the in-house secret store provider.
func NewProvider() secretmanagersvc.Provider {
	return &Provider{}
}

// Capabilities returns the provider's capabilities.
func (p *Provider) Capabilities() secretmanagersvc.StoreCapabilities {
	return secretmanagersvc.StoreCapabilityReadWrite
}

// NewClient creates a SecretsClient backed by the agent manager database.
func (p *Provider) NewClient(config *secretmanagersvc.StoreConfig) (secretmanagersvc.SecretsClient, error) {
	if err := p.ValidateConfig(config); err != nil {
		return nil, err
	}
	return NewClient(config.InHouse)
}

// ValidateConfig validates the in-house provider configuration.
func (p *Provider) ValidateConfig(config *secretmanagersvc.StoreConfig) error {
	if config == nil {
		return errors.New("config is required")
	}
	if config.InHouse == nil {
		return errors.New("in-house secret store config is required")
	}
	return nil
}

// NewClient creates the in-house secrets client. It is exported so the
// internal API can resolve values with the same client the services write with.
func NewClient(cfg *secretmanagersvc.InHouseConfig) (*Client, error) {
	if cfg == nil {
		return nil, errors.New("in-house secret store config is required")
	}
	if cfg.DB == nil {
		return nil, errors.New("database is required")
	}
	v, err := vault.NewInHouseVault(cfg.EncryptionKey)
	if err != nil {
		return nil, err
	}
	maxBytes := cfg.MaxSecretBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxSecretBytes
	}
	return &Client{db: cfg.DB, vault: v, maxBytes: maxBytes}, nil
}
