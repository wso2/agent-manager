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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
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
		orgOCClients:      make(map[string]orgOCClientEntry),
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
		orgOCClients:      make(map[string]orgOCClientEntry),
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
		orgOCClients:      make(map[string]orgOCClientEntry),
	}

	ocClient, err := p.GetOCClientForOrg(context.Background(), "acme")
	require.NoError(t, err)
	assert.NotNil(t, ocClient)
}

type recordedAuthProvider struct {
	tokenURL, clientID, clientSecret string
}

func (r *recordedAuthProvider) GetToken(context.Context) (string, error) { return "", nil }
func (r *recordedAuthProvider) InvalidateToken()                         {}

func TestGetOCClientForOrg_BuildsTokenSourceThroughDeploymentFactory(t *testing.T) {
	encryptedSecret, err := utils.EncryptBytes([]byte("scheduler-secret-value"), testEncryptionKey)
	require.NoError(t, err)

	var built *recordedAuthProvider
	SetOrgClientAuthProviderFactory(func(tokenURL, clientID, clientSecret string) occlient.AuthProvider {
		built = &recordedAuthProvider{tokenURL: tokenURL, clientID: clientID, clientSecret: clientSecret}
		return built
	})
	t.Cleanup(func() { SetOrgClientAuthProviderFactory(nil) })

	p := &publisherCredentialProvisioner{
		schedulerCredRepo: &repomocks.OrgSchedulerCredentialRepositoryMock{
			GetByOrgNameFunc: func(string) (*models.OrgSchedulerCredential, error) {
				return &models.OrgSchedulerCredential{
					ClientID:              "amp-scheduler-acme",
					ClientSecretEncrypted: encryptedSecret,
				}, nil
			},
		},
		logger:        discardLogger(),
		encryptionKey: testEncryptionKey,
		idpTokenURL:   "http://thunder.test/oauth2/token",
		ocBaseURL:     "http://openchoreo.test",
		orgOCClients:  make(map[string]orgOCClientEntry),
		orgOCGen:      make(map[string]uint64),
	}

	_, err = p.GetOCClientForOrg(context.Background(), "acme")
	require.NoError(t, err)
	require.NotNil(t, built)
	assert.Equal(t, recordedAuthProvider{
		tokenURL:     "http://thunder.test/oauth2/token",
		clientID:     "amp-scheduler-acme",
		clientSecret: "scheduler-secret-value",
	}, *built)
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
		orgOCClients:      make(map[string]orgOCClientEntry),
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
			// Not found on the first read, on the re-read under the org lock, and on
			// provisionSchedulerCredentials' own lookup, so the credential is provisioned;
			// the read after provisioning then fails for real.
			if calls <= 3 {
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
		orgOCClients:      make(map[string]orgOCClientEntry),
	}

	_, err := p.GetOCClientForOrg(context.Background(), "acme")
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrSchedulerCredentialNotFound),
		"a real DB error must not be reported as the not-found sentinel")
	assert.ErrorIs(t, err, dbTimeout)
	assert.ErrorContains(t, err, "after provisioning", "the failing read must be the recheck after provisioning")
	assert.Len(t, thunderMock.EnsureAppCalls(), 1, "the credential must have been provisioned before the recheck")
	assert.Len(t, schedulerCredRepo.UpsertCalls(), 1, "the credential must have been stored before the recheck")
	assert.Equal(t, 4, calls)
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
		orgOCClients:      map[string]orgOCClientEntry{"acme": {client: stale}},
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
		orgOCClients: make(map[string]orgOCClientEntry),
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

// fakeOrgCredentialLocker stands in for the database org credential lock and
// records whether it is held.
type fakeOrgCredentialLocker struct {
	err   error
	held  bool
	locks int
}

func (f *fakeOrgCredentialLocker) LockOrgCredentials(_ context.Context, _ string) (func(), error) {
	if f.err != nil {
		return nil, f.err
	}
	f.locks++
	f.held = true
	return func() { f.held = false }, nil
}

// lockingSchedulerCredRepo is a scheduler credential repository that can also
// take the org credential lock, as the real one can.
type lockingSchedulerCredRepo struct {
	*repomocks.OrgSchedulerCredentialRepositoryMock
	fakeOrgCredentialLocker
}

