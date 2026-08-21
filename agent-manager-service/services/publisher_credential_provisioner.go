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

// ErrPublisherCredentialNotFound indicates EnsureCredentials has not yet been called for the org.
// Distinct from real DB errors so callers can decide whether to provision-on-demand vs retry.
var ErrPublisherCredentialNotFound = errors.New("publisher credentials not found")

// ErrSchedulerCredentialNotFound means a scheduler credential still wasn't found right after provisioning it.
var ErrSchedulerCredentialNotFound = errors.New("scheduler credentials not found")

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
	EnsureCredentials(ctx context.Context, ouID, orgUUID string) (*PublisherCredentials, error)

	// IsThunderMode returns true when Thunder is configured for multi-tenant
	// credential provisioning, false for static single-tenant mode.
	IsThunderMode() bool

	// GetOCClientForOrg returns an OC client authenticated with the org's publisher app token.
	// Used by the scheduler which runs without a user request context and therefore has no
	// user JWT in ctx. Decrypts the stored client secret and exchanges it for an access token
	// via the IDP token endpoint.
	// In non-Thunder mode returns nil, ErrNotThunderMode — callers must fall back to the system OC client.
	GetOCClientForOrg(ctx context.Context, ouID string) (client.OpenChoreoClient, error)
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
	thunderClient     thundersvc.ThunderClient
	secretClient      secretmanagersvc.SecretManagementClient
	ocClient          client.OpenChoreoClient
	credRepo          repositories.OrgPublisherCredentialRepository
	schedulerCredRepo repositories.OrgSchedulerCredentialRepository
	logger            *slog.Logger
	encryptionKey     []byte
	idpTokenURL       string
	ocBaseURL         string

	sfg singleflight.Group // serializes provisioning and per-org client construction

	// orgOCClients caches per-org OpenChoreoClients so that the underlying http.Client
	// connection pool and the wrapped AuthProvider's token cache are reused across
	// scheduler cycles. Singleflight serializes builders; the lock guards map access only.
	orgOCMu      sync.RWMutex
	orgOCClients map[string]client.OpenChoreoClient
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
	schedulerCredRepo repositories.OrgSchedulerCredentialRepository,
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

	var thunderCl thundersvc.ThunderClient
	if cfg.Thunder.ResolveToHost != "" {
		thunderCl = thundersvc.NewThunderClientWithDialOverride(
			cfg.Thunder.BaseURL,
			cfg.Thunder.ClientID,
			cfg.Thunder.ClientSecret,
			cfg.Thunder.ResolveToHost,
			thundersvc.SystemResourceIdentifier(cfg.Thunder.BaseURL),
		)
	} else {
		thunderCl = thundersvc.NewThunderClient(
			cfg.Thunder.BaseURL,
			cfg.Thunder.ClientID,
			cfg.Thunder.ClientSecret,
		)
	}

	logger.Info(
		"Publisher credential provisioner initialized with Thunder",
		"thunder_base_url", cfg.Thunder.BaseURL,
	)

	return &publisherCredentialProvisioner{
		thunderClient:     thunderCl,
		secretClient:      secretClient,
		ocClient:          ocClient,
		credRepo:          credRepo,
		schedulerCredRepo: schedulerCredRepo,
		logger:            logger,
		encryptionKey:     encryptionKey,
		idpTokenURL:       cfg.IDP.TokenURL,
		ocBaseURL:         cfg.OpenChoreo.BaseURL,
		orgOCClients:      make(map[string]client.OpenChoreoClient),
	}, nil
}

func (p *publisherCredentialProvisioner) IsThunderMode() bool { return true }

// publisherSecretLocation builds the SecretLocation for publisher credentials.
func publisherSecretLocation(ouID string) secretmanagersvc.SecretLocation {
	return secretmanagersvc.SecretLocation{
		OrgName:    ouID,
		EntityName: "amp-publisher-" + ouID,
	}
}

