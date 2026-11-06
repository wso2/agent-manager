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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const bcOrg = "6f1e2c3d-0000-4000-8000-000000000001"

func TestNewBuildCredentialProvisioner_DisabledIsNil(t *testing.T) {
	p := NewBuildCredentialProvisioner(config.Config{}, discardLogger(), nil, nil, nil)
	assert.Nil(t, p)
}

// lockingRepo returns an OrgBuildCredentialRepositoryMock whose provisioning lock
// runs fn directly, recording that it was taken.
func lockingRepo(repo *repomocks.OrgBuildCredentialRepositoryMock, locked *bool) *repomocks.OrgBuildCredentialRepositoryMock {
	repo.WithProvisioningLockFunc = func(_ context.Context, _ string, fn func() error) error {
		if locked != nil {
			*locked = true
		}
		return fn()
	}
	return repo
}

func bothEntries() *client.SecretReferenceInfo {
	return &client.SecretReferenceInfo{Data: []client.SecretDataSourceInfo{
		{SecretKey: "client-id", RemoteRef: client.RemoteRefInfo{Key: "kv/org/amp-build", Property: "client-id"}},
		{SecretKey: "client-secret", RemoteRef: client.RemoteRefInfo{Key: "kv/org/amp-build", Property: "client-secret"}},
	}}
}

func TestBuildCredentialProvisioner_ReturnsStoredClientWithoutProvisioning(t *testing.T) {
	repo := &repomocks.OrgBuildCredentialRepositoryMock{
		GetByOrgNameFunc: func(ouID string) (*models.OrgBuildCredential, error) {
			return &models.OrgBuildCredential{OUID: ouID, ClientID: "amp-build-" + ouID, SecretRefName: "amp-build-ref"}, nil
		},
	}
	oc := &clientmocks.OpenChoreoClientMock{
		GetSecretReferenceFunc: func(_ context.Context, _, name string) (*client.SecretReferenceInfo, error) {
			assert.Equal(t, "amp-build-ref", name, "the stored SecretReference is checked before it is used")
			return bothEntries(), nil
		},
	}
	// Lock, Thunder and secret store funcs are nil: provisioning must not happen.
	p := newBuildCredentialProvisioner(&clientmocks.ThunderClientMock{}, &clientmocks.SecretManagementClientMock{},
		oc, repo, discardLogger())

	got, err := p.EnsureBuildClient(context.Background(), bcOrg)
	require.NoError(t, err)
	assert.Equal(t, &client.BuildClientParams{SecretRefName: "amp-build-ref"}, got)
	assert.Len(t, oc.GetSecretReferenceCalls(), 1)
}

// staleStoredRepo stores a build client whose SecretReference is staleRef until
// provisioning replaces it; upserted records the replacement.
func staleStoredRepo(staleRef string, upserted **models.OrgBuildCredential) *repomocks.OrgBuildCredentialRepositoryMock {
	return lockingRepo(&repomocks.OrgBuildCredentialRepositoryMock{
		GetByOrgNameFunc: func(ouID string) (*models.OrgBuildCredential, error) {
			if *upserted != nil {
				return *upserted, nil
			}
			return &models.OrgBuildCredential{OUID: ouID, ClientID: "amp-build-" + ouID, SecretRefName: staleRef}, nil
		},
		UpsertFunc: func(cred *models.OrgBuildCredential) error {
			*upserted = cred
			return nil
		},
	}, nil)
}

// reprovisioningThunder answers for a build client app that already exists:
// Thunder does not return its secret again, so the provisioner regenerates it.
func reprovisioningThunder(regenerated *bool) *clientmocks.ThunderClientMock {
	return &clientmocks.ThunderClientMock{
		EnsureAppFunc: func(_ context.Context, appName, _ string) (string, string, bool, error) {
			return appName, "", false, nil
		},
		RegenerateAppClientSecretFunc: func(context.Context, string) (string, error) {
			*regenerated = true
			return "new-secret", nil
		},
	}
}

func newSecretStore() *clientmocks.SecretManagementClientMock {
	return &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
			return "new-ref", nil
		},
	}
}

func TestBuildCredentialProvisioner_ProvisionsAgainWhenTheStoredSecretReferenceIsGone(t *testing.T) {
	var upserted *models.OrgBuildCredential
	regenerated := false
	oc := &clientmocks.OpenChoreoClientMock{
		GetSecretReferenceFunc: func(_ context.Context, _, name string) (*client.SecretReferenceInfo, error) {
			if name == "old-ref" {
				return nil, fmt.Errorf("%w: secret reference %s", utils.ErrNotFound, name)
			}
			return bothEntries(), nil
		},
	}
	p := newBuildCredentialProvisioner(reprovisioningThunder(&regenerated), newSecretStore(), oc,
		staleStoredRepo("old-ref", &upserted), discardLogger())

	got, err := p.EnsureBuildClient(auditableCtx(t), bcOrg)
	require.NoError(t, err)
	assert.Equal(t, &client.BuildClientParams{SecretRefName: "new-ref"}, got)
	assert.True(t, regenerated, "the existing app's secret is regenerated, since the stored one is lost")
	require.NotNil(t, upserted, "the stale row is replaced")
	assert.Equal(t, "new-ref", upserted.SecretRefName)
}