func TestNewPublisherCredentialProvisioner_UsesRepoOrgCredentialLocker(t *testing.T) {
	cfg := config.Config{}
	cfg.Thunder.BaseURL = "http://thunder.test"
	repo := &lockingSchedulerCredRepo{OrgSchedulerCredentialRepositoryMock: &repomocks.OrgSchedulerCredentialRepositoryMock{}}

	prov, err := NewPublisherCredentialProvisioner(cfg, testEncryptionKey, discardLogger(), nil, nil, nil, repo)
	require.NoError(t, err)
	assert.NotNil(t, prov.(*publisherCredentialProvisioner).orgCredLocker,
		"a repository that can lock org credentials must back provisioning across replicas")

	plain, err := NewPublisherCredentialProvisioner(cfg, testEncryptionKey, discardLogger(), nil, nil, nil,
		&repomocks.OrgSchedulerCredentialRepositoryMock{})
	require.NoError(t, err)
	assert.Nil(t, plain.(*publisherCredentialProvisioner).orgCredLocker)
}

// Two replicas handling an org's first runs must not both provision: the org
// lock must be held across every Thunder call and store.
func TestEnsureCredentials_HoldsOrgLockWhileProvisioning(t *testing.T) {
	locker := &fakeOrgCredentialLocker{}
	var heldDuring []bool
	thunderMock := &clientmocks.ThunderClientMock{
		EnsurePublisherAppFunc: func(_ context.Context, orgName, _ string) (string, string, bool, error) {
			heldDuring = append(heldDuring, locker.held)
			return "amp-publisher-" + orgName, "publisher-secret", true, nil
		},
		EnsureAppFunc: func(_ context.Context, appName, _ string) (string, string, bool, error) {
			heldDuring = append(heldDuring, locker.held)
			return appName, "scheduler-secret", true, nil
		},
	}
	p := &publisherCredentialProvisioner{
		thunderClient: thunderMock,
		secretClient: &clientmocks.SecretManagementClientMock{
			CreateSecretFunc: func(_ context.Context, location secretmanagersvc.SecretLocation, _ map[string]string) (string, error) {
				heldDuring = append(heldDuring, locker.held)
				return "ref-" + location.EntityName, nil
			},
		},
		ocClient: &clientmocks.OpenChoreoClientMock{
			EnsureClusterRoleBindingFunc: func(context.Context, string, string) error { return nil },
		},
		credRepo: &repomocks.OrgPublisherCredentialRepositoryMock{
			GetByOrgNameFunc: func(string) (*models.OrgPublisherCredential, error) { return nil, gorm.ErrRecordNotFound },
			UpsertFunc:       func(*models.OrgPublisherCredential) error { return nil },
		},
		schedulerCredRepo: &repomocks.OrgSchedulerCredentialRepositoryMock{
			GetByOrgNameFunc: func(string) (*models.OrgSchedulerCredential, error) { return nil, gorm.ErrRecordNotFound },
			UpsertFunc:       func(*models.OrgSchedulerCredential) error { return nil },
		},
		orgCredLocker: locker,
		logger:        discardLogger(),
		encryptionKey: testEncryptionKey,
	}

	_, err := p.EnsureCredentials(context.Background(), "acme", "org-uuid-1")
	require.NoError(t, err)

	assert.Equal(t, 1, locker.locks)
	assert.Equal(t, []bool{true, true, true, true}, heldDuring, "every Thunder call and secret store must run under the org lock")
	assert.False(t, locker.held, "the org lock must be released afterwards")
}

func TestEnsureCredentials_LockErrorProvisionsNothing(t *testing.T) {
	lockErr := errors.New("database unavailable")
	thunderMock := &clientmocks.ThunderClientMock{}
	credRepo := &repomocks.OrgPublisherCredentialRepositoryMock{}
	p := &publisherCredentialProvisioner{
		thunderClient: thunderMock,
		credRepo:      credRepo,
		orgCredLocker: &fakeOrgCredentialLocker{err: lockErr},
		logger:        discardLogger(),
	}

	_, err := p.EnsureCredentials(context.Background(), "acme", "org-uuid-1")

	require.ErrorIs(t, err, lockErr)
	assert.Empty(t, credRepo.GetByOrgNameCalls(), "nothing may be read or provisioned without the org lock")
	assert.Empty(t, thunderMock.EnsurePublisherAppCalls())
}