// schedulerSecretLocation builds the SecretLocation for scheduler-only credentials.
func schedulerSecretLocation(ouID string) secretmanagersvc.SecretLocation {
	return secretmanagersvc.SecretLocation{
		OrgName:    ouID,
		EntityName: "amp-scheduler-" + ouID,
	}
}

// resolveSecretRef fetches the SecretReference via OpenChoreo and extracts
// the remoteRef key and property for the "client-secret" data source.
func (p *publisherCredentialProvisioner) resolveSecretRef(ctx context.Context, ouID, secretRefName string) (kvPath, secretKey string, err error) {
	p.logger.Info("Resolving SecretReference from OpenChoreo",
		"ou_id", ouID, "secret_ref_name", secretRefName)

	ref, err := p.ocClient.GetSecretReference(ctx, ouID, secretRefName)
	if err != nil {
		return "", "", fmt.Errorf("failed to get SecretReference %s: %w", secretRefName, err)
	}

	p.logger.Info("SecretReference fetched",
		"ou_id", ouID, "secret_ref_name", secretRefName, "data_sources", len(ref.Data))

	for _, ds := range ref.Data {
		if ds.SecretKey == "client-secret" {
			return ds.RemoteRef.Key, ds.RemoteRef.Property, nil
		}
	}

	return "", "", fmt.Errorf("SecretReference %s has no \"client-secret\" data source (found %d other sources)",
		secretRefName, len(ref.Data))
}

// EnsureCredentials provisions per-org publisher and scheduler credentials.
// Uses singleflight to deduplicate concurrent provisioning calls for the same org.
func (p *publisherCredentialProvisioner) EnsureCredentials(ctx context.Context, ouID, orgUUID string) (*PublisherCredentials, error) {
	p.logger.Debug("EnsureCredentials called", "ou_id", ouID, "org_uuid", orgUUID)

	result, err, _ := p.sfg.Do("provision:"+ouID, func() (any, error) {
		pubCreds, err := p.provisionPublisherCredentials(ctx, ouID, orgUUID)
		if err != nil {
			return nil, err
		}
		if err := p.provisionSchedulerCredentials(ctx, ouID, orgUUID); err != nil {
			return nil, fmt.Errorf("failed to provision scheduler credentials for org %s: %w", ouID, err)
		}
		return pubCreds, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*PublisherCredentials), nil
}

// provisionPublisherCredentials provisions the eval-job pod's credential — bound to no OpenChoreo role.
func (p *publisherCredentialProvisioner) provisionPublisherCredentials(ctx context.Context, ouID, orgUUID string) (*PublisherCredentials, error) {
	// Check DB for existing credentials
	existing, err := p.credRepo.GetByOrgName(ouID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("failed to look up publisher credentials for org %s: %w", ouID, err)
		}
		// ErrRecordNotFound: no credentials yet, fall through to provision.
	} else {
		p.logger.Debug("Found existing publisher credentials in DB",
			"ou_id", ouID, "client_id", existing.ClientID)

		return &PublisherCredentials{
			ClientID:     existing.ClientID,
			SecretKVPath: existing.SecretKVPath,
			SecretKey:    existing.SecretKey,
		}, nil
	}

	p.logger.Info("No existing credentials, provisioning via Thunder", "ou_id", ouID)

	// Not found — create Thunder OAuth app
	clientID, clientSecret, created, err := p.thunderClient.EnsurePublisherApp(ctx, ouID, orgUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to provision Thunder app for org %s: %w", ouID, err)
	}
	p.logger.Info("Thunder EnsurePublisherApp result",
		"ou_id", ouID, "client_id", clientID, "created", created, "has_secret", clientSecret != "")

	// If app already existed in Thunder but not in DB, clientSecret is empty.
	// Regenerate rather than deleting the whole app.
	if !created && clientSecret == "" {
		p.logger.Warn("Thunder app exists but secret not available — regenerating client secret",
			"ou_id", ouID, "client_id", clientID)

		clientSecret, err = p.thunderClient.RegenerateClientSecret(ctx, ouID)
		if err != nil {
			return nil, fmt.Errorf("failed to regenerate client secret for org %s: %w", ouID, err)
		}
		p.logger.Info("Regenerated Thunder client secret",
			"ou_id", ouID, "client_id", clientID)
	}

	if clientSecret == "" {
		return nil, fmt.Errorf("failed to provision publisher credentials for org %s: no client secret available", ouID)
	}

	// Store secret via SecretManagementClient (creates KV entry + SecretReference CR)
	location := publisherSecretLocation(ouID)
	secretData := map[string]string{
		"client-id":     clientID,
		"client-secret": clientSecret,
	}

	secretRefName, createErr := p.secretClient.CreateSecret(ctx, location, secretData)
	if createErr != nil {
		return nil, fmt.Errorf("failed to store publisher secret for org %s: %w", ouID, createErr)
	}
	p.logger.Info("Secret stored successfully",
		"ou_id", ouID, "secret_ref_name", secretRefName)

	// Resolve the SecretReference from OpenChoreo to get the actual remoteRef key/property
	resolvedKVPath, resolvedKey, resolveErr := p.resolveSecretRef(ctx, ouID, secretRefName)
	if resolveErr != nil {
		return nil, fmt.Errorf("failed to resolve SecretReference for org %s: %w", ouID, resolveErr)
	}

	// Encrypt the client secret so the scheduler can decrypt and use it for token generation.
	encryptedSecret, err := utils.EncryptBytes([]byte(clientSecret), p.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt publisher secret for org %s: %w", ouID, err)
	}

	// Save to DB — treat as fatal since we just provisioned real credentials
	dbCred := &models.OrgPublisherCredential{
		OUID:                  ouID,
		OrgUUID:               orgUUID,
		ClientID:              clientID,
		SecretKVPath:          resolvedKVPath,
		SecretKey:             resolvedKey,
		ClientSecretEncrypted: encryptedSecret,
	}
	if dbErr := p.credRepo.Upsert(dbCred); dbErr != nil {
		return nil, fmt.Errorf("failed to persist publisher credentials for org %s: %w", ouID, dbErr)
	}

	p.logger.Info("Provisioned new publisher credentials",
		"ou_id", ouID, "client_id", clientID, "kv_path", resolvedKVPath, "secret_key", resolvedKey)

	return &PublisherCredentials{
		ClientID:     clientID,
		SecretKVPath: resolvedKVPath,
		SecretKey:    resolvedKey,
	}, nil
}