func TestBuildCredentialProvisioner_ProvisionsAgainWhenTheStoredSecretReferenceLostAnEntry(t *testing.T) {
	var upserted *models.OrgBuildCredential
	regenerated := false
	oc := &clientmocks.OpenChoreoClientMock{
		GetSecretReferenceFunc: func(_ context.Context, _, name string) (*client.SecretReferenceInfo, error) {
			if name == "old-ref" {
				return &client.SecretReferenceInfo{Data: bothEntries().Data[:1]}, nil // no client-secret
			}
			return bothEntries(), nil
		},
	}
	p := newBuildCredentialProvisioner(reprovisioningThunder(&regenerated), newSecretStore(), oc,
		staleStoredRepo("old-ref", &upserted), discardLogger())

	got, err := p.EnsureBuildClient(auditableCtx(t), bcOrg)
	require.NoError(t, err)
	assert.Equal(t, &client.BuildClientParams{SecretRefName: "new-ref"}, got)
	require.NotNil(t, upserted)
	assert.Equal(t, "new-ref", upserted.SecretRefName)
}

func TestBuildCredentialProvisioner_KeepsTheStoredClientWhenItCannotBeChecked(t *testing.T) {
	repo := &repomocks.OrgBuildCredentialRepositoryMock{
		GetByOrgNameFunc: func(ouID string) (*models.OrgBuildCredential, error) {
			return &models.OrgBuildCredential{OUID: ouID, SecretRefName: "amp-build-ref"}, nil
		},
	}
	oc := &clientmocks.OpenChoreoClientMock{
		GetSecretReferenceFunc: func(context.Context, string, string) (*client.SecretReferenceInfo, error) {
			return nil, fmt.Errorf("%w: upstream unavailable", utils.ErrInternalServerError)
		},
	}
	// Lock, Thunder and secret store funcs are nil: a transient failure must not
	// regenerate a secret that may still work.
	p := newBuildCredentialProvisioner(&clientmocks.ThunderClientMock{}, &clientmocks.SecretManagementClientMock{},
		oc, repo, discardLogger())

	got, err := p.EnsureBuildClient(context.Background(), bcOrg)
	require.NoError(t, err)
	assert.Equal(t, &client.BuildClientParams{SecretRefName: "amp-build-ref"}, got)
}

func TestBuildCredentialProvisioner_ProvisionsOnFirstUseUnderTheLock(t *testing.T) {
	var upserted *models.OrgBuildCredential
	var storedAt secretmanagersvc.SecretLocation
	var storedData map[string]string
	locked := false
	repo := lockingRepo(&repomocks.OrgBuildCredentialRepositoryMock{
		GetByOrgNameFunc: func(string) (*models.OrgBuildCredential, error) { return nil, gorm.ErrRecordNotFound },
		UpsertFunc: func(cred *models.OrgBuildCredential) error {
			upserted = cred
			return nil
		},
	}, &locked)
	thunder := &clientmocks.ThunderClientMock{
		EnsureAppFunc: func(_ context.Context, appName, orgUUID string) (string, string, bool, error) {
			assert.True(t, locked, "the client must be provisioned under the per-organization lock")
			assert.Equal(t, "amp-build-"+bcOrg, appName)
			assert.Equal(t, bcOrg, orgUUID, "the client must be created in the organization's own OU")
			return appName, "s3cret", true, nil
		},
	}
	secrets := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(_ context.Context, location secretmanagersvc.SecretLocation, data map[string]string) (string, error) {
			storedAt, storedData = location, data
			return "amp-build-ref", nil
		},
	}
	oc := &clientmocks.OpenChoreoClientMock{
		GetSecretReferenceFunc: func(_ context.Context, _, name string) (*client.SecretReferenceInfo, error) {
			assert.Equal(t, "amp-build-ref", name)
			return bothEntries(), nil
		},
	}
	p := newBuildCredentialProvisioner(thunder, secrets, oc, repo, discardLogger())

	got, err := p.EnsureBuildClient(auditableCtx(t), bcOrg)
	require.NoError(t, err)
	assert.Equal(t, &client.BuildClientParams{SecretRefName: "amp-build-ref"}, got)

	assert.Equal(t, bcOrg, storedAt.OrgName)
	assert.Equal(t, "amp-build-"+bcOrg, storedAt.EntityName)
	assert.Equal(t, map[string]string{"client-id": "amp-build-" + bcOrg, "client-secret": "s3cret"}, storedData)

	require.NotNil(t, upserted)
	assert.Equal(t, bcOrg, upserted.OUID)
	assert.Equal(t, "amp-build-"+bcOrg, upserted.ClientID)
	assert.Equal(t, "amp-build-ref", upserted.SecretRefName)
}

