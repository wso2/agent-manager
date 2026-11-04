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
	"database/sql"
	"database/sql/driver"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

const proxySyncUpstreamHeader = "X-Upstream-Key"

// noopTxPool lets the core's write-back transactions begin and commit without a database; every write inside them
// goes to a mocked repository.
type noopTxPool struct{ gorm.ConnPool }

func (*noopTxPool) BeginTx(context.Context, *sql.TxOptions) (gorm.ConnPool, error) {
	return &noopTx{}, nil
}

type noopTx struct{ gorm.ConnPool }

func (*noopTx) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return driver.RowsAffected(1), nil
}
func (*noopTx) Commit() error   { return nil }
func (*noopTx) Rollback() error { return nil }

// proxySyncFixture is syncProxyAuthHeader's collaborators, mocked down to the repositories: the proxy store, the
// render queue and the API-key store a minted key is revoked from.
type proxySyncFixture struct {
	proxy        *models.LLMProxy
	provider     *models.LLMProvider
	proxyRepo    *repomocks.LLMProxyRepositoryMock
	deployRepo   *repomocks.DeploymentRepositoryMock
	revokedKeys  []string
	writtenBack  []uuid.UUID
	savedConfigs []models.LLMProxyConfig
	svc          *LLMProviderService
	proxySvc     *LLMProxyService
	deploySvc    *LLMProxyDeploymentService
}

// newProxySyncFixture is a proxy with no upstream credential in front of a secured provider, so the sync mints one.
func newProxySyncFixture(t *testing.T) *proxySyncFixture {
	t.Helper()
	f := &proxySyncFixture{proxy: goldenProxy(), provider: goldenProvider()}
	f.proxy.Configuration.UpstreamAuth = nil
	stored := *f.proxy

	providerRepo := &repomocks.LLMProviderRepositoryMock{
		GetByUUIDFunc: func(string, string) (*models.LLMProvider, error) { return f.provider, nil },
	}
	f.proxyRepo = &repomocks.LLMProxyRepositoryMock{
		GetByIDFunc: func(string, string) (*models.LLMProxy, error) {
			current := stored
			return &current, nil
		},
		UpdateFunc: func(p *models.LLMProxy, _, _ string) error {
			f.savedConfigs = append(f.savedConfigs, p.Configuration)
			return nil
		},
	}
	gatewayRepo := &repomocks.GatewayRepositoryMock{
		GetByOrganizationIDFunc: func(context.Context, string) ([]*models.Gateway, error) {
			return []*models.Gateway{{UUID: uuid.New()}}, nil
		},
	}
	apiKeyRepo := &repomocks.APIKeyRepositoryMock{
		UpsertFunc: func(*models.StoredAPIKey) error { return nil },
		DeleteFunc: func(_, name string) error {
			f.revokedKeys = append(f.revokedKeys, name)
			return nil
		},
	}
	f.deployRepo = &repomocks.DeploymentRepositoryMock{}

	database, err := gorm.Open(postgres.New(postgres.Config{Conn: &noopTxPool{}}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)
	kind := goldenProxyKind(f.proxy)
	core := reconcile.NewCore(database, f.deployRepo, &stubEventHub{}, reconcile.Kinds{kind},
		reconcile.Config{Lease: time.Minute, AttemptBudget: 3}, slog.New(slog.DiscardHandler))

	encryptionKey := make([]byte, 32)
	apiKeySvc := NewLLMProviderAPIKeyService(providerRepo, gatewayRepo, NewGatewayEventsService(&stubEventHub{}), apiKeyRepo)
	f.svc = NewLLMProviderService(nil, providerRepo, nil, nil, f.proxyRepo, nil, encryptionKey, gatewayRepo, f.deployRepo,
		nil, nil, apiKeySvc)
	f.proxySvc = NewLLMProxyService(f.proxyRepo, providerRepo, encryptionKey)
	f.deploySvc = NewLLMProxyDeploymentService(nil, f.deployRepo, f.proxyRepo, providerRepo, gatewayRepo, nil, core, kind)
	return f
}

// placeOnGateways queues the proxy on gateways and lets the write-back of each succeed or fail as writeBack says.
func (f *proxySyncFixture) placeOnGateways(gateways []uuid.UUID, writeBack func(gateway uuid.UUID) error) {
	f.deployRepo.QueueRenderFunc = func(context.Context, *gorm.DB, string, uuid.UUID, []uuid.UUID, string, repositories.QueueScope) (int64, error) {
		return int64(len(gateways)), nil
	}
	f.deployRepo.ClaimDueRendersOfFunc = func(_ context.Context, ouID string, artifact uuid.UUID, _ []uuid.UUID, now time.Time, _ time.Duration) ([]models.ClaimedRender, error) {
		claims := make([]models.ClaimedRender, 0, len(gateways))
		for _, gateway := range gateways {
			claims = append(claims, models.ClaimedRender{
				Placement: models.Placement{
					ArtifactRef: models.ArtifactRef{OUID: ouID, ArtifactUUID: artifact, Kind: models.KindLLMProxy, Handle: f.proxy.Handle},
					GatewayUUID: gateway,
				},
				Token: now, ClaimedAt: now,
			})
		}
		return claims, nil
	}
	f.deployRepo.CompleteRenderFunc = func(_ context.Context, _ *gorm.DB, claim models.ClaimedRender, deployment *models.Deployment) error {
		if err := writeBack(claim.GatewayUUID); err != nil {
			return err
		}
		now := time.Now()
		deployment.DeploymentID = uuid.New()
		deployment.UpdatedAt = &now
		f.writtenBack = append(f.writtenBack, claim.GatewayUUID)
		return nil
	}
}

func (f *proxySyncFixture) sync(t *testing.T) (bool, error) {
	t.Helper()
	return f.svc.syncProxyAuthHeader(auditableCtx(t), f.proxy, "ou-acme", f.provider, proxySyncUpstreamHeader,
		f.proxySvc, f.deploySvc)
}

// Once saved, the stored headers are what every gateway renders from: a gateway that commits them before another
// fails must not be left serving headers the store no longer holds, nor lose the key it authenticates with.
func TestSyncProxyAuthHeaderKeepsHeadersWhenALaterGatewayFails(t *testing.T) {
	f := newProxySyncFixture(t)
	first, second := uuid.New(), uuid.New()
	f.placeOnGateways([]uuid.UUID{first, second}, func(gateway uuid.UUID) error {
		if gateway == second {
			return errors.New("connection reset")
		}
		return nil
	})

	_, err := f.sync(t)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "saved")
	assert.Contains(t, err.Error(), "recorded on each affected gateway")
	assert.NotErrorIs(t, err, reconcile.ErrQueueFailed)
	assert.Equal(t, []uuid.UUID{first}, f.writtenBack, "the first gateway's write-back committed")
	require.Len(t, f.savedConfigs, 1, "the saved headers are never rolled back")
	saved := f.savedConfigs[0]
	assert.Equal(t, newProxyIngressSecurity(f.provider), saved.Security)
	require.NotNil(t, saved.UpstreamAuth)
	assert.Equal(t, proxySyncUpstreamHeader, *saved.UpstreamAuth.Header)
	assert.NotNil(t, saved.UpstreamAuth.SecretRef)
	assert.Empty(t, f.revokedKeys, "the first gateway authenticates with the minted key")
}

