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
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// statusPathFixture is one artifact on one egress gateway, with a deployment repository that records the
// transition starters and answers GetStatusRecord with row.
type statusPathFixture struct {
	gateway    *models.Gateway
	deployment *models.Deployment
	row        models.DeploymentStatusRecord
	repo       *repomocks.DeploymentRepositoryMock
	hub        *recordingEventHub
}

func newStatusPathFixture(t *testing.T, artifactUUID uuid.UUID) *statusPathFixture {
	t.Helper()
	f := &statusPathFixture{
		gateway: &models.Gateway{UUID: uuid.New(), OUID: "ou-1", GatewayFunctionalityType: models.GatewayRoleEgress},
		hub:     &recordingEventHub{},
	}
	f.deployment = &models.Deployment{
		DeploymentID: uuid.New(), ArtifactUUID: artifactUUID, OUID: "ou-1", GatewayUUID: f.gateway.UUID,
		Content: []byte("kind: test\n"),
	}
	f.row = models.DeploymentStatusRecord{PerformedAt: models.NewPerformedAt(time.Now())}
	f.repo = &repomocks.DeploymentRepositoryMock{
		PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) { return nil, nil },
		GetWithContentFunc: func(string, string, string) (*models.Deployment, error) {
			return f.deployment, nil
		},
		GetWithStateFunc: func(string, string, string) (*models.Deployment, error) { return f.deployment, nil },
		GetWithStateCtxFunc: func(context.Context, string, string, string) (*models.Deployment, error) {
			return f.deployment, nil
		},
		StartDeployFunc: func(_ context.Context, tx *gorm.DB, _ models.Placement, d *models.Deployment) error {
			assert.Same(t, txSentinel, tx)
			deploying := models.DeploymentStatusDeploying
			d.Status, d.UpdatedAt = &deploying, &f.row.PerformedAt
			return nil
		},
		StartRestoreFunc: func(_ context.Context, tx *gorm.DB, _ models.Placement, _ uuid.UUID) error {
			assert.Same(t, txSentinel, tx)
			f.row.Status = models.DeploymentStatusDeploying
			return nil
		},
		BeginUndeployFunc: func(_ context.Context, tx *gorm.DB, _ models.Placement) (bool, error) {
			assert.Same(t, txSentinel, tx)
			sendEvent := f.row.Status != models.DeploymentStatusPending
			f.row.Status = models.DeploymentStatusUndeploying
			if !sendEvent {
				f.row.Status = models.DeploymentStatusUndeployed
			}
			return sendEvent, nil
		},
		GetStatusRecordFunc: func(context.Context, models.Placement) (*models.DeploymentStatusRecord, error) {
			row := f.row
			return &row, nil
		},
	}
	f.repo.WithTxFunc = func(tx *gorm.DB) repositories.DeploymentRepository {
		assert.Same(t, txSentinel, tx)
		return f.repo
	}
	return f
}

func (f *statusPathFixture) gatewayRepo() *repomocks.GatewayRepositoryMock {
	return &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(string) (*models.Gateway, error) { return f.gateway, nil },
	}
}

// payload decodes the one event the hub recorded, checking it was written in the caller's transaction.
func (f *statusPathFixture) payload(t *testing.T) (eventhub.Event, map[string]any) {
	t.Helper()
	require.Len(t, f.hub.published, 1)
	assert.Same(t, txSentinel, f.hub.txs[0], "the event is written in the status row's transaction")
	var dto struct {
		Payload map[string]any `json:"payload"`
	}
	require.NoError(t, json.Unmarshal([]byte(f.hub.published[0].EventData), &dto))
	return f.hub.published[0], dto.Payload
}

// core reconciles kinds against the fixture's repository and hub.
func (f *statusPathFixture) core(kinds ...reconcile.Kind) *reconcile.Core {
	return reconcile.NewCore(nil, f.repo, f.hub, kinds, reconcile.Config{}, slog.New(slog.DiscardHandler))
}

// providerKindOf renders provider whatever artifact it is asked for.
func providerKindOf(provider *models.LLMProvider) *LLMProviderKind {
	return &LLMProviderKind{load: func(context.Context, models.ArtifactRef) (*models.LLMProvider, error) {
		return provider, nil
	}}
}

