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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

var testEncryptionKey = []byte("01234567890123456789012345678901") // 32 bytes for AES-256

// Regression guard: only the scheduler credential is bound to amp-monitor-scheduler, never the publisher one.
func TestEnsureCredentials_NewOrg_BindsOnlySchedulerCredential(t *testing.T) {
	var boundClientIDs []string
	var boundRoles []string

	thunderMock := &clientmocks.ThunderClientMock{
		EnsurePublisherAppFunc: func(_ context.Context, orgName, _ string) (string, string, bool, error) {
			return "amp-publisher-" + orgName, "publisher-secret", true, nil
		},
		EnsureAppFunc: func(_ context.Context, appName, _ string) (string, string, bool, error) {
			return appName, "scheduler-secret", true, nil
		},
	}

	ocMock := &clientmocks.OpenChoreoClientMock{
		EnsureClusterRoleBindingFunc: func(_ context.Context, clientID string, roleName string) error {
			boundClientIDs = append(boundClientIDs, clientID)
			boundRoles = append(boundRoles, roleName)
			return nil
		},
	}

	var publisherUpserts []*models.OrgPublisherCredential
	credRepo := &repomocks.OrgPublisherCredentialRepositoryMock{
		GetByOrgNameFunc: func(_ string) (*models.OrgPublisherCredential, error) {
			return nil, gorm.ErrRecordNotFound
		},
		UpsertFunc: func(cred *models.OrgPublisherCredential) error {
			publisherUpserts = append(publisherUpserts, cred)
			return nil
		},
	}

	var schedulerUpserts []*models.OrgSchedulerCredential
	schedulerCredRepo := &repomocks.OrgSchedulerCredentialRepositoryMock{
		GetByOrgNameFunc: func(_ string) (*models.OrgSchedulerCredential, error) {
			return nil, gorm.ErrRecordNotFound
		},
		UpsertFunc: func(cred *models.OrgSchedulerCredential) error {
			schedulerUpserts = append(schedulerUpserts, cred)
			return nil
		},
	}

	secretClient := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(_ context.Context, location secretmanagersvc.SecretLocation, _ map[string]string) (string, error) {
			return "ref-" + location.EntityName, nil
		},
	}

	p := &publisherCredentialProvisioner{
		thunderClient:     thunderMock,
		secretClient:      secretClient,
		ocClient:          ocMock,
		credRepo:          credRepo,
		schedulerCredRepo: schedulerCredRepo,
		logger:            discardLogger(),
		encryptionKey:     testEncryptionKey,
		orgOCClients:      make(map[string]occlient.OpenChoreoClient),
	}

	// GetSecretReferenceFunc is nil: the SecretReference names returned by
	// CreateSecret are stored as-is, never read back through OpenChoreo.
	creds, err := p.EnsureCredentials(context.Background(), "acme", "org-uuid-1")
	require.NoError(t, err)
	assert.Equal(t, "amp-publisher-acme", creds.ClientID)
	assert.Equal(t, "ref-amp-publisher-acme", creds.SecretRefName)

	require.Len(t, publisherUpserts, 1)
	assert.Equal(t, "amp-publisher-acme", publisherUpserts[0].ClientID)
	assert.Equal(t, "ref-amp-publisher-acme", publisherUpserts[0].SecretKVPath)

	require.Len(t, schedulerUpserts, 1)
	assert.Equal(t, "amp-scheduler-acme", schedulerUpserts[0].ClientID)
	assert.Equal(t, "ref-amp-scheduler-acme", schedulerUpserts[0].SecretKVPath)

	require.Len(t, boundClientIDs, 1, "EnsureClusterRoleBinding should be called exactly once")
	assert.Equal(t, "amp-scheduler-acme", boundClientIDs[0])
	assert.Equal(t, schedulerRoleName, boundRoles[0])
}

