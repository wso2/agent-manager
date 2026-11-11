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
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

const schedulerRoleName = "amp-monitor-scheduler"

// ErrNotThunderMode is returned when the provisioner is not in Thunder mode.
var ErrNotThunderMode = errors.New("not in Thunder mode")

// ErrPublisherCredentialNotFound indicates EnsureCredentials has not yet been called for the org.
// Distinct from real DB errors so callers can decide whether to provision-on-demand vs retry.
var ErrPublisherCredentialNotFound = errors.New("publisher credentials not found")

// ErrSchedulerCredentialNotFound means a scheduler credential still wasn't found right after provisioning it.
var ErrSchedulerCredentialNotFound = errors.New("scheduler credentials not found")

// PublisherCredentials holds the provisioned OAuth2 credentials for publishing scores.
// Per-org (Thunder) credentials are referenced by SecretRefName, which the evaluation
// workflow resolves through OpenChoreo. Static credentials have no SecretReference and
// are referenced by SecretKVPath and SecretKey directly.
type PublisherCredentials struct {
	ClientID      string // OAuth2 client ID (becomes JWT subject)
	SecretRefName string // SecretReference holding client-secret; empty for static credentials
	SecretKVPath  string // KV path in the secret store (remoteRef.key for ExternalSecret); static credentials only
	SecretKey     string // Key within the KV secret (remoteRef.property for ExternalSecret); static credentials only
}

// publisherCredentialsFromRow builds the credentials for a stored per-org row.
// SecretKVPath holds the SecretReference name; rows written before the name was
// persisted hold its remote store path instead.
func publisherCredentialsFromRow(cred *models.OrgPublisherCredential) *PublisherCredentials {
	secretRefName := cred.SecretKVPath
	if isLegacySecretRefPath(secretRefName) {
		secretRefName = secretRefNameFromStorePath(secretRefName, publisherSecretLocation(cred.OUID))
	}
	return &PublisherCredentials{
		ClientID:      cred.ClientID,
		SecretRefName: secretRefName,
	}
}

// secretRefNameFromStorePath returns the name of the SecretReference behind a
// stored remote store path, given the location the secret was created for.
//
// A secret created through a Secret Manager API is stored at
// "<mount>/<namespace>/generic/<name>", named after its SecretReference: on-prem
// "secret/<ns>/generic/monitor-<id>-secrets", in WSO2 Cloud
// "<mount>/<ns>/generic/cred-<id>". There the last segment is the name, which in
// WSO2 Cloud cannot be rebuilt from the location.
//
// A secret written by the earlier OpenBao provider is stored at a path built from
// its location ("<org>/monitor-<id>/llm-proxy-configs", "<org>/amp-publisher-<org>"),
// and its SecretReference took the location's deterministic name.
func secretRefNameFromStorePath(storePath string, location secretmanagersvc.SecretLocation) string {
	segments := strings.Split(storePath, "/")
	if len(segments) >= 2 && segments[len(segments)-2] == "generic" {
		return segments[len(segments)-1]
	}
	return location.SecretRefName()
}

// PublisherCredentialProvisioner provisions per-org publisher credentials.
type PublisherCredentialProvisioner interface {
	// EnsureCredentials provisions per-org publisher credentials.
	// orgUUID is the Thunder organization unit UUID (from JWT ouId claim).
	EnsureCredentials(ctx context.Context, ouID, orgUUID string) (*PublisherCredentials, error)

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
	credRepo          repositories.OrgPublisherCredentialRepository
	schedulerCredRepo repositories.OrgSchedulerCredentialRepository
	logger            *slog.Logger
	encryptionKey     []byte
	idpTokenURL       string

	sfg singleflight.Group // serializes provisioning
}