// renderableProvider is the least a provider needs to render.
func renderableProvider() *models.LLMProvider {
	return &models.LLMProvider{
		UUID:           uuid.New(),
		TemplateHandle: "openai",
		Artifact:       &models.Artifact{Handle: "provider-handle"},
		Configuration: models.LLMProviderConfig{
			Handle: "provider-handle", Name: "Provider", Version: "v1.0",
			Upstream: &models.UpstreamConfig{Main: &models.UpstreamEndpoint{URL: "https://api.openai.com/v1"}},
		},
	}
}

func (f *statusPathFixture) providerService(provider *models.LLMProvider) *LLMProviderDeploymentService {
	return NewLLMProviderDeploymentService(sentinelTx, f.repo, &repomocks.LLMProviderRepositoryMock{
		GetByUUIDFunc:   func(string, string) (*models.LLMProvider, error) { return provider, nil },
		GetByHandleFunc: func(string, string) (*models.LLMProvider, error) { return provider, nil },
	}, nil, f.gatewayRepo(), NewGatewayEventsService(f.hub), f.core(providerKindOf(provider)))
}

// proxyKindOf renders proxy, fronting a renderable provider, whatever artifact it is asked for.
func proxyKindOf(proxy *models.LLMProxy) *LLMProxyKind {
	return &LLMProxyKind{
		load: func(context.Context, models.ArtifactRef) (*models.LLMProxy, *models.LLMProvider, error) {
			return proxy, renderableProvider(), nil
		},
		vhostOf: func(*gorm.DB, models.ArtifactRef) string { return proxyVhost(proxy) },
	}
}

func (f *statusPathFixture) proxyService(proxy *models.LLMProxy) *LLMProxyDeploymentService {
	kind := proxyKindOf(proxy)
	return NewLLMProxyDeploymentService(sentinelTx, f.repo, &repomocks.LLMProxyRepositoryMock{
		GetByIDFunc:    func(string, string) (*models.LLMProxy, error) { return proxy, nil },
		GetByIDCtxFunc: func(context.Context, string, string) (*models.LLMProxy, error) { return proxy, nil },
	}, nil, f.gatewayRepo(), NewGatewayEventsService(f.hub), f.core(kind), kind)
}

func assertPerformedAt(t *testing.T, want time.Time, payload map[string]any) {
	t.Helper()
	got, err := time.Parse(time.RFC3339Nano, payload["performedAt"].(string))
	require.NoError(t, err)
	assert.True(t, want.Equal(got), "performedAt %s is the row's %s", got, want)
}

// A provider addressed by handle is still sent to the gateway by UUID, which is what the gateway stores it under.
func TestDeployLLMProviderStartsDeployingAndSendsTheUUID(t *testing.T) {
	provider := renderableProvider()
	f := newStatusPathFixture(t, provider.UUID)

	got, err := f.providerService(provider).DeployLLMProvider("provider-handle", &models.DeployAPIRequest{
		Name: "ignored", Metadata: map[string]any{"ignored": true}, GatewayID: f.gateway.UUID.String(),
	}, "ou-1")

	require.NoError(t, err)
	assert.Equal(t, models.DeploymentStatusDeploying, *got.Status)
	assert.Regexp(t, `^provider-handle-[0-9a-f]{8}$`, got.Name, "the kind names the deployment")
	assert.Nil(t, got.Metadata)
	want, err := renderLLMProvider(provider)
	require.NoError(t, err)
	assert.Equal(t, want, string(got.Content), "the deployment holds the current configuration")
	require.Len(t, f.repo.StartDeployCalls(), 1)
	placement := f.repo.StartDeployCalls()[0].Placement
	assert.Equal(t, provider.UUID, placement.ArtifactUUID)
	assert.Equal(t, f.gateway.UUID, placement.GatewayUUID)
	event, payload := f.payload(t)
	assert.Equal(t, eventhub.EventType("llmprovider.deployed"), event.EventType)
	assert.Equal(t, provider.UUID.String(), payload["providerId"])
	assert.Equal(t, got.DeploymentID.String(), payload["deploymentId"])
	assertPerformedAt(t, f.row.PerformedAt, payload)
}