// Idempotent path: only the scheduler credential's binding is re-verified on repeat calls.
func TestEnsureCredentials_ExistingOrg_ReverifiesOnlySchedulerBinding(t *testing.T) {
	var boundClientIDs []string

	thunderMock := &clientmocks.ThunderClientMock{}
	ocMock := &clientmocks.OpenChoreoClientMock{
		EnsureClusterRoleBindingFunc: func(_ context.Context, clientID string, _ string) error {
			boundClientIDs = append(boundClientIDs, clientID)
			return nil
		},
	}

	credRepo := &repomocks.OrgPublisherCredentialRepositoryMock{
		GetByOrgNameFunc: func(_ string) (*models.OrgPublisherCredential, error) {
			return &models.OrgPublisherCredential{
				ClientID: "amp-publisher-acme", SecretKVPath: "kv/publisher", SecretKey: "client-secret",
			}, nil
		},
	}
	schedulerCredRepo := &repomocks.OrgSchedulerCredentialRepositoryMock{
		GetByOrgNameFunc: func(_ string) (*models.OrgSchedulerCredential, error) {
			return &models.OrgSchedulerCredential{
				ClientID: "amp-scheduler-acme", SecretKVPath: "kv/scheduler", SecretKey: "client-secret",
			}, nil
		},
	}

	p := &publisherCredentialProvisioner{
		thunderClient:     thunderMock,
		ocClient:          ocMock,
		credRepo:          credRepo,
		schedulerCredRepo: schedulerCredRepo,
		logger:            discardLogger(),
		encryptionKey:     testEncryptionKey,
		orgOCClients:      make(map[string]occlient.OpenChoreoClient),
	}

	creds, err := p.EnsureCredentials(context.Background(), "acme", "org-uuid-1")
	require.NoError(t, err)
	assert.Equal(t, "amp-publisher-acme", creds.ClientID)

	require.Len(t, boundClientIDs, 1, "only the scheduler credential's binding should be re-verified")
	assert.Equal(t, "amp-scheduler-acme", boundClientIDs[0])
}

// credRepo has no GetByOrgNameFunc — the mock panics if GetOCClientForOrg ever touches it.
func TestGetOCClientForOrg_ReadsFromSchedulerCredRepo(t *testing.T) {
	encryptedSecret, err := utils.EncryptBytes([]byte("scheduler-secret-value"), testEncryptionKey)
	require.NoError(t, err)

	credRepo := &repomocks.OrgPublisherCredentialRepositoryMock{} // no GetByOrgNameFunc — must not be called
	schedulerCredRepo := &repomocks.OrgSchedulerCredentialRepositoryMock{
		GetByOrgNameFunc: func(ouID string) (*models.OrgSchedulerCredential, error) {
			assert.Equal(t, "acme", ouID)
			return &models.OrgSchedulerCredential{
				ClientID:              "amp-scheduler-acme",
				ClientSecretEncrypted: encryptedSecret,
			}, nil
		},
	}

	p := &publisherCredentialProvisioner{
		credRepo:          credRepo,
		schedulerCredRepo: schedulerCredRepo,
		logger:            discardLogger(),
		encryptionKey:     testEncryptionKey,
		idpTokenURL:       "http://thunder.test/oauth2/token",
		ocBaseURL:         "http://openchoreo.test",
		orgOCClients:      make(map[string]occlient.OpenChoreoClient),
	}

	ocClient, err := p.GetOCClientForOrg(context.Background(), "acme")
	require.NoError(t, err)
	assert.NotNil(t, ocClient)
}

