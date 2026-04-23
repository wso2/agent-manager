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

package services

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// PublisherCredentials holds the provisioned OAuth2 credentials for publishing scores.
type PublisherCredentials struct {
	ClientID     string // OAuth2 client ID (becomes JWT subject)
	SecretKVPath string // KV path in the secret store (remoteRef.key for ExternalSecret)
	SecretKey    string // Key within the KV secret (remoteRef.property for ExternalSecret)
}

// PublisherCredentialProvisioner provisions per-org publisher credentials.
type PublisherCredentialProvisioner interface {
	// EnsureCredentials provisions per-org publisher credentials.
	// orgUUID is the Thunder organization unit UUID (from JWT ouId claim).
	// If empty, the default OU is used.
	EnsureCredentials(ctx context.Context, orgName, orgUUID string) (*PublisherCredentials, error)
}

// staticPublisherCredentialProvisioner returns hardcoded static credentials
// when Thunder is not configured (on-prem single-tenant mode).
type staticPublisherCredentialProvisioner struct {
	creds *PublisherCredentials
}

func (s *staticPublisherCredentialProvisioner) EnsureCredentials(_ context.Context, _, _ string) (*PublisherCredentials, error) {
	return s.creds, nil
}

// publisherCredentialProvisioner provisions per-org credentials via Thunder + SecretManagementClient.
type publisherCredentialProvisioner struct {
	thunderClient thundersvc.ThunderClient
	secretClient  secretmanagersvc.SecretManagementClient
	ocClient      client.OpenChoreoClient
	credRepo      repositories.OrgPublisherCredentialRepository
	logger        *slog.Logger

	mu    sync.RWMutex
	cache map[string]*PublisherCredentials // orgName -> creds
}

// NewPublisherCredentialProvisioner creates a provisioner.
// If Thunder is not configured (BaseURL empty), returns a static provisioner
// that always returns the default amp-publisher-client credentials.
func NewPublisherCredentialProvisioner(
	cfg config.Config,
	logger *slog.Logger,
	secretClient secretmanagersvc.SecretManagementClient,
	ocClient client.OpenChoreoClient,
	credRepo repositories.OrgPublisherCredentialRepository,
) (PublisherCredentialProvisioner, error) {
	if cfg.Thunder.BaseURL == "" {
		logger.Info("Thunder not configured, using static publisher credentials")
		return &staticPublisherCredentialProvisioner{
			creds: &PublisherCredentials{
				ClientID:     "amp-publisher-client",
				SecretKVPath: "amp-publisher-client-secret",
				SecretKey:    "value",
			},
		}, nil
	}

	thunderCl := thundersvc.NewThunderClient(
		cfg.Thunder.BaseURL,
		cfg.Thunder.ClientID,
		cfg.Thunder.ClientSecret,
	)

	logger.Info("Publisher credential provisioner initialized with Thunder",
		"thunderBaseURL", cfg.Thunder.BaseURL,
	)

	return &publisherCredentialProvisioner{
		thunderClient: thunderCl,
		secretClient:  secretClient,
		ocClient:      ocClient,
		credRepo:      credRepo,
		logger:        logger,
		cache:         make(map[string]*PublisherCredentials),
	}, nil
}

// publisherSecretLocation builds the SecretLocation for publisher credentials.
func publisherSecretLocation(orgName string) secretmanagersvc.SecretLocation {
	return secretmanagersvc.SecretLocation{
		OrgName:    orgName,
		EntityName: "amp-publisher-" + orgName,
	}
}

// resolveSecretRef fetches the SecretReference via OpenChoreo and extracts
// the remoteRef key and property for the "client-secret" data source.
func (p *publisherCredentialProvisioner) resolveSecretRef(ctx context.Context, orgName, secretRefName string) (kvPath, secretKey string, err error) {
	p.logger.Info("Resolving SecretReference from OpenChoreo",
		"orgName", orgName, "secretRefName", secretRefName)

	ref, err := p.ocClient.GetSecretReference(ctx, orgName, secretRefName)
	if err != nil {
		return "", "", fmt.Errorf("failed to get SecretReference %s: %w", secretRefName, err)
	}

	p.logger.Info("SecretReference fetched",
		"orgName", orgName, "secretRefName", secretRefName, "dataSources", len(ref.Data))

	for _, ds := range ref.Data {
		if ds.SecretKey == "client-secret" {
			return ds.RemoteRef.Key, ds.RemoteRef.Property, nil
		}
	}

	if len(ref.Data) > 0 {
		return ref.Data[0].RemoteRef.Key, ref.Data[0].RemoteRef.Property, nil
	}

	return "", "", fmt.Errorf("SecretReference %s has no data sources", secretRefName)
}