// The steady state must not touch the org lock: a stored, usable credential is
// served from the cache.
func TestGetOCClientForOrg_ServesCachedClientWithoutOrgLock(t *testing.T) {
	encrypted, err := utils.EncryptBytes([]byte("scheduler-secret"), testEncryptionKey)
	require.NoError(t, err)
	locker := &fakeOrgCredentialLocker{}
	p := &publisherCredentialProvisioner{
		schedulerCredRepo: &repomocks.OrgSchedulerCredentialRepositoryMock{
			GetByOrgNameFunc: func(string) (*models.OrgSchedulerCredential, error) {
				return &models.OrgSchedulerCredential{ClientID: "amp-scheduler-acme", ClientSecretEncrypted: encrypted}, nil
			},
		},
		orgCredLocker: locker,
		logger:        discardLogger(),
		encryptionKey: testEncryptionKey,
		idpTokenURL:   "http://thunder.test/oauth2/token",
		ocBaseURL:     "http://openchoreo.test",
	}

	first, err := p.GetOCClientForOrg(context.Background(), "acme")
	require.NoError(t, err)
	second, err := p.GetOCClientForOrg(context.Background(), "acme")
	require.NoError(t, err)

	assert.Same(t, first, second, "an unchanged credential must reuse the cached client")
	assert.Zero(t, locker.locks, "a usable stored credential needs no org lock")
}

// Another replica can regenerate the scheduler secret. A client cached here
// from the old one keeps working only until its token expires, so the next call
// must rebuild from the credential now stored.
func TestGetOCClientForOrg_RebuildsWhenStoredCredentialChanged(t *testing.T) {
	oldSecret, err := utils.EncryptBytes([]byte("old-secret"), testEncryptionKey)
	require.NoError(t, err)
	newSecret, err := utils.EncryptBytes([]byte("new-secret"), testEncryptionKey)
	require.NoError(t, err)
	stored := &models.OrgSchedulerCredential{ClientID: "amp-scheduler-acme", ClientSecretEncrypted: oldSecret}

	var builtWith []string
	SetOrgClientAuthProviderFactory(func(_, _, clientSecret string) occlient.AuthProvider {
		builtWith = append(builtWith, clientSecret)
		return &recordedAuthProvider{clientSecret: clientSecret}
	})
	t.Cleanup(func() { SetOrgClientAuthProviderFactory(nil) })

	p := &publisherCredentialProvisioner{
		schedulerCredRepo: &repomocks.OrgSchedulerCredentialRepositoryMock{
			GetByOrgNameFunc: func(string) (*models.OrgSchedulerCredential, error) {
				copied := *stored
				return &copied, nil
			},
		},
		logger:        discardLogger(),
		encryptionKey: testEncryptionKey,
		idpTokenURL:   "http://thunder.test/oauth2/token",
		ocBaseURL:     "http://openchoreo.test",
	}

	first, err := p.GetOCClientForOrg(context.Background(), "acme")
	require.NoError(t, err)

	stored = &models.OrgSchedulerCredential{ClientID: "amp-scheduler-acme", ClientSecretEncrypted: newSecret}
	second, err := p.GetOCClientForOrg(context.Background(), "acme")
	require.NoError(t, err)

	assert.NotSame(t, first, second, "a client built from a replaced credential must not be served")
	assert.Equal(t, []string{"old-secret", "new-secret"}, builtWith)
}

