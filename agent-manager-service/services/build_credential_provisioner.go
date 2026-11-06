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
	"time"

	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// BuildCredentialProvisioner provisions an organization's build client: the OAuth2
// client a source build's workload step uses to call the build publisher endpoint.
//
// The client is created in the organization's own OU and named
// jwtassertion.BuildClientAudiencePrefix + ouID, which is exactly what
// jwtassertion.BuildClientAuthMiddleware requires of a caller, so a build can
// only ever publish for the organization that owns it.
type BuildCredentialProvisioner interface {
	// EnsureBuildClient returns the organization's build client, provisioning it on
	// first use. The result names the client and where its secret is stored; it
	// never carries the secret.
	EnsureBuildClient(ctx context.Context, ouID string) (*client.BuildClientParams, error)
}

// buildClientAppName is the Thunder application (and OAuth2 client ID) of an
// organization's build client.
func buildClientAppName(ouID string) string {
	return jwtassertion.BuildClientAudiencePrefix + ouID
}

// buildClientSecretLocation is where an organization's build client secret is stored.
func buildClientSecretLocation(ouID string) secretmanagersvc.SecretLocation {
	return secretmanagersvc.SecretLocation{
		OrgName:    ouID,
		EntityName: buildClientAppName(ouID),
	}
}

type buildCredentialProvisioner struct {
	thunderClient thundersvc.ThunderClient
	secretClient  secretmanagersvc.SecretManagementClient
	ocClient      client.OpenChoreoClient
	credRepo      repositories.OrgBuildCredentialRepository
	logger        *slog.Logger

	sfg singleflight.Group // serializes provisioning per organization
}

// NewBuildCredentialProvisioner returns nil when the build client is disabled
// (config.BuildClient.Enabled is false): builds then carry no build client and
// keep their previous path. Config validation guarantees Thunder is configured
// whenever it is enabled.
func NewBuildCredentialProvisioner(
	cfg config.Config,
	logger *slog.Logger,
	secretClient secretmanagersvc.SecretManagementClient,
	ocClient client.OpenChoreoClient,
	credRepo repositories.OrgBuildCredentialRepository,
) BuildCredentialProvisioner {
	if !cfg.BuildClient.Enabled {
		logger.Info("Build client disabled; builds carry no per-organization build client")
		return nil
	}
	thunderCl := thundersvc.NewThunderClientWithDialOverride(
		cfg.Thunder.BaseURL,
		cfg.Thunder.ClientID,
		cfg.Thunder.ClientSecret,
		cfg.Thunder.ResolveToHost,
		cfg.Thunder.ResolvedSystemResourceIdentifier(),
	)
	logger.Info("Build client enabled", "workflows", cfg.BuildClient.Workflows)
	return newBuildCredentialProvisioner(thunderCl, secretClient, ocClient, credRepo, logger)
}

func newBuildCredentialProvisioner(
	thunderClient thundersvc.ThunderClient,
	secretClient secretmanagersvc.SecretManagementClient,
	ocClient client.OpenChoreoClient,
	credRepo repositories.OrgBuildCredentialRepository,
	logger *slog.Logger,
) *buildCredentialProvisioner {
	return &buildCredentialProvisioner{
		thunderClient: thunderClient,
		secretClient:  secretClient,
		ocClient:      ocClient,
		credRepo:      credRepo,
		logger:        logger,
	}
}

// buildClientProvisionTimeout bounds one provisioning run (identity provider,
// secret store and OpenChoreo calls under the per-organization lock).
const buildClientProvisionTimeout = 60 * time.Second

// EnsureBuildClient returns the stored build client, or provisions one.
// Concurrent calls for the same organization in this process share a single
// provisioning run, and across replicas provisioning is serialized by a
// per-organization database lock.
func (p *buildCredentialProvisioner) EnsureBuildClient(ctx context.Context, ouID string) (*client.BuildClientParams, error) {
	if ouID == "" {
		return nil, errors.New("build client: organization is required")
	}
	if params, found, err := p.storedBuildClient(ctx, ouID); err != nil || found {
		return params, err
	}
	result, err, _ := p.sfg.Do("build-client:"+ouID, func() (any, error) {
		// The run is shared by every caller that joins it, so it must not end
		// when the first caller's request does; it keeps that caller's values
		// (the audit recorder, the request token the secret store is called with).
		runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), buildClientProvisionTimeout)
		defer cancel()
		var params *client.BuildClientParams
		err := p.credRepo.WithProvisioningLock(runCtx, ouID, func() error {
			// Another replica may have provisioned it (or repaired a stale one)
			// while this one waited.
			stored, found, err := p.storedBuildClient(runCtx, ouID)
			if err != nil || found {
				params = stored
				return err
			}
			params, err = p.provisionAudited(runCtx, ouID)
			return err
		})
		return params, err
	})
	if err != nil {
		return nil, err
	}
	return result.(*client.BuildClientParams), nil
}