// provisionSchedulerCredentials provisions the scheduler-only credential, bound to schedulerRoleName; never injected into the eval-job pod.
func (p *publisherCredentialProvisioner) provisionSchedulerCredentials(ctx context.Context, ouID, orgUUID string) error {
	existing, err := p.schedulerCredRepo.GetByOrgName(ouID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("failed to look up scheduler credentials for org %s: %w", ouID, err)
		}
		// ErrRecordNotFound: no credentials yet, fall through to provision.
	} else {
		p.logger.Debug("Found existing scheduler credentials in DB",
			"ou_id", ouID, "client_id", existing.ClientID)

		// Idempotent re-verify; non-fatal if the ClusterAuthzRole isn't installed yet.
		if bindErr := p.ocClient.EnsureClusterRoleBinding(ctx, existing.ClientID, schedulerRoleName); bindErr != nil {
			p.logger.Warn("Failed to ensure ClusterAuthzRoleBinding for existing scheduler credentials",
				"ou_id", ouID, "client_id", existing.ClientID, "error", bindErr)
		}
		return nil
	}

	p.logger.Info("No existing scheduler credentials, provisioning via Thunder", "ou_id", ouID)

	appName := "amp-scheduler-" + ouID
	clientID, clientSecret, created, err := p.thunderClient.EnsureApp(ctx, appName, orgUUID)
	if err != nil {
		return fmt.Errorf("failed to provision Thunder scheduler app for org %s: %w", ouID, err)
	}
	p.logger.Info("Thunder EnsureApp result for scheduler credential",
		"ou_id", ouID, "client_id", clientID, "created", created, "has_secret", clientSecret != "")

	if !created && clientSecret == "" {
		p.logger.Warn("Thunder scheduler app exists but secret not available — regenerating client secret",
			"ou_id", ouID, "client_id", clientID)

		clientSecret, err = p.thunderClient.RegenerateAppClientSecret(ctx, appName)
		if err != nil {
			return fmt.Errorf("failed to regenerate scheduler client secret for org %s: %w", ouID, err)
		}
		p.logger.Info("Regenerated Thunder scheduler client secret",
			"ou_id", ouID, "client_id", clientID)
	}

	if clientSecret == "" {
		return fmt.Errorf("failed to provision scheduler credentials for org %s: no client secret available", ouID)
	}

	location := schedulerSecretLocation(ouID)
	secretData := map[string]string{
		"client-id":     clientID,
		"client-secret": clientSecret,
	}

	secretRefName, createErr := p.secretClient.CreateSecret(ctx, location, secretData)
	if createErr != nil {
		return fmt.Errorf("failed to store scheduler secret for org %s: %w", ouID, createErr)
	}
	p.logger.Info("Scheduler secret stored successfully",
		"ou_id", ouID, "secret_ref_name", secretRefName)

	resolvedKVPath, resolvedKey, resolveErr := p.resolveSecretRef(ctx, ouID, secretRefName)
	if resolveErr != nil {
		return fmt.Errorf("failed to resolve SecretReference for scheduler credentials of org %s: %w", ouID, resolveErr)
	}

	encryptedSecret, err := utils.EncryptBytes([]byte(clientSecret), p.encryptionKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt scheduler secret for org %s: %w", ouID, err)
	}

	// ClusterAuthzRoleBindings are cluster-scoped; non-fatal if the role isn't installed yet.
	if bindErr := p.ocClient.EnsureClusterRoleBinding(ctx, clientID, schedulerRoleName); bindErr != nil {
		p.logger.Warn("Failed to ensure ClusterAuthzRoleBinding for new scheduler credentials",
			"ou_id", ouID, "client_id", clientID, "role", schedulerRoleName, "error", bindErr)
	} else {
		p.logger.Info("ClusterAuthzRoleBinding ensured for scheduler credential",
			"ou_id", ouID, "client_id", clientID, "role", schedulerRoleName)
	}

	dbCred := &models.OrgSchedulerCredential{
		OUID:                  ouID,
		OrgUUID:               orgUUID,
		ClientID:              clientID,
		SecretKVPath:          resolvedKVPath,
		SecretKey:             resolvedKey,
		ClientSecretEncrypted: encryptedSecret,
	}
	if dbErr := p.schedulerCredRepo.Upsert(dbCred); dbErr != nil {
		return fmt.Errorf("failed to persist scheduler credentials for org %s: %w", ouID, dbErr)
	}

	p.logger.Info("Provisioned new scheduler credentials",
		"ou_id", ouID, "client_id", clientID, "kv_path", resolvedKVPath, "secret_key", resolvedKey)

	return nil
}