// A queue failure leaves no redeploy owed, which the caller is told; the saved headers still stand.
func TestSyncProxyAuthHeaderKeepsHeadersWhenQueueingFails(t *testing.T) {
	f := newProxySyncFixture(t)
	f.deployRepo.QueueRenderFunc = func(context.Context, *gorm.DB, string, uuid.UUID, []uuid.UUID, string, repositories.QueueScope) (int64, error) {
		return 0, errors.New("connection reset")
	}

	_, err := f.sync(t)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not queued")
	assert.Len(t, f.savedConfigs, 1)
	assert.Empty(t, f.revokedKeys)
}

// A key minted for a proxy whose save failed has no owner, so it is handed back and nothing is redeployed.
func TestSyncProxyAuthHeaderRevokesTheKeyWhenTheSaveFails(t *testing.T) {
	f := newProxySyncFixture(t)
	f.proxyRepo.UpdateFunc = func(*models.LLMProxy, string, string) error { return errors.New("connection reset") }
	// QueueRenderFunc is nil: queueing a redeploy panics.

	_, err := f.sync(t)

	require.Error(t, err)
	assert.Equal(t, []string{f.proxy.Configuration.Name}, f.revokedKeys)
}

// The old per-gateway rollout's pending list is cleared by the one save, not a second write after the redeploy.
func TestSyncProxyAuthHeaderClearsPendingGatewaysInTheSave(t *testing.T) {
	f := newProxySyncFixture(t)
	f.proxy.Configuration.AuthRolloutPendingGateways = []string{"g"}
	f.placeOnGateways(nil, func(uuid.UUID) error { return nil })

	changed, err := f.sync(t)

	require.NoError(t, err)
	assert.True(t, changed)
	require.Len(t, f.savedConfigs, 1)
	assert.NotNil(t, f.savedConfigs[0].AuthRolloutPendingGateways)
	assert.Empty(t, f.savedConfigs[0].AuthRolloutPendingGateways)
}