// EnsureCredentials provisions per-org publisher credentials.
func (p *publisherCredentialProvisioner) EnsureCredentials(ctx context.Context, orgName, orgUUID string) (*PublisherCredentials, error) {
	p.logger.Info("EnsureCredentials called", "orgName", orgName, "orgUUID", orgUUID)

	// Fast path: check in-memory cache
	p.mu.RLock()
	if creds, ok := p.cache[orgName]; ok {
		p.mu.RUnlock()
		p.logger.Debug("Returning cached publisher credentials", "orgName", orgName)
		return creds, nil
	}
	p.mu.RUnlock()

	// Check DB for existing credentials
	existing, err := p.credRepo.GetByOrgName(orgName)
	if err == nil && existing != nil {
		p.logger.Info("Found existing publisher credentials in DB",
			"orgName", orgName, "clientID", existing.ClientID)

		creds := &PublisherCredentials{
			ClientID:     existing.ClientID,
			SecretKVPath: existing.SecretKVPath,
			SecretKey:    existing.SecretKey,
		}
		p.mu.Lock()
		p.cache[orgName] = creds
		p.mu.Unlock()
		return creds, nil
	}

	p.logger.Info("No existing credentials, provisioning via Thunder", "orgName", orgName)

	// Not found — create Thunder OAuth app
	clientID, clientSecret, created, err := p.thunderClient.EnsurePublisherApp(orgName, orgUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to provision Thunder app for org %s: %w", orgName, err)
	}
	p.logger.Info("Thunder EnsurePublisherApp result",
		"orgName", orgName, "clientID", clientID, "created", created, "hasSecret", clientSecret != "")

	// If app already existed in Thunder but not in DB, clientSecret is empty.
	// Delete and recreate.
	if !created && clientSecret == "" {
		p.logger.Warn("Thunder app exists but secret lost — deleting and recreating",
			"orgName", orgName, "clientID", clientID)

		if _, delErr := p.thunderClient.DeletePublisherApp(orgName); delErr != nil {
			return nil, fmt.Errorf("failed to delete stale Thunder app for org %s: %w", orgName, delErr)
		}

		clientID, clientSecret, _, err = p.thunderClient.EnsurePublisherApp(orgName, orgUUID)
		if err != nil {
			return nil, fmt.Errorf("failed to re-provision Thunder app for org %s: %w", orgName, err)
		}
		p.logger.Info("Re-created Thunder app",
			"orgName", orgName, "clientID", clientID, "hasSecret", clientSecret != "")
	}

	if clientSecret == "" {
		return nil, fmt.Errorf("failed to provision publisher credentials for org %s: no client secret available", orgName)
	}

	// Store secret via SecretManagementClient (creates KV entry + SecretReference CR)
	location := publisherSecretLocation(orgName)
	secretData := map[string]string{
		"client-id":     clientID,
		"client-secret": clientSecret,
	}

	secretRefName, createErr := p.secretClient.CreateSecret(ctx, location, secretData)
	if createErr != nil {
		return nil, fmt.Errorf("failed to store publisher secret for org %s: %w", orgName, createErr)
	}
	p.logger.Info("Secret stored successfully",
		"orgName", orgName, "secretRefName", secretRefName)

	// Resolve the SecretReference from OpenChoreo to get the actual remoteRef key/property
	kvPath, err := location.KVPath()
	if err != nil {
		return nil, fmt.Errorf("invalid secret location for org %s: %w", orgName, err)
	}

	resolvedKVPath, resolvedKey, resolveErr := p.resolveSecretRef(ctx, orgName, secretRefName)
	if resolveErr != nil {
		p.logger.Warn("Failed to resolve SecretReference, using computed values",
			"orgName", orgName, "error", resolveErr)
		resolvedKVPath = kvPath
		resolvedKey = "client-secret"
	}

	// Save to DB
	dbCred := &models.OrgPublisherCredential{
		OrgName:      orgName,
		OrgUUID:      orgUUID,
		ClientID:     clientID,
		SecretKVPath: resolvedKVPath,
		SecretKey:    resolvedKey,
	}
	if dbErr := p.credRepo.Upsert(dbCred); dbErr != nil {
		p.logger.Error("Failed to save publisher credentials to DB (non-fatal)",
			"orgName", orgName, "error", dbErr)
	}

	p.logger.Info("Provisioned new publisher credentials",
		"orgName", orgName, "clientID", clientID, "kvPath", resolvedKVPath, "secretKey", resolvedKey)

	creds := &PublisherCredentials{
		ClientID:     clientID,
		SecretKVPath: resolvedKVPath,
		SecretKey:    resolvedKey,
	}
	p.mu.Lock()
	p.cache[orgName] = creds
	p.mu.Unlock()

	return creds, nil
}