// Orgs whose monitors predate the credential split have no scheduler credential yet —
// GetOCClientForOrg must provision one on demand rather than fail permanently.
func TestGetOCClientForOrg_ProvisionsOnDemand_WhenSchedulerCredMissing(t *testing.T) {
	var upserted *models.OrgSchedulerCredential

	schedulerCredRepo := &repomocks.OrgSchedulerCredentialRepositoryMock{
		GetByOrgNameFunc: func(ouID string) (*models.OrgSchedulerCredential, error) {
			if upserted == nil {
				return nil, gorm.ErrRecordNotFound
			}
			return upserted, nil
		},
		UpsertFunc: func(cred *models.OrgSchedulerCredential) error {
			upserted = cred
			return nil
		},
	}

	var boundClientID string
	thunderMock := &clientmocks.ThunderClientMock{
		EnsureAppFunc: func(_ context.Context, appName, _ string) (string, string, bool, error) {
			assert.Equal(t, "amp-scheduler-acme", appName)
			return appName, "scheduler-secret", true, nil
		},
	}
	ocMock := &clientmocks.OpenChoreoClientMock{
		EnsureClusterRoleBindingFunc: func(_ context.Context, clientID string, roleName string) error {
			boundClientID = clientID
			assert.Equal(t, schedulerRoleName, roleName)
			return nil
		},
	}
	secretClient := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(_ context.Context, location secretmanagersvc.SecretLocation, _ map[string]string) (string, error) {
			return "ref-" + location.EntityName, nil
		},
	}

	p := &publisherCredentialProvisioner{
		thunderClient:     thunderMock,
		secretClient:      secretClient,
		ocClient:          ocMock,
		schedulerCredRepo: schedulerCredRepo,
		logger:            discardLogger(),
		encryptionKey:     testEncryptionKey,
		idpTokenURL:       "http://thunder.test/oauth2/token",
		ocBaseURL:         "http://openchoreo.test",
		orgOCClients:      make(map[string]occlient.OpenChoreoClient),
	}

	ocClient, err := p.GetOCClientForOrg(context.Background(), "acme")
	require.NoError(t, err)
	assert.NotNil(t, ocClient)
	require.NotNil(t, upserted, "the missing scheduler credential should have been provisioned")
	assert.Equal(t, "amp-scheduler-acme", upserted.ClientID)
	assert.Equal(t, "amp-scheduler-acme", boundClientID)
}

// A real DB error on the post-provisioning recheck must not be reported as "genuinely absent".
func TestGetOCClientForOrg_RealDBErrorOnRecheck_NotMisreportedAsNotFound(t *testing.T) {
	dbTimeout := errors.New("db timeout")
	calls := 0
	schedulerCredRepo := &repomocks.OrgSchedulerCredentialRepositoryMock{
		GetByOrgNameFunc: func(_ string) (*models.OrgSchedulerCredential, error) {
			calls++
			if calls == 1 {
				return nil, gorm.ErrRecordNotFound
			}
			return nil, dbTimeout
		},
		UpsertFunc: func(_ *models.OrgSchedulerCredential) error { return nil },
	}
	thunderMock := &clientmocks.ThunderClientMock{
		EnsureAppFunc: func(_ context.Context, appName, _ string) (string, string, bool, error) {
			return appName, "scheduler-secret", true, nil
		},
	}
	ocMock := &clientmocks.OpenChoreoClientMock{
		EnsureClusterRoleBindingFunc: func(_ context.Context, _ string, _ string) error { return nil },
	}
	secretClient := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(_ context.Context, location secretmanagersvc.SecretLocation, _ map[string]string) (string, error) {
			return "ref-" + location.EntityName, nil
		},
	}

	p := &publisherCredentialProvisioner{
		thunderClient:     thunderMock,
		secretClient:      secretClient,
		ocClient:          ocMock,
		schedulerCredRepo: schedulerCredRepo,
		logger:            discardLogger(),
		encryptionKey:     testEncryptionKey,
		idpTokenURL:       "http://thunder.test/oauth2/token",
		ocBaseURL:         "http://openchoreo.test",
		orgOCClients:      make(map[string]occlient.OpenChoreoClient),
	}

	_, err := p.GetOCClientForOrg(context.Background(), "acme")
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrSchedulerCredentialNotFound),
		"a real DB error must not be reported as the not-found sentinel")
	assert.ErrorIs(t, err, dbTimeout)
}