// GetOCClientForOrg returns a cached OC client authenticated with the publisher app's
// org-scoped token. Used by the scheduler for CreateWorkflowRun and GetWorkflowRun —
// operations that run without a live user request context.
//
// The OpenChoreoClient (and the AuthProvider it wraps, plus the underlying http.Client)
// is built once per org and cached, so connection-pool keep-alive and token-refresh state
// are preserved across scheduler cycles.
func (p *publisherCredentialProvisioner) GetOCClientForOrg(ctx context.Context, ouID string) (client.OpenChoreoClient, error) {
	p.orgOCMu.RLock()
	c, ok := p.orgOCClients[ouID]
	p.orgOCMu.RUnlock()
	if ok {
		return c, nil
	}

	result, err, _ := p.sfg.Do("ocClient:"+ouID, func() (any, error) {
		// Re-check under read lock — singleflight may have just finished a previous build.
		p.orgOCMu.RLock()
		if c, ok := p.orgOCClients[ouID]; ok {
			p.orgOCMu.RUnlock()
			return c, nil
		}
		p.orgOCMu.RUnlock()

		// DB I/O and decrypt run with no lock held; singleflight already serializes
		// concurrent callers for this ouID.
		cred, err := p.schedulerCredRepo.GetByOrgName(ouID)
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("failed to look up scheduler credentials for org %s: %w", ouID, err)
			}
			// Provision on demand: the periodic scheduler calls this directly and never calls EnsureCredentials.
			p.logger.Info("No scheduler credentials found for org, provisioning on demand", "ou_id", ouID)
			if provErr := p.provisionSchedulerCredentials(ctx, ouID, ""); provErr != nil {
				return nil, fmt.Errorf("failed to provision scheduler credentials for org %s: %w", ouID, provErr)
			}
			cred, err = p.schedulerCredRepo.GetByOrgName(ouID)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, fmt.Errorf("%w: org %s — provisioned but still not found: %w", ErrSchedulerCredentialNotFound, ouID, err)
				}
				return nil, fmt.Errorf("failed to look up scheduler credentials for org %s after provisioning: %w", ouID, err)
			}
		}
		if len(cred.ClientSecretEncrypted) == 0 {
			// Record exists but has no encrypted secret — regenerate the Thunder client secret,
			// push it to the secret store, and persist the encrypted copy to DB.
			p.logger.Info("No encrypted secret for org, regenerating Thunder client secret",
				"ou_id", ouID, "client_id", cred.ClientID)
			newSecret, backfillErr := p.thunderClient.RegenerateAppClientSecret(ctx, cred.ClientID)
			if backfillErr != nil {
				return nil, fmt.Errorf("failed to regenerate client secret for org %s: %w", ouID, backfillErr)
			}
			// Propagate the new secret to the secret store.
			if _, backfillErr = p.secretClient.PatchSecret(ctx, schedulerSecretLocation(ouID),
				map[string]string{"client-secret": newSecret}, nil); backfillErr != nil {
				return nil, fmt.Errorf("failed to update secret store for org %s: %w", ouID, backfillErr)
			}
			encrypted, backfillErr := utils.EncryptBytes([]byte(newSecret), p.encryptionKey)
			if backfillErr != nil {
				return nil, fmt.Errorf("failed to encrypt regenerated client secret for org %s: %w", ouID, backfillErr)
			}
			cred.ClientSecretEncrypted = encrypted
			if backfillErr = p.schedulerCredRepo.Upsert(cred); backfillErr != nil {
				return nil, fmt.Errorf("failed to persist regenerated secret for org %s: %w", ouID, backfillErr)
			}
			p.logger.Info("Backfilled encrypted client secret", "ou_id", ouID, "client_id", cred.ClientID)
		}

		secretBytes, err := utils.DecryptBytes(cred.ClientSecretEncrypted, p.encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt publisher secret for org %s: %w", ouID, err)
		}

		authProv := ocauth.NewAuthProvider(ocauth.Config{
			TokenURL:     p.idpTokenURL,
			ClientID:     cred.ClientID,
			ClientSecret: string(secretBytes),
		})
		ocCl, err := client.NewOpenChoreoClient(&client.Config{
			BaseURL:          p.ocBaseURL,
			DefaultNamespace: config.GetConfig().OpenChoreo.DefaultNamespace,
			AuthProvider:     authProv,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to build OC client for org %s: %w", ouID, err)
		}

		p.orgOCMu.Lock()
		p.orgOCClients[ouID] = ocCl
		p.orgOCMu.Unlock()

		p.logger.Debug("Created org OC client", "ou_id", ouID, "client_id", cred.ClientID)
		return ocCl, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(client.OpenChoreoClient), nil
}
