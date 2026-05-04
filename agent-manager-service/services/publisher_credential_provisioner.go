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
	"sync"

	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

	ocauth "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/auth"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const schedulerRoleName = "amp-monitor-scheduler"

// ErrNotThunderMode is returned by GetOCClientForOrg when the provisioner is not in Thunder mode.
var ErrNotThunderMode = errors.New("not in Thunder mode")

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
	EnsureCredentials(ctx context.Context, orgName, orgUUID string) (*PublisherCredentials, error)

	// IsThunderMode returns true when Thunder is configured for multi-tenant
	// credential provisioning, false for static single-tenant mode.
	IsThunderMode() bool

	// GetOCClientForOrg returns an OC client authenticated with the org's publisher app token.
	// Used by the scheduler which runs without a user request context and therefore has no
	// user JWT in ctx. Decrypts the stored client secret and exchanges it for an access token
	// via the IDP token endpoint.
	// In non-Thunder mode returns nil, ErrNotThunderMode — callers must fall back to the system OC client.
	GetOCClientForOrg(ctx context.Context, orgName string) (client.OpenChoreoClient, error)
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

func (s *staticPublisherCredentialProvisioner) GetOCClientForOrg(_ context.Context, _ string) (client.OpenChoreoClient, error) {
	return nil, ErrNotThunderMode
}

// NewStaticPublisherCredentialProvisioner creates a static provisioner for use in tests.
func NewStaticPublisherCredentialProvisioner() PublisherCredentialProvisioner {
	return &staticPublisherCredentialProvisioner{
		creds: &PublisherCredentials{
			ClientID:     "amp-publisher-client",
			SecretKVPath: "amp-publisher-client-secret",
			SecretKey:    "value",
		},
	}
}

// publisherCredentialProvisioner provisions per-org credentials via Thunder + SecretManagementClient.
type publisherCredentialProvisioner struct {
	thunderClient thundersvc.ThunderClient
	secretClient  secretmanagersvc.SecretManagementClient
	ocClient      client.OpenChoreoClient
	credRepo      repositories.OrgPublisherCredentialRepository
	logger        *slog.Logger
	encryptionKey []byte
	idpTokenURL   string
	ocBaseURL     string

	sfg singleflight.Group // serializes provisioning per orgName

	// per-org auth providers for publisher tokens — created once, cache tokens internally
	orgAuthMu        sync.RWMutex
	orgAuthProviders map[string]client.AuthProvider
}

// NewPublisherCredentialProvisioner creates a provisioner.
// If Thunder is not configured (BaseURL empty), returns a static provisioner
// that always returns the default amp-publisher-client credentials.
func NewPublisherCredentialProvisioner(
	cfg config.Config,
	encryptionKey []byte,
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
		thunderClient:    thunderCl,
		secretClient:     secretClient,
		ocClient:         ocClient,
		credRepo:         credRepo,
		logger:           logger,
		encryptionKey:    encryptionKey,
		idpTokenURL:      cfg.IDP.TokenURL,
		ocBaseURL:        cfg.OpenChoreo.BaseURL,
		orgAuthProviders: make(map[string]client.AuthProvider),
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

	result, err, _ := p.sfg.Do("provision:"+orgName, func() (any, error) {
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

		// Ensure the binding exists — idempotent, handles orgs provisioned before this was added.
		// Non-fatal: log and continue if the ClusterAuthzRole isn't installed yet.
		if bindErr := p.ocClient.EnsureClusterRoleBinding(ctx, existing.ClientID, schedulerRoleName); bindErr != nil {
			p.logger.Warn("Failed to ensure ClusterAuthzRoleBinding for existing credentials",
				"orgName", orgName, "clientID", existing.ClientID, "error", bindErr)
		}

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

	// If app already existed in Thunder but not in DB, clientSecret is empty.
	// Regenerate rather than deleting the whole app.
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

	// Encrypt the client secret so the scheduler can decrypt and use it for token generation.
	encryptedSecret, err := utils.EncryptBytes([]byte(clientSecret), p.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt publisher secret for org %s: %w", orgName, err)
	}

	// Bind the publisher app to the scheduler role in OpenChoreo so it can create/track WorkflowRuns.
	// Uses the system OC client (not org-bound) — ClusterAuthzRoleBindings are cluster-scoped resources.
	if bindErr := p.ocClient.EnsureClusterRoleBinding(ctx, clientID, schedulerRoleName); bindErr != nil {
		return nil, fmt.Errorf("failed to bind publisher app to scheduler role for org %s: %w", orgName, bindErr)
	}
	p.logger.Info("ClusterAuthzRoleBinding ensured",
		"orgName", orgName, "clientID", clientID, "role", schedulerRoleName)

	// Save to DB — treat as fatal since we just provisioned real credentials
	dbCred := &models.OrgPublisherCredential{
		OrgName:               orgName,
		OrgUUID:               orgUUID,
		ClientID:              clientID,
		SecretKVPath:          resolvedKVPath,
		SecretKey:             resolvedKey,
		ClientSecretEncrypted: encryptedSecret,
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

// GetOCClientForOrg returns an OC client authenticated with the publisher app's org-scoped token.
// Used by the scheduler for CreateWorkflowRun and GetWorkflowRun — operations that run
// without a live user request context.
func (p *publisherCredentialProvisioner) GetOCClientForOrg(ctx context.Context, orgName string) (client.OpenChoreoClient, error) {
	authProv, err := p.getOrCreateOrgAuthProvider(ctx, orgName)
	if err != nil {
		return nil, err
	}
	return client.NewOpenChoreoClient(&client.Config{
		BaseURL:      p.ocBaseURL,
		AuthProvider: authProv,
	})
}

// getOrCreateOrgAuthProvider returns (or lazily creates) a cached auth provider for the org.
// singleflight deduplicates concurrent calls for the same org; the write lock prevents a
// TOCTOU race between the cache miss check and the map write.
func (p *publisherCredentialProvisioner) getOrCreateOrgAuthProvider(ctx context.Context, orgName string) (client.AuthProvider, error) {
	p.orgAuthMu.RLock()
	authProv, ok := p.orgAuthProviders[orgName]
	p.orgAuthMu.RUnlock()
	if ok {
		return authProv, nil
	}

	result, err, _ := p.sfg.Do("auth:"+orgName, func() (any, error) {
		// Re-check under write lock — another goroutine may have populated the cache
		// while we were waiting on singleflight.
		p.orgAuthMu.Lock()
		defer p.orgAuthMu.Unlock()
		if ap, ok := p.orgAuthProviders[orgName]; ok {
			return ap, nil
		}

		cred, err := p.credRepo.GetByOrgName(orgName)
		if err != nil {
			return nil, fmt.Errorf("no publisher credentials for org %s: %w", orgName, err)
		}
		if len(cred.ClientSecretEncrypted) == 0 {
			return nil, fmt.Errorf("no encrypted secret stored for org %s — call EnsureCredentials first", orgName)
		}

		secretBytes, err := utils.DecryptBytes(cred.ClientSecretEncrypted, p.encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt publisher secret for org %s: %w", orgName, err)
		}

		ap := ocauth.NewAuthProvider(ocauth.Config{
			TokenURL:     p.idpTokenURL,
			ClientID:     cred.ClientID,
			ClientSecret: string(secretBytes),
		})
		p.orgAuthProviders[orgName] = ap
		p.logger.Debug("Created org auth provider", "orgName", orgName, "clientID", cred.ClientID)
		return ap, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(client.AuthProvider), nil
}