// NewPublisherCredentialProvisioner creates a provisioner.
// If Thunder is not configured (BaseURL empty), returns a static provisioner
// that always returns the default amp-publisher-client credentials.
func NewPublisherCredentialProvisioner(
	cfg config.Config,
	encryptionKey []byte,
	logger *slog.Logger,
	secretClient secretmanagersvc.SecretManagementClient,
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

	thunderCl := thundersvc.NewThunderClientWithDialOverride(
		cfg.Thunder.BaseURL,
		cfg.Thunder.ClientID,
		cfg.Thunder.ClientSecret,
		cfg.Thunder.ResolveToHost,
		cfg.Thunder.ResolvedSystemResourceIdentifier(),
	)

	logger.Info(
		"Publisher credential provisioner initialized with Thunder",
		"thunderBaseURL", cfg.Thunder.BaseURL,
		"usesDefaultResourceServer", cfg.Thunder.UseDefaultResourceServer,
	)

	return &publisherCredentialProvisioner{
		thunderClient:     thunderCl,
		secretClient:      secretClient,
		credRepo:          credRepo,
		schedulerCredRepo: schedulerCredRepo,
		logger:            logger,
		encryptionKey:     encryptionKey,
		idpTokenURL:       cfg.IDP.TokenURL,
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

// schedulerAppName is the Thunder application name for an org's scheduler credential.
// Thunder resolves applications by name, so every caller must derive it here rather
// than passing something that merely identifies the same app (a client ID will not
// match, and the lookup fails silently as "app not found").
func schedulerAppName(ouID string) string { return "amp-scheduler-" + ouID }

// schedulerSecretLocation builds the SecretLocation for scheduler-only credentials.
func schedulerSecretLocation(ouID string) secretmanagersvc.SecretLocation {
	return secretmanagersvc.SecretLocation{
		OrgName:    ouID,
		EntityName: schedulerAppName(ouID),
	}
}

// EnsureCredentials provisions per-org publisher and scheduler credentials.
// Uses singleflight to deduplicate concurrent provisioning calls for the same org.
func (p *publisherCredentialProvisioner) EnsureCredentials(ctx context.Context, ouID, orgUUID string) (*PublisherCredentials, error) {
	p.logger.Debug("EnsureCredentials called", "ouID", ouID, "orgUUID", orgUUID)

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
			"ouID", ouID, "clientID", existing.ClientID)

		return publisherCredentialsFromRow(existing), nil
	}

	p.logger.Info("No existing credentials, provisioning via Thunder", "ouID", ouID)

	// Not found — create Thunder OAuth app
	clientID, clientSecret, created, err := p.thunderClient.EnsurePublisherApp(ctx, ouID, orgUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to provision Thunder app for org %s: %w", ouID, err)
	}
	p.logger.Info("Thunder EnsurePublisherApp result",
		"ouID", ouID, "clientID", clientID, "created", created, "hasSecret", clientSecret != "")

	// If app already existed in Thunder but not in DB, clientSecret is empty.
	// Regenerate rather than deleting the whole app.
	if !created && clientSecret == "" {
		p.logger.Warn("Thunder app exists but secret not available — regenerating client secret",
			"ouID", ouID, "clientID", clientID)

		clientSecret, err = p.thunderClient.RegenerateClientSecret(ctx, ouID)
		if err != nil {
			return nil, fmt.Errorf("failed to regenerate client secret for org %s: %w", ouID, err)
		}
		p.logger.Info("Regenerated Thunder client secret",
			"ouID", ouID, "clientID", clientID)
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
		"ouID", ouID, "secretRefName", secretRefName)

	// Encrypt the client secret so the scheduler can decrypt and use it for token generation.
	encryptedSecret, err := utils.EncryptBytes([]byte(clientSecret), p.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt publisher secret for org %s: %w", ouID, err)
	}

	// Save to DB — treat as fatal since we just provisioned real credentials.
	// SecretKVPath stores the SecretReference name the workflow resolves.
	dbCred := &models.OrgPublisherCredential{
		OUID:                  ouID,
		OrgUUID:               orgUUID,
		ClientID:              clientID,
		SecretKVPath:          secretRefName,
		SecretKey:             "client-secret",
		ClientSecretEncrypted: encryptedSecret,
	}
	if dbErr := p.credRepo.Upsert(dbCred); dbErr != nil {
		return nil, fmt.Errorf("failed to persist publisher credentials for org %s: %w", ouID, dbErr)
	}

	p.logger.Info("Provisioned new publisher credentials",
		"ouID", ouID, "clientID", clientID, "secretRefName", secretRefName)

	return &PublisherCredentials{
		ClientID:      clientID,
		SecretRefName: secretRefName,
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
			"ouID", ouID, "clientID", existing.ClientID)

		return nil
	}

	p.logger.Info("No existing scheduler credentials, provisioning via Thunder", "ouID", ouID)

	appName := schedulerAppName(ouID)
	clientID, clientSecret, created, err := p.thunderClient.EnsureApp(ctx, appName, orgUUID)
	if err != nil {
		return fmt.Errorf("failed to provision Thunder scheduler app for org %s: %w", ouID, err)
	}
	p.logger.Info("Thunder EnsureApp result for scheduler credential",
		"ouID", ouID, "clientID", clientID, "created", created, "hasSecret", clientSecret != "")

	if !created && clientSecret == "" {
		p.logger.Warn("Thunder scheduler app exists but secret not available — regenerating client secret",
			"ouID", ouID, "clientID", clientID)

		clientSecret, err = p.thunderClient.RegenerateAppClientSecret(ctx, appName)
		if err != nil {
			return fmt.Errorf("failed to regenerate scheduler client secret for org %s: %w", ouID, err)
		}
		p.logger.Info("Regenerated Thunder scheduler client secret",
			"ouID", ouID, "clientID", clientID)
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
		"ouID", ouID, "secretRefName", secretRefName)

	encryptedSecret, err := utils.EncryptBytes([]byte(clientSecret), p.encryptionKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt scheduler secret for org %s: %w", ouID, err)
	}

	dbCred := &models.OrgSchedulerCredential{
		OUID:                  ouID,
		OrgUUID:               orgUUID,
		ClientID:              clientID,
		SecretKVPath:          secretRefName,
		SecretKey:             "client-secret",
		ClientSecretEncrypted: encryptedSecret,
	}
	if dbErr := p.schedulerCredRepo.Upsert(dbCred); dbErr != nil {
		return fmt.Errorf("failed to persist scheduler credentials for org %s: %w", ouID, dbErr)
	}

	// A cached client for this org was built from the credential this one replaces, and
	// nothing else ever evicts it. Its access token keeps working until it expires, so
	// leaving it in place makes re-provisioning look like it worked and then fail an hour
	// later, when the wrapped AuthProvider tries to refresh against a Thunder app that no
	// longer exists. Drop it so the next caller rebuilds from what was just persisted.

	p.logger.Info("Provisioned new scheduler credentials",
		"ouID", ouID, "clientID", clientID, "secretRefName", secretRefName)

	return nil
}