// A replica that waited on the org lock while another provisioned must reuse
// what the other one stored, not provision again: provisioning again would
// regenerate the Thunder secret and invalidate the stored one.
func TestGetOCClientForOrg_ReusesCredentialStoredWhileWaitingForOrgLock(t *testing.T) {
	encrypted, err := utils.EncryptBytes([]byte("scheduler-secret"), testEncryptionKey)
	require.NoError(t, err)
	reads := 0
	thunderMock := &clientmocks.ThunderClientMock{}
	p := &publisherCredentialProvisioner{
		thunderClient: thunderMock,
		schedulerCredRepo: &repomocks.OrgSchedulerCredentialRepositoryMock{
			GetByOrgNameFunc: func(string) (*models.OrgSchedulerCredential, error) {
				reads++
				if reads == 1 {
					return nil, gorm.ErrRecordNotFound // before the lock: nothing stored yet
				}
				return &models.OrgSchedulerCredential{ClientID: "amp-scheduler-acme", ClientSecretEncrypted: encrypted}, nil
			},
		},
		orgCredLocker: &fakeOrgCredentialLocker{},
		logger:        discardLogger(),
		encryptionKey: testEncryptionKey,
		idpTokenURL:   "http://thunder.test/oauth2/token",
		ocBaseURL:     "http://openchoreo.test",
	}

	_, err = p.GetOCClientForOrg(context.Background(), "acme")
	require.NoError(t, err)
	assert.Empty(t, thunderMock.EnsureAppCalls(), "a credential stored by another replica must be reused")
}

// Backfilling a missing encrypted secret regenerates the Thunder secret, so it
// must run under the org lock like provisioning.
func TestGetOCClientForOrg_BackfillsSecretUnderOrgLock(t *testing.T) {
	locker := &fakeOrgCredentialLocker{}
	stored := &models.OrgSchedulerCredential{ClientID: "amp-scheduler-acme"}
	var heldDuringRegenerate bool
	p := &publisherCredentialProvisioner{
		thunderClient: &clientmocks.ThunderClientMock{
			RegenerateAppClientSecretFunc: func(_ context.Context, appName string) (string, error) {
				heldDuringRegenerate = locker.held
				assert.Equal(t, "amp-scheduler-acme", appName)
				return "regenerated-secret", nil
			},
		},
		secretClient: &clientmocks.SecretManagementClientMock{
			PatchSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string, []string) (string, error) {
				return "ref", nil
			},
		},
		schedulerCredRepo: &repomocks.OrgSchedulerCredentialRepositoryMock{
			GetByOrgNameFunc: func(string) (*models.OrgSchedulerCredential, error) {
				copied := *stored
				return &copied, nil
			},
			UpsertFunc: func(cred *models.OrgSchedulerCredential) error {
				stored = cred
				return nil
			},
		},
		orgCredLocker: locker,
		logger:        discardLogger(),
		encryptionKey: testEncryptionKey,
		idpTokenURL:   "http://thunder.test/oauth2/token",
		ocBaseURL:     "http://openchoreo.test",
	}

	_, err := p.GetOCClientForOrg(context.Background(), "acme")
	require.NoError(t, err)
	assert.True(t, heldDuringRegenerate, "the Thunder secret must be regenerated under the org lock")
	assert.NotEmpty(t, stored.ClientSecretEncrypted)
	assert.False(t, locker.held)
}

// Checking that the cached client is current reads the database on every call.
// A failed read must keep serving a cached client, as before the check existed,
// and fail only when there is nothing cached to serve.
func TestGetOCClientForOrg_DBErrorServesCachedClient(t *testing.T) {
	encrypted, err := utils.EncryptBytes([]byte("scheduler-secret"), testEncryptionKey)
	require.NoError(t, err)
	dbErr := errors.New("db timeout")
	failReads := false
	p := &publisherCredentialProvisioner{
		schedulerCredRepo: &repomocks.OrgSchedulerCredentialRepositoryMock{
			GetByOrgNameFunc: func(string) (*models.OrgSchedulerCredential, error) {
				if failReads {
					return nil, dbErr
				}
				return &models.OrgSchedulerCredential{ClientID: "amp-scheduler-acme", ClientSecretEncrypted: encrypted}, nil
			},
		},
		logger:        discardLogger(),
		encryptionKey: testEncryptionKey,
		idpTokenURL:   "http://thunder.test/oauth2/token",
		ocBaseURL:     "http://openchoreo.test",
	}

	cached, err := p.GetOCClientForOrg(context.Background(), "acme")
	require.NoError(t, err)

	failReads = true
	served, err := p.GetOCClientForOrg(context.Background(), "acme")
	require.NoError(t, err, "a cached client must still be served when the read fails")
	assert.Same(t, cached, served)

	_, err = p.GetOCClientForOrg(context.Background(), "other-org")
	require.ErrorIs(t, err, dbErr, "with nothing cached, the read error must surface")
}