func TestDeployLLMProviderAcceptsOnlyTheCurrentBase(t *testing.T) {
	for _, base := range []string{"", "current"} {
		provider := renderableProvider()
		f := newStatusPathFixture(t, provider.UUID)
		_, err := f.providerService(provider).DeployLLMProvider("provider-handle", &models.DeployAPIRequest{
			Base: base, GatewayID: f.gateway.UUID.String(),
		}, "ou-1")
		assert.NoError(t, err, "base %q", base)
	}
	for _, base := range []string{uuid.NewString(), "not-a-uuid"} {
		provider := renderableProvider()
		f := newStatusPathFixture(t, provider.UUID)
		_, err := f.providerService(provider).DeployLLMProvider("provider-handle", &models.DeployAPIRequest{
			Base: base, GatewayID: f.gateway.UUID.String(),
		}, "ou-1")
		assert.ErrorIs(t, err, utils.ErrDeploymentBaseNotCurrent, "base %q", base)
		assert.Empty(t, f.repo.StartDeployCalls())
	}
}

func TestRestoreOfAnUnknownDeploymentIsNotFound(t *testing.T) {
	provider := renderableProvider()
	f := newStatusPathFixture(t, provider.UUID)
	f.repo.GetWithContentFunc = func(string, string, string) (*models.Deployment, error) {
		return nil, utils.ErrDeploymentNotFound
	}
	svc := f.providerService(provider)

	_, err := svc.RestoreLLMProviderDeployment("provider-handle", uuid.NewString(), f.gateway.UUID.String(), "ou-1")
	assert.ErrorIs(t, err, utils.ErrDeploymentNotFound)
	_, err = svc.RestoreLLMProviderDeployment("provider-handle", "not-a-uuid", f.gateway.UUID.String(), "ou-1")
	assert.ErrorIs(t, err, utils.ErrDeploymentNotFound)
	assert.Empty(t, f.repo.StartRestoreCalls())
}

func TestUndeployLLMProviderSendsAnEmptyDeploymentID(t *testing.T) {
	provider := &models.LLMProvider{UUID: uuid.New()}
	f := newStatusPathFixture(t, provider.UUID)
	deploying := models.DeploymentStatusDeploying
	f.deployment.Status = &deploying
	f.row.Status = deploying

	got, err := f.providerService(provider).UndeployLLMProviderDeployment("provider-handle",
		f.deployment.DeploymentID.String(), f.gateway.UUID.String(), "ou-1")

	require.NoError(t, err)
	assert.Equal(t, models.DeploymentStatusUndeploying, *got.Status)
	event, payload := f.payload(t)
	assert.Equal(t, eventhub.EventType("llmprovider.undeployed"), event.EventType)
	assert.Equal(t, provider.UUID.String(), payload["providerId"])
	assert.Contains(t, payload, "deploymentId")
	assert.Empty(t, payload["deploymentId"])
	assertPerformedAt(t, f.row.PerformedAt, payload)
}

func TestUndeployOfAnArchivedDeploymentIsNotActive(t *testing.T) {
	provider := &models.LLMProvider{UUID: uuid.New()}
	f := newStatusPathFixture(t, provider.UUID)
	archived := models.DeploymentStatusArchived
	f.deployment.Status = &archived

	_, err := f.providerService(provider).UndeployLLMProviderDeployment("provider-handle",
		f.deployment.DeploymentID.String(), f.gateway.UUID.String(), "ou-1")

	assert.ErrorIs(t, err, utils.ErrDeploymentNotActive)
	assert.Empty(t, f.repo.BeginUndeployCalls())
}

func TestRestoreLLMProviderSendsTheUUID(t *testing.T) {
	provider := &models.LLMProvider{UUID: uuid.New()}
	f := newStatusPathFixture(t, provider.UUID)
	f.row.Status = models.DeploymentStatusFailed

	got, err := f.providerService(provider).RestoreLLMProviderDeployment("provider-handle",
		f.deployment.DeploymentID.String(), f.gateway.UUID.String(), "ou-1")

	require.NoError(t, err)
	assert.Equal(t, models.DeploymentStatusDeploying, *got.Status)
	require.Len(t, f.repo.StartRestoreCalls(), 1)
	assert.Equal(t, f.deployment.DeploymentID, f.repo.StartRestoreCalls()[0].DeploymentID)
	event, payload := f.payload(t)
	assert.Equal(t, eventhub.EventType("llmprovider.deployed"), event.EventType)
	assert.Equal(t, provider.UUID.String(), payload["providerId"])
	assert.Equal(t, f.deployment.DeploymentID.String(), payload["deploymentId"])
	assertPerformedAt(t, f.row.PerformedAt, payload)
}