func TestBuildCredentialProvisioner_UsesTheClientAnotherReplicaStoredWhileWaiting(t *testing.T) {
	lookups := 0
	repo := lockingRepo(&repomocks.OrgBuildCredentialRepositoryMock{
		GetByOrgNameFunc: func(ouID string) (*models.OrgBuildCredential, error) {
			lookups++
			if lookups == 1 {
				return nil, gorm.ErrRecordNotFound // before the lock
			}
			return &models.OrgBuildCredential{OUID: ouID, SecretRefName: "stored-by-other-replica"}, nil
		},
	}, nil)
	oc := &clientmocks.OpenChoreoClientMock{
		GetSecretReferenceFunc: func(context.Context, string, string) (*client.SecretReferenceInfo, error) {
			return bothEntries(), nil
		},
	}
	// Thunder funcs are nil: the re-check under the lock must stop provisioning.
	p := newBuildCredentialProvisioner(&clientmocks.ThunderClientMock{}, nil, oc, repo, discardLogger())

	got, err := p.EnsureBuildClient(auditableCtx(t), bcOrg)
	require.NoError(t, err)
	assert.Equal(t, &client.BuildClientParams{SecretRefName: "stored-by-other-replica"}, got)
}

func TestBuildCredentialProvisioner_RegeneratesTheSecretOfAnExistingApp(t *testing.T) {
	regenerated := false
	repo := lockingRepo(&repomocks.OrgBuildCredentialRepositoryMock{
		GetByOrgNameFunc: func(string) (*models.OrgBuildCredential, error) { return nil, gorm.ErrRecordNotFound },
		UpsertFunc:       func(*models.OrgBuildCredential) error { return nil },
	}, nil)
	thunder := &clientmocks.ThunderClientMock{
		EnsureAppFunc: func(_ context.Context, appName, _ string) (string, string, bool, error) {
			return appName, "", false, nil
		},
		RegenerateAppClientSecretFunc: func(_ context.Context, appName string) (string, error) {
			regenerated = true
			assert.Equal(t, "amp-build-"+bcOrg, appName)
			return "new-secret", nil
		},
	}
	secrets := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(_ context.Context, _ secretmanagersvc.SecretLocation, data map[string]string) (string, error) {
			assert.Equal(t, "new-secret", data["client-secret"])
			return "ref", nil
		},
	}
	oc := &clientmocks.OpenChoreoClientMock{
		GetSecretReferenceFunc: func(context.Context, string, string) (*client.SecretReferenceInfo, error) {
			return bothEntries(), nil
		},
	}
	p := newBuildCredentialProvisioner(thunder, secrets, oc, repo, discardLogger())

	_, err := p.EnsureBuildClient(auditableCtx(t), bcOrg)
	require.NoError(t, err)
	assert.True(t, regenerated)
}

type stubBuildCredentials struct {
	params *client.BuildClientParams
	err    error
}

func (s stubBuildCredentials) EnsureBuildClient(context.Context, string) (*client.BuildClientParams, error) {
	return s.params, s.err
}

// countingBuildCredentials counts EnsureBuildClient calls.
type countingBuildCredentials struct {
	calls *int
}

func (s countingBuildCredentials) EnsureBuildClient(context.Context, string) (*client.BuildClientParams, error) {
	*s.calls++
	return &client.BuildClientParams{SecretRefName: "amp-build-ref"}, nil
}

func TestAgentManagerService_BuildClientFor(t *testing.T) {
	t.Run("disabled: no build client", func(t *testing.T) {
		s := &agentManagerService{}
		assert.Nil(t, s.buildClientFor(bcOrg))
	})
	t.Run("enabled: nothing is provisioned until the source is called", func(t *testing.T) {
		calls := 0
		s := &agentManagerService{buildCredentials: countingBuildCredentials{calls: &calls}}
		source := s.buildClientFor(bcOrg)
		require.NotNil(t, source)
		assert.Zero(t, calls, "provisioning waits for TriggerBuild to decide the workflow takes the client")

		got, err := source(context.Background())
		require.NoError(t, err)
		assert.Equal(t, &client.BuildClientParams{SecretRefName: "amp-build-ref"}, got)
		assert.Equal(t, 1, calls)
	})
	t.Run("a provisioning failure reaches TriggerBuild", func(t *testing.T) {
		cause := errors.New("thunder down")
		s := &agentManagerService{buildCredentials: stubBuildCredentials{err: cause}}
		_, err := s.buildClientFor(bcOrg)(context.Background())
		require.ErrorIs(t, err, cause)
	})
}