// A caller that read a newer credential must not join a build from an older one,
// so the build key follows the stored credential, without carrying its secret.
func TestOrgOCBuildKey_FollowsStoredCredential(t *testing.T) {
	cred := &models.OrgSchedulerCredential{ClientID: "amp-scheduler-acme", ClientSecretEncrypted: []byte("encrypted-v1")}
	same := &models.OrgSchedulerCredential{ClientID: "amp-scheduler-acme", ClientSecretEncrypted: []byte("encrypted-v1")}
	rotated := &models.OrgSchedulerCredential{ClientID: "amp-scheduler-acme", ClientSecretEncrypted: []byte("encrypted-v2")}
	otherClient := &models.OrgSchedulerCredential{ClientID: "amp-scheduler-other", ClientSecretEncrypted: []byte("encrypted-v1")}

	assert.Equal(t, orgOCBuildKey("acme", cred), orgOCBuildKey("acme", same), "callers that read the same credential share a build")
	assert.NotEqual(t, orgOCBuildKey("acme", cred), orgOCBuildKey("acme", rotated), "a rotated secret needs its own build")
	assert.NotEqual(t, orgOCBuildKey("acme", cred), orgOCBuildKey("acme", otherClient))
	assert.NotEqual(t, orgOCBuildKey("acme", cred), orgOCBuildKey("globex", cred), "orgs never share a build")
	assert.NotContains(t, orgOCBuildKey("acme", cred), "encrypted-v1", "the key must not carry the encrypted secret")

	repair := orgOCBuildKey("acme", nil)
	assert.Equal(t, repair, orgOCBuildKey("acme", &models.OrgSchedulerCredential{ClientID: "amp-scheduler-acme"}),
		"every build that must repair the credential shares one key per org")
	assert.NotEqual(t, repair, orgOCBuildKey("acme", cred))
}

// A build from the old credential can still be in flight when another replica
// rotates the secret. A caller that read the rotated credential must build its
// own client instead of joining, and waiting for, the stale build.
func TestGetOCClientForOrg_DoesNotJoinABuildFromAnOlderCredential(t *testing.T) {
	oldSecret, err := utils.EncryptBytes([]byte("old-secret"), testEncryptionKey)
	require.NoError(t, err)
	newSecret, err := utils.EncryptBytes([]byte("new-secret"), testEncryptionKey)
	require.NoError(t, err)

	var mu sync.Mutex
	stored := &models.OrgSchedulerCredential{ClientID: "amp-scheduler-acme", ClientSecretEncrypted: oldSecret}
	oldBuildStarted := make(chan struct{})
	releaseOldBuild := make(chan struct{})
	SetOrgClientAuthProviderFactory(func(_, _, clientSecret string) occlient.AuthProvider {
		if clientSecret == "old-secret" {
			close(oldBuildStarted)
			<-releaseOldBuild
		}
		return &recordedAuthProvider{clientSecret: clientSecret}
	})
	t.Cleanup(func() { SetOrgClientAuthProviderFactory(nil) })

	p := &publisherCredentialProvisioner{
		schedulerCredRepo: &repomocks.OrgSchedulerCredentialRepositoryMock{
			GetByOrgNameFunc: func(string) (*models.OrgSchedulerCredential, error) {
				mu.Lock()
				defer mu.Unlock()
				copied := *stored
				return &copied, nil
			},
		},
		logger:        discardLogger(),
		encryptionKey: testEncryptionKey,
		idpTokenURL:   "http://thunder.test/oauth2/token",
		ocBaseURL:     "http://openchoreo.test",
	}

	oldDone := make(chan struct{})
	go func() {
		defer close(oldDone)
		_, _ = p.GetOCClientForOrg(context.Background(), "acme")
	}()
	<-oldBuildStarted

	mu.Lock()
	stored = &models.OrgSchedulerCredential{ClientID: "amp-scheduler-acme", ClientSecretEncrypted: newSecret}
	mu.Unlock()

	newDone := make(chan error, 1)
	go func() {
		_, err := p.GetOCClientForOrg(context.Background(), "acme")
		newDone <- err
	}()
	select {
	case err := <-newDone:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		close(releaseOldBuild)
		<-oldDone
		t.Fatal("a caller that read the rotated credential joined the build from the old one")
	}
	close(releaseOldBuild)
	<-oldDone
}