// Re-provisioning must drop the cached per-org OpenChoreo client. The cached client wraps
// an AuthProvider holding the *previous* client ID and secret, and its access token stays
// valid for about an hour after the Thunder app behind it is gone — so keeping it makes a
// credential wipe look repaired while the scheduler is silently one token-refresh away from
// being unable to create any WorkflowRun until the process restarts.
func TestProvisionSchedulerCredentials_EvictsCachedOrgOCClient(t *testing.T) {
	thunderMock := &clientmocks.ThunderClientMock{
		EnsurePublisherAppFunc: func(_ context.Context, orgName, _ string) (string, string, bool, error) {
			return "amp-publisher-" + orgName, "publisher-secret", true, nil
		},
		EnsureAppFunc: func(_ context.Context, appName, _ string) (string, string, bool, error) {
			return appName, "scheduler-secret", true, nil
		},
	}

	ocMock := &clientmocks.OpenChoreoClientMock{
		EnsureClusterRoleBindingFunc: func(_ context.Context, _ string, _ string) error { return nil },
	}

	credRepo := &repomocks.OrgPublisherCredentialRepositoryMock{
		GetByOrgNameFunc: func(_ string) (*models.OrgPublisherCredential, error) {
			return nil, gorm.ErrRecordNotFound
		},
		UpsertFunc: func(_ *models.OrgPublisherCredential) error { return nil },
	}
	schedulerCredRepo := &repomocks.OrgSchedulerCredentialRepositoryMock{
		GetByOrgNameFunc: func(_ string) (*models.OrgSchedulerCredential, error) {
			return nil, gorm.ErrRecordNotFound
		},
		UpsertFunc: func(_ *models.OrgSchedulerCredential) error { return nil },
	}
	secretClient := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(_ context.Context, location secretmanagersvc.SecretLocation, _ map[string]string) (string, error) {
			return "ref-" + location.EntityName, nil
		},
	}

	stale := &clientmocks.OpenChoreoClientMock{}
	p := &publisherCredentialProvisioner{
		thunderClient:     thunderMock,
		secretClient:      secretClient,
		ocClient:          ocMock,
		credRepo:          credRepo,
		schedulerCredRepo: schedulerCredRepo,
		logger:            discardLogger(),
		encryptionKey:     testEncryptionKey,
		orgOCClients:      map[string]occlient.OpenChoreoClient{"acme": stale},
		orgOCGen:          make(map[string]uint64),
	}

	_, err := p.EnsureCredentials(context.Background(), "acme", "org-uuid-1")
	require.NoError(t, err)

	p.orgOCMu.RLock()
	cached, ok := p.orgOCClients["acme"]
	p.orgOCMu.RUnlock()

	assert.False(t, ok, "cached OC client should be evicted after re-provisioning, got %v", cached)
}

// A credential replacement that lands while a client is being built must stop that
// client from being cached. The two paths use different singleflight keys, so without
// the generation check the builder would insert a client made from the superseded
// secret straight after the eviction that was meant to remove it.
func TestGetOCClientForOrg_DoesNotCacheAClientSupersededMidBuild(t *testing.T) {
	p := &publisherCredentialProvisioner{
		logger:       discardLogger(),
		orgOCClients: make(map[string]occlient.OpenChoreoClient),
		orgOCGen:     make(map[string]uint64),
	}

	// Stand in for a builder that snapshotted the generation before the replacement.
	p.orgOCMu.RLock()
	gen := p.orgOCGen["acme"]
	p.orgOCMu.RUnlock()

	p.evictOrgOCClient("acme")

	p.orgOCMu.Lock()
	superseded := p.orgOCGen["acme"] != gen
	p.orgOCMu.Unlock()

	assert.True(t, superseded, "eviction must advance the generation so an in-flight build is detected")

	p.orgOCMu.RLock()
	_, cached := p.orgOCClients["acme"]
	p.orgOCMu.RUnlock()
	assert.False(t, cached)
}