func TestBuildCredentialProvisioner_Failures(t *testing.T) {
	notFound := func(string) (*models.OrgBuildCredential, error) { return nil, gorm.ErrRecordNotFound }
	createdApp := func(_ context.Context, appName, _ string) (string, string, bool, error) {
		return appName, "s", true, nil
	}
	storedSecret := func(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
		return "ref", nil
	}

	t.Run("a real lookup error is not treated as no client", func(t *testing.T) {
		dbErr := errors.New("connection refused")
		repo := &repomocks.OrgBuildCredentialRepositoryMock{
			GetByOrgNameFunc: func(string) (*models.OrgBuildCredential, error) { return nil, dbErr },
		}
		p := newBuildCredentialProvisioner(&clientmocks.ThunderClientMock{}, nil, nil, repo, discardLogger())
		_, err := p.EnsureBuildClient(auditableCtx(t), bcOrg)
		require.ErrorIs(t, err, dbErr)
	})

	t.Run("refuses to issue a credential without an audit trail", func(t *testing.T) {
		repo := lockingRepo(&repomocks.OrgBuildCredentialRepositoryMock{GetByOrgNameFunc: notFound}, nil)
		// EnsureAppFunc nil: the credential must not be created.
		p := newBuildCredentialProvisioner(&clientmocks.ThunderClientMock{}, nil, nil, repo, discardLogger())
		_, err := p.EnsureBuildClient(context.Background(), bcOrg)
		require.ErrorIs(t, err, audit.ErrRecorderUnavailable)
	})

	t.Run("an app with another client ID is not adopted", func(t *testing.T) {
		repo := lockingRepo(&repomocks.OrgBuildCredentialRepositoryMock{GetByOrgNameFunc: notFound}, nil)
		thunder := &clientmocks.ThunderClientMock{
			EnsureAppFunc: func(context.Context, string, string) (string, string, bool, error) {
				return "someone-elses-client", "s", false, nil
			},
		}
		p := newBuildCredentialProvisioner(thunder, nil, nil, repo, discardLogger())
		_, err := p.EnsureBuildClient(auditableCtx(t), bcOrg)
		require.Error(t, err)
	})

	t.Run("a SecretReference missing an entry is not persisted", func(t *testing.T) {
		repo := lockingRepo(&repomocks.OrgBuildCredentialRepositoryMock{GetByOrgNameFunc: notFound}, nil) // UpsertFunc nil
		oc := &clientmocks.OpenChoreoClientMock{
			GetSecretReferenceFunc: func(context.Context, string, string) (*client.SecretReferenceInfo, error) {
				return &client.SecretReferenceInfo{Data: []client.SecretDataSourceInfo{
					{SecretKey: "client-secret", RemoteRef: client.RemoteRefInfo{Key: "kv"}},
				}}, nil
			},
		}
		p := newBuildCredentialProvisioner(&clientmocks.ThunderClientMock{EnsureAppFunc: createdApp},
			&clientmocks.SecretManagementClientMock{CreateSecretFunc: storedSecret}, oc, repo, discardLogger())
		_, err := p.EnsureBuildClient(auditableCtx(t), bcOrg)
		require.Error(t, err)
	})

	t.Run("a cancelled first caller does not abort the shared provisioning run", func(t *testing.T) {
		repo := lockingRepo(&repomocks.OrgBuildCredentialRepositoryMock{
			GetByOrgNameFunc: notFound,
			UpsertFunc:       func(*models.OrgBuildCredential) error { return nil },
		}, nil)
		thunder := &clientmocks.ThunderClientMock{
			EnsureAppFunc: func(ctx context.Context, appName, _ string) (string, string, bool, error) {
				require.NoError(t, ctx.Err(), "the provisioning run must not inherit the caller's cancellation")
				return appName, "s", true, nil
			},
		}
		oc := &clientmocks.OpenChoreoClientMock{
			GetSecretReferenceFunc: func(context.Context, string, string) (*client.SecretReferenceInfo, error) { return bothEntries(), nil },
		}
		p := newBuildCredentialProvisioner(thunder, &clientmocks.SecretManagementClientMock{CreateSecretFunc: storedSecret}, oc, repo, discardLogger())
		ctx, cancel := context.WithCancel(auditableCtx(t))
		cancel()
		_, err := p.EnsureBuildClient(ctx, bcOrg)
		require.NoError(t, err)
	})

	t.Run("no organization", func(t *testing.T) {
		p := newBuildCredentialProvisioner(nil, nil, nil, &repomocks.OrgBuildCredentialRepositoryMock{}, discardLogger())
		_, err := p.EnsureBuildClient(auditableCtx(t), "")
		require.Error(t, err)
	})
}