// storedBuildClient returns the organization's stored build client; found is
// false (with a nil error) when none is stored yet, or when the stored one is
// stale and must be provisioned again.
//
// A stored row is only a record of where the secret was put, so its
// SecretReference is checked on every use, as provisioning checks it: one that
// no longer exists, or has lost an entry the workload step reads, would fail
// every build in the organization at the workload step, and no one would ever
// clean up the row. Such a row counts as not stored, and provisioning replaces
// it. A check that fails for any other reason (OpenChoreo unreachable, say) is
// not taken as proof that the row is stale: the stored client is used, rather
// than regenerating a working secret because of a transient error.
func (p *buildCredentialProvisioner) storedBuildClient(ctx context.Context, ouID string) (*client.BuildClientParams, bool, error) {
	existing, err := p.credRepo.GetByOrgName(ouID)
	switch {
	case err == nil:
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, false, nil
	default:
		return nil, false, fmt.Errorf("failed to look up build client for org %s: %w", ouID, err)
	}
	if err := p.verifyClientSecretRef(ctx, ouID, existing.SecretRefName); err != nil {
		if isStaleBuildClientSecretRef(err) {
			p.logger.Warn("Stored build client's SecretReference is missing or unusable; provisioning the build client again",
				"ouID", ouID, "secretRefName", existing.SecretRefName, "error", err)
			return nil, false, nil
		}
		p.logger.Warn("Could not verify the stored build client's SecretReference; using it",
			"ouID", ouID, "secretRefName", existing.SecretRefName, "error", err)
	}
	return &client.BuildClientParams{SecretRefName: existing.SecretRefName}, true, nil
}

// errBuildClientSecretRefUnusable marks a SecretReference that exists but lacks
// an entry the workload step reads.
var errBuildClientSecretRefUnusable = errors.New("build client SecretReference is unusable")

// isStaleBuildClientSecretRef reports whether verifyClientSecretRef's error
// proves the stored SecretReference cannot work: it no longer exists, or it has
// lost an entry. Only these make the stored row stale.
func isStaleBuildClientSecretRef(err error) bool {
	return errors.Is(err, utils.ErrNotFound) || errors.Is(err, errBuildClientSecretRefUnusable)
}

// provisionAudited provisions the build client inside a fail-closed audit record.
func (p *buildCredentialProvisioner) provisionAudited(ctx context.Context, ouID string) (*client.BuildClientParams, error) {
	appName := buildClientAppName(ouID)
	// Issuing a live OAuth2 client secret: record the intent first and refuse to
	// proceed if the trail is unavailable.
	attempt, err := audit.Begin(ctx, audit.ActionBuildClientProvision,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceBuildClient, appName, appName),
		audit.Detail("clientId", appName),
	)
	if err != nil {
		return nil, err
	}
	params, err := p.provision(ctx, ouID, appName)
	attempt.Complete(ctx, err)
	return params, err
}

// provision creates (or adopts) the Thunder app, stores its secret and records
// where the secret lives. Mirrors publisherCredentialProvisioner's flow.
func (p *buildCredentialProvisioner) provision(ctx context.Context, ouID, appName string) (*client.BuildClientParams, error) {
	p.logger.Info("No usable build client stored for org, provisioning", "ouID", ouID, "appName", appName)

	clientID, clientSecret, created, err := p.thunderClient.EnsureApp(ctx, appName, ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to provision build client app for org %s: %w", ouID, err)
	}
	// Apps this service creates use their name as the client ID, which is what
	// the build publisher endpoint matches the token audience against. An app of
	// that name with another client ID was not created here and is not adopted.
	if clientID != appName {
		return nil, fmt.Errorf("build client app %s for org %s has unexpected client ID %q", appName, ouID, clientID)
	}
	// An app that already exists (an earlier run that failed after creating it)
	// does not return its secret; regenerate it rather than deleting the app.
	if !created && clientSecret == "" {
		p.logger.Warn("Build client app exists but its secret is not available, regenerating it",
			"ouID", ouID, "clientID", clientID)
		clientSecret, err = p.thunderClient.RegenerateAppClientSecret(ctx, appName)
		if err != nil {
			return nil, fmt.Errorf("failed to regenerate build client secret for org %s: %w", ouID, err)
		}
	}
	if clientSecret == "" {
		return nil, fmt.Errorf("failed to provision build client for org %s: no client secret available", ouID)
	}

	secretRefName, err := p.secretClient.CreateSecret(ctx, buildClientSecretLocation(ouID), map[string]string{
		"client-id":     clientID,
		"client-secret": clientSecret,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to store build client secret for org %s: %w", ouID, err)
	}
	if err := p.verifyClientSecretRef(ctx, ouID, secretRefName); err != nil {
		return nil, err
	}

	if err := p.credRepo.Upsert(&models.OrgBuildCredential{
		OUID:          ouID,
		OrgUUID:       ouID,
		ClientID:      clientID,
		SecretRefName: secretRefName,
	}); err != nil {
		return nil, fmt.Errorf("failed to persist build client for org %s: %w", ouID, err)
	}

	p.logger.Info("Provisioned build client", "ouID", ouID, "clientID", clientID, "secretRefName", secretRefName)
	return &client.BuildClientParams{SecretRefName: secretRefName}, nil
}

// verifyClientSecretRef checks that the stored SecretReference, which the build
// resolves into its ExternalSecret, carries both entries the workload step reads.
func (p *buildCredentialProvisioner) verifyClientSecretRef(ctx context.Context, ouID, secretRefName string) error {
	ref, err := p.ocClient.GetSecretReference(ctx, ouID, secretRefName)
	if err != nil {
		return fmt.Errorf("failed to get build client SecretReference %s: %w", secretRefName, err)
	}
	for _, key := range []string{"client-id", "client-secret"} {
		found := false
		for _, ds := range ref.Data {
			if ds.SecretKey == key && ds.RemoteRef.Key != "" {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%w: %s has no usable %q entry", errBuildClientSecretRefUnusable, secretRefName, key)
		}
	}
	return nil
}