// The gateway stores the event's deploymentId and compares it with what GET /deployments lists, so a restore
// addressed by a non-canonical id still sends the canonical one.
func TestRestoreSendsTheCanonicalDeploymentID(t *testing.T) {
	t.Run("LLM provider", func(t *testing.T) {
		provider := &models.LLMProvider{UUID: uuid.New()}
		f := newStatusPathFixture(t, provider.UUID)

		_, err := f.providerService(provider).RestoreLLMProviderDeployment("provider-handle",
			strings.ToUpper(f.deployment.DeploymentID.String()), f.gateway.UUID.String(), "ou-1")

		require.NoError(t, err)
		_, payload := f.payload(t)
		assert.Equal(t, f.deployment.DeploymentID.String(), payload["deploymentId"])
	})
	t.Run("LLM proxy", func(t *testing.T) {
		proxy := &models.LLMProxy{UUID: uuid.New()}
		f := newStatusPathFixture(t, proxy.UUID)

		_, err := f.proxyService(proxy).RestoreLLMProxyDeployment("proxy-handle",
			strings.ToUpper(f.deployment.DeploymentID.String()), f.gateway.UUID.String(), "ou-1")

		require.NoError(t, err)
		_, payload := f.payload(t)
		assert.Equal(t, f.deployment.DeploymentID.String(), payload["deploymentId"])
	})
}

func TestDeployLLMProxyStartsDeployingAndSendsTheHandle(t *testing.T) {
	proxy := &models.LLMProxy{UUID: uuid.New()}
	f := newStatusPathFixture(t, proxy.UUID)

	got, err := f.proxyService(proxy).DeployLLMProxy("proxy-handle", &models.DeployAPIRequest{
		Base: "current", GatewayID: f.gateway.UUID.String(),
	}, "ou-1")

	require.NoError(t, err)
	assert.Equal(t, models.DeploymentStatusDeploying, *got.Status)
	event, payload := f.payload(t)
	assert.Equal(t, eventhub.EventType("llmproxy.deployed"), event.EventType)
	assert.Equal(t, "proxy-handle", payload["proxyId"])
	assertPerformedAt(t, f.row.PerformedAt, payload)
}

func TestUndeployLLMProxy(t *testing.T) {
	t.Run("sends an empty deployment id", func(t *testing.T) {
		proxy := &models.LLMProxy{UUID: uuid.New()}
		f := newStatusPathFixture(t, proxy.UUID)
		deployed := models.DeploymentStatusDeployed
		f.deployment.Status = &deployed
		f.row.Status = deployed

		_, err := f.proxyService(proxy).UndeployLLMProxyDeployment(context.Background(), "proxy-handle",
			f.deployment.DeploymentID.String(), f.gateway.UUID.String(), "ou-1")

		require.NoError(t, err)
		event, payload := f.payload(t)
		assert.Equal(t, eventhub.EventType("llmproxy.undeployed"), event.EventType)
		assert.Equal(t, "proxy-handle", payload["proxyId"])
		assert.Contains(t, payload, "deploymentId")
		assert.Empty(t, payload["deploymentId"])
		assertPerformedAt(t, f.row.PerformedAt, payload)
	})
	t.Run("a row never sent to the gateway sends nothing", func(t *testing.T) {
		proxy := &models.LLMProxy{UUID: uuid.New()}
		f := newStatusPathFixture(t, proxy.UUID)
		pending := models.DeploymentStatusPending
		f.deployment.Status = &pending
		f.row.Status = pending

		got, err := f.proxyService(proxy).UndeployLLMProxyDeployment(context.Background(), "proxy-handle",
			f.deployment.DeploymentID.String(), f.gateway.UUID.String(), "ou-1")

		require.NoError(t, err)
		assert.Equal(t, models.DeploymentStatusUndeployed, *got.Status)
		assert.Empty(t, f.hub.published)
	})
}

