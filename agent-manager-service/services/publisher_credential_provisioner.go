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
	"errors"
	"fmt"
	"log/slog"

	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

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

	// IsThunderMode returns true when Thunder is configured for multi-tenant
	// credential provisioning, false for static single-tenant mode.
	IsThunderMode() bool
}

// staticPublisherCredentialProvisioner returns hardcoded static credentials
// when Thunder is not configured (on-prem single-tenant mode).
type staticPublisherCredentialProvisioner struct {
	creds *PublisherCredentials
}

func (s *staticPublisherCredentialProvisioner) EnsureCredentials(_ context.Context, _, _ string) (*PublisherCredentials, error) {
	return s.creds, nil
}

func (s *staticPublisherCredentialProvisioner) IsThunderMode() bool { return false }

// publisherCredentialProvisioner provisions per-org credentials via Thunder + SecretManagementClient.
type publisherCredentialProvisioner struct {
	thunderClient thundersvc.ThunderClient
	secretClient  secretmanagersvc.SecretManagementClient
	ocClient      client.OpenChoreoClient
	credRepo      repositories.OrgPublisherCredentialRepository
	logger        *slog.Logger

	sfg singleflight.Group // serializes provisioning per orgName
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
	}, nil
}

func (p *publisherCredentialProvisioner) IsThunderMode() bool { return true }

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

	return "", "", fmt.Errorf("SecretReference %s has no \"client-secret\" data source (found %d other sources)",
		secretRefName, len(ref.Data))
}

// EnsureCredentials provisions per-org publisher credentials.
// Uses singleflight to deduplicate concurrent provisioning calls for the same org.
func (p *publisherCredentialProvisioner) EnsureCredentials(ctx context.Context, orgName, orgUUID string) (*PublisherCredentials, error) {
	p.logger.Debug("EnsureCredentials called", "orgName", orgName, "orgUUID", orgUUID)

	// Singleflight ensures only one goroutine provisions per org at a time.
	// NOTE: uses the first caller's context — if cancelled, other waiters also get the error.
	// Acceptable because provisioning is rare (once per org) and callers can retry.
	result, err, _ := p.sfg.Do(orgName, func() (any, error) {
		return p.provisionCredentials(ctx, orgName, orgUUID)
	})
	if err != nil {
		return nil, err
	}
	return result.(*PublisherCredentials), nil
}

// provisionCredentials performs the DB lookup and, if needed, the full Thunder provisioning flow.
func (p *publisherCredentialProvisioner) provisionCredentials(ctx context.Context, orgName, orgUUID string) (*PublisherCredentials, error) {
	// Check DB for existing credentials
	existing, err := p.credRepo.GetByOrgName(orgName)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to look up publisher credentials for org %s: %w", orgName, err)
	}
	if err == nil && existing != nil {
		p.logger.Debug("Found existing publisher credentials in DB",
			"orgName", orgName, "clientID", existing.ClientID)

		return &PublisherCredentials{
			ClientID:     existing.ClientID,
			SecretKVPath: existing.SecretKVPath,
			SecretKey:    existing.SecretKey,
		}, nil
	}

	p.logger.Info("No existing credentials, provisioning via Thunder", "orgName", orgName)

	// Not found — create Thunder OAuth app
	clientID, clientSecret, created, err := p.thunderClient.EnsurePublisherApp(ctx, orgName, orgUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to provision Thunder app for org %s: %w", orgName, err)
	}
	p.logger.Info("Thunder EnsurePublisherApp result",
		"orgName", orgName, "clientID", clientID, "created", created, "hasSecret", clientSecret != "")

	// If app already existed in Thunder but not in DB, clientSecret is empty
	// (Thunder only returns the secret at creation time). Regenerate the client
	// secret rather than deleting the whole app — this avoids invalidating any
	// tokens that may have been issued from the existing app's prior secret.
	if !created && clientSecret == "" {
		p.logger.Warn("Thunder app exists but secret not available — regenerating client secret",
			"orgName", orgName, "clientID", clientID)

		clientSecret, err = p.thunderClient.RegenerateClientSecret(ctx, orgName)
		if err != nil {
			return nil, fmt.Errorf("failed to regenerate client secret for org %s: %w", orgName, err)
		}
		p.logger.Info("Regenerated Thunder client secret",
			"orgName", orgName, "clientID", clientID)
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
	resolvedKVPath, resolvedKey, resolveErr := p.resolveSecretRef(ctx, orgName, secretRefName)
	if resolveErr != nil {
		return nil, fmt.Errorf("failed to resolve SecretReference for org %s: %w", orgName, resolveErr)
	}

	// Save to DB — treat as fatal since we just provisioned real credentials
	dbCred := &models.OrgPublisherCredential{
		OrgName:      orgName,
		OrgUUID:      orgUUID,
		ClientID:     clientID,
		SecretKVPath: resolvedKVPath,
		SecretKey:    resolvedKey,
	}
	if dbErr := p.credRepo.Upsert(dbCred); dbErr != nil {
		return nil, fmt.Errorf("failed to persist publisher credentials for org %s: %w", orgName, dbErr)
	}

	p.logger.Info("Provisioned new publisher credentials",
		"orgName", orgName, "clientID", clientID, "kvPath", resolvedKVPath, "secretKey", resolvedKey)

	return &PublisherCredentials{
		ClientID:     clientID,
		SecretKVPath: resolvedKVPath,
		SecretKey:    resolvedKey,
	}, nil
}
