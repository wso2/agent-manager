//go:build integration

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
	"log/slog"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// providerFixture is an org with one egress gateway and the provider services over the real repositories.
type providerFixture struct {
	sync       *syncFixture
	hub        *recordingEventHub
	providers  *LLMProviderService
	deployment *LLMProviderDeploymentService
}

func newProviderFixture(t *testing.T) *providerFixture {
	t.Helper()
	f := &providerFixture{sync: newSyncFixture(t), hub: &recordingEventHub{}}
	gormDB := db.GetDB()
	core := reconcile.NewCore(gormDB, f.sync.repo, f.hub, reconcile.Kinds{NewLLMProviderKind()}, reconcile.Config{},
		slog.New(slog.DiscardHandler))
	gateways := repositories.NewGatewayRepo(gormDB)
	providerRepo := repositories.NewLLMProviderRepo(gormDB)
	f.deployment = NewLLMProviderDeploymentService(NewTxRunner(gormDB), f.sync.repo, providerRepo, nil, gateways, nil, core)
	templates := NewLLMTemplateStore()
	templates.Load([]*models.LLMProviderTemplate{{Handle: "openai"}})
	f.providers = NewLLMProviderService(gormDB, providerRepo, repositories.NewLLMProviderTemplateRepo(gormDB),
		templates, nil, nil, make([]byte, 32), gateways, f.sync.repo, nil, nil, nil)
	return f
}

func (f *providerFixture) newProvider(t *testing.T) *models.LLMProvider {
	t.Helper()
	provider := goldenProvider()
	provider.UUID = uuid.Nil
	provider.Configuration.Handle = "p-" + uuid.NewString()[:8]
	provider.Configuration.Upstream.Main.Auth = nil
	t.Cleanup(func() {
		db.GetDB().Exec(`DELETE FROM artifacts WHERE handle = ? AND ou_id = ?`, provider.Configuration.Handle, f.sync.ouID)
	})
	return provider
}

func (f *providerFixture) artifactCount(t *testing.T, handle string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.GetDB().Table("artifacts").Where("handle = ? AND ou_id = ?", handle, f.sync.ouID).Count(&n).Error)
	return n
}

func TestCreateAndDeployCommitsTheProviderWithItsDeployment(t *testing.T) {
	f := newProviderFixture(t)
	provider := f.newProvider(t)

	got, err := f.providers.CreateAndDeploy(context.Background(), f.sync.ouID, "me", provider,
		[]string{f.sync.gateway.String()}, f.deployment)

	require.NoError(t, err)
	require.Len(t, got.Deployments, 1)
	assert.True(t, got.Deployments[0].Success)
	p := models.Placement{
		ArtifactRef: models.ArtifactRef{OUID: f.sync.ouID, ArtifactUUID: got.Provider.UUID, Kind: models.KindLLMProvider},
		GatewayUUID: f.sync.gateway,
	}
	row, err := f.sync.repo.GetStatusRecord(context.Background(), p)
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, models.DeploymentStatusDeploying, row.Status)
	pulled, err := f.sync.repo.GetPullable(context.Background(), f.sync.ouID, got.Provider.UUID, f.sync.gateway.String())
	require.NoError(t, err)
	assert.Contains(t, string(pulled.Content), "name: "+provider.Configuration.Handle)
	require.Len(t, f.hub.published, 1)
	assert.Equal(t, "llmprovider.deployed", string(f.hub.published[0].EventType))
}

// A provider no gateway can take is rolled back with its deployments: the transaction replaces the old
// delete-on-total-failure.
func TestCreateAndDeployRollsBackAProviderThatCannotRender(t *testing.T) {
	f := newProviderFixture(t)
	provider := f.newProvider(t)
	provider.Configuration.RateLimiting.ProviderLevel.Global.Token.Reset.Unit = "fortnight"

	_, err := f.providers.CreateAndDeploy(context.Background(), f.sync.ouID, "me", provider,
		[]string{f.sync.gateway.String()}, f.deployment)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "all 1 gateway deployments failed")
	assert.Zero(t, f.artifactCount(t, provider.Configuration.Handle), "nothing of the provider is committed")
	assert.Empty(t, f.hub.published)
}

func TestDeleteProviderSendsItsDeleteEventInTheDeletingTransaction(t *testing.T) {
	f := newProviderFixture(t)
	provider := f.newProvider(t)
	got, err := f.providers.CreateAndDeploy(context.Background(), f.sync.ouID, "me", provider,
		[]string{f.sync.gateway.String()}, f.deployment)
	require.NoError(t, err)

	require.NoError(t, f.providers.Delete(context.Background(), got.Provider.UUID.String(), f.sync.ouID, f.deployment))

	assert.Zero(t, f.artifactCount(t, provider.Configuration.Handle))
	last := f.hub.published[len(f.hub.published)-1]
	assert.Equal(t, "llmprovider.deleted", string(last.EventType))
	assert.Equal(t, f.sync.gateway.String(), last.GatewayID)
	assert.Equal(t, got.Provider.UUID.String(), last.EntityID)
}

// A delete reaches every active gateway of the org, not just the gateways the artifact's rows name: an artifact
// left with no rows (a switch to direct under an older release deleted them) is still dropped everywhere. The
// events go in gateway order.
func TestDeleteReachesEveryActiveGatewayOfTheOrg(t *testing.T) {
	f := newProviderFixture(t)
	other := f.sync.seedGateway(t, f.sync.ouID)
	inactive := f.sync.seedGateway(t, f.sync.ouID)
	require.NoError(t, db.GetDB().Exec(`UPDATE gateways SET is_active = FALSE WHERE uuid = ?`, inactive).Error)
	created, err := f.providers.Create(context.Background(), f.sync.ouID, "me", f.newProvider(t))
	require.NoError(t, err)

	require.NoError(t, f.providers.Delete(context.Background(), created.UUID.String(), f.sync.ouID, f.deployment))

	var addressed []string
	for _, evt := range f.hub.published {
		require.Equal(t, "llmprovider.deleted", string(evt.EventType))
		addressed = append(addressed, evt.GatewayID)
	}
	want := []string{f.sync.gateway.String(), other.String()}
	slices.Sort(want)
	assert.Equal(t, want, addressed)
}
