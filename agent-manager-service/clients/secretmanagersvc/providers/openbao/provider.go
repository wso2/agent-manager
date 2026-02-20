package openbao

import (
	"context"
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
func (p *Provider) NewClient(ctx context.Context, config *secretmanagersvc.StoreConfig) (secretmanagersvc.SecretsClient, error) {
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