// A deployment the gateway is to serve, or may still hold, can't be deleted: the status row would cascade away with it
// and orphan the artifact on the gateway. Only undeployed and archived deployments can be.
func TestDeleteRefusesThePlacedOrHeldDeployment(t *testing.T) {
	type rowShape struct {
		status, desired models.DeploymentStatus
		owed            bool
		archived        bool
	}
	placed := map[string]rowShape{
		"deploying":       {status: models.DeploymentStatusDeploying, desired: models.DeploymentStatusDeployed},
		"deployed":        {status: models.DeploymentStatusDeployed, desired: models.DeploymentStatusDeployed},
		"failed deploy":   {status: models.DeploymentStatusFailed, desired: models.DeploymentStatusDeployed},
		"undeploying":     {status: models.DeploymentStatusUndeploying, desired: models.DeploymentStatusUndeployed},
		"failed undeploy": {status: models.DeploymentStatusFailed, desired: models.DeploymentStatusUndeployed},
		"undeployed, heading for DEPLOYED": {
			status: models.DeploymentStatusUndeployed, desired: models.DeploymentStatusDeployed,
		},
		"undeployed, a deploy owed": {
			status: models.DeploymentStatusUndeployed, desired: models.DeploymentStatusUndeployed, owed: true,
		},
	}
	notPlaced := map[string]rowShape{
		"undeployed": {status: models.DeploymentStatusUndeployed, desired: models.DeploymentStatusUndeployed},
		"archived":   {archived: true},
	}
	withRow := func(f *statusPathFixture, shape rowShape) {
		if shape.archived {
			archived := models.DeploymentStatusArchived
			f.deployment.Status = &archived
			return
		}
		row := &models.DeploymentStatusRecord{Status: shape.status, StatusDesired: shape.desired}
		if shape.owed {
			due := time.Now()
			row.NextAttemptAt = &due
		}
		f.deployment.SetStatusRow(row)
	}
	deleteBoth := func(t *testing.T, shape rowShape) (deletes int, providerErr, proxyErr error) {
		provider := &models.LLMProvider{UUID: uuid.New()}
		f := newStatusPathFixture(t, provider.UUID)
		withRow(f, shape)
		f.repo.DeleteFunc = func(string, string, string) error { return nil }
		providerErr = f.providerService(provider).DeleteLLMProviderDeployment("provider-handle",
			f.deployment.DeploymentID.String(), "ou-1")
		proxyErr = f.proxyService(&models.LLMProxy{UUID: provider.UUID}).DeleteLLMProxyDeployment("proxy-handle",
			f.deployment.DeploymentID.String(), "ou-1")
		return len(f.repo.DeleteCalls()), providerErr, proxyErr
	}

	for name, shape := range placed {
		t.Run(name, func(t *testing.T) {
			deletes, providerErr, proxyErr := deleteBoth(t, shape)
			assert.ErrorIs(t, providerErr, utils.ErrDeploymentIsDeployed)
			assert.ErrorIs(t, proxyErr, utils.ErrDeploymentIsDeployed)
			assert.Zero(t, deletes)
		})
	}
	for name, shape := range notPlaced {
		t.Run(name, func(t *testing.T) {
			deletes, providerErr, proxyErr := deleteBoth(t, shape)
			assert.NoError(t, providerErr)
			assert.NoError(t, proxyErr)
			assert.Equal(t, 2, deletes)
		})
	}
}

// Restoring the deployment the gateway is already deploying or serving is refused, as in api-platform. A restore
// over any other status goes ahead, and so does one that cancels a redeploy still owed.
func TestRestoreOfTheCurrentDeployment(t *testing.T) {
	due := time.Now()
	for name, tc := range map[string]struct {
		row     models.DeploymentStatusRecord
		refused bool
	}{
		"deploying":               {models.DeploymentStatusRecord{Status: models.DeploymentStatusDeploying}, true},
		"deployed":                {models.DeploymentStatusRecord{Status: models.DeploymentStatusDeployed}, true},
		"deployed, redeploy owed": {models.DeploymentStatusRecord{Status: models.DeploymentStatusDeployed, NextAttemptAt: &due}, false},
		"failed":                  {models.DeploymentStatusRecord{Status: models.DeploymentStatusFailed}, false},
		"undeploying":             {models.DeploymentStatusRecord{Status: models.DeploymentStatusUndeploying}, false},
		"undeployed":              {models.DeploymentStatusRecord{Status: models.DeploymentStatusUndeployed}, false},
	} {
		t.Run(name, func(t *testing.T) {
			provider := &models.LLMProvider{UUID: uuid.New()}
			f := newStatusPathFixture(t, provider.UUID)
			f.row = tc.row
			f.row.DeploymentID = &f.deployment.DeploymentID

			_, err := f.providerService(provider).RestoreLLMProviderDeployment("provider-handle",
				f.deployment.DeploymentID.String(), f.gateway.UUID.String(), "ou-1")

			if tc.refused {
				assert.ErrorIs(t, err, utils.ErrDeploymentAlreadyDeployed)
				assert.Empty(t, f.repo.StartRestoreCalls())
				return
			}
			require.NoError(t, err)
			assert.Len(t, f.repo.StartRestoreCalls(), 1)
		})
	}
}
