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

package gwsvc

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/reconcile"
)

// statusPathFixture is one artifact on one egress gateway, with a deployment repository that records the
// transition starters and answers GetStatusRecord with row.
type statusPathFixture struct {
	gateway    *gwmodels.Gateway
	deployment *gwmodels.Deployment
	row        gwmodels.DeploymentStatusRecord
	repo       *gwrepomocks.DeploymentRepositoryMock
	hub        *recordingEventHub
}

func newStatusPathFixture(t *testing.T, artifactUUID uuid.UUID) *statusPathFixture {
	t.Helper()
	f := &statusPathFixture{
		gateway: &gwmodels.Gateway{UUID: uuid.New(), OUID: "ou-1", GatewayFunctionalityType: gwmodels.GatewayRoleEgress},
		hub:     &recordingEventHub{},
	}
	f.deployment = &gwmodels.Deployment{
		DeploymentID: uuid.New(), ArtifactUUID: artifactUUID, OUID: "ou-1", GatewayUUID: f.gateway.UUID,
		Content: []byte("kind: test\n"),
	}
	f.row = gwmodels.DeploymentStatusRecord{PerformedAt: gwmodels.NewPerformedAt(time.Now())}
	f.repo = &gwrepomocks.DeploymentRepositoryMock{
		PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) { return nil, nil },
		GetWithContentFunc: func(string, string, string) (*gwmodels.Deployment, error) {
			return f.deployment, nil
		},
		GetWithStateFunc: func(string, string, string) (*gwmodels.Deployment, error) { return f.deployment, nil },
		GetWithStateCtxFunc: func(context.Context, string, string, string) (*gwmodels.Deployment, error) {
			return f.deployment, nil
		},
		StartDeployFunc: func(_ context.Context, tx *gorm.DB, _ gwmodels.Placement, d *gwmodels.Deployment) error {
			assert.Same(t, txSentinel, tx)
			deploying := gwmodels.DeploymentStatusDeploying
			d.Status, d.UpdatedAt = &deploying, &f.row.PerformedAt
			return nil
		},
		StartRestoreFunc: func(_ context.Context, tx *gorm.DB, _ gwmodels.Placement, _ uuid.UUID) error {
			assert.Same(t, txSentinel, tx)
			f.row.Status = gwmodels.DeploymentStatusDeploying
			return nil
		},
		BeginUndeployFunc: func(_ context.Context, tx *gorm.DB, _ gwmodels.Placement) (bool, error) {
			assert.Same(t, txSentinel, tx)
			sendEvent := f.row.Status != gwmodels.DeploymentStatusPending
			f.row.Status = gwmodels.DeploymentStatusUndeploying
			if !sendEvent {
				f.row.Status = gwmodels.DeploymentStatusUndeployed
			}
			return sendEvent, nil
		},
		GetStatusRecordFunc: func(context.Context, gwmodels.Placement) (*gwmodels.DeploymentStatusRecord, error) {
			row := f.row
			return &row, nil
		},
	}
	f.repo.WithTxFunc = func(tx *gorm.DB) gwrepo.DeploymentRepository {
		assert.Same(t, txSentinel, tx)
		return f.repo
	}
	return f
}

func (f *statusPathFixture) gatewayRepo() *gwrepomocks.GatewayRepositoryMock {
	return &gwrepomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(string) (*gwmodels.Gateway, error) { return f.gateway, nil },
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
func providerKindOf(provider *gwmodels.LLMProvider) *LLMProviderKind {
	return &LLMProviderKind{load: func(context.Context, gwmodels.ArtifactRef) (*gwmodels.LLMProvider, error) {
		return provider, nil
	}}
}

// renderableProvider is the least a provider needs to render.
func renderableProvider() *gwmodels.LLMProvider {
	return &gwmodels.LLMProvider{
		UUID:           uuid.New(),
		TemplateHandle: "openai",
		Artifact:       &gwmodels.Artifact{Handle: "provider-handle"},
		Configuration: gwmodels.LLMProviderConfig{
			Handle: "provider-handle", Name: "Provider", Version: "v1.0",
			Upstream: &gwmodels.UpstreamConfig{Main: &gwmodels.UpstreamEndpoint{URL: "https://api.openai.com/v1"}},
		},
	}
}

func (f *statusPathFixture) providerService(provider *gwmodels.LLMProvider) *LLMProviderDeploymentService {
	return NewLLMProviderDeploymentService(sentinelTx, f.repo, &gwrepomocks.LLMProviderRepositoryMock{
		GetByUUIDFunc:   func(string, string) (*gwmodels.LLMProvider, error) { return provider, nil },
		GetByHandleFunc: func(string, string) (*gwmodels.LLMProvider, error) { return provider, nil },
	}, nil, f.gatewayRepo(), NewGatewayEventsService(f.hub), f.core(providerKindOf(provider)))
}

// proxyKindOf renders proxy, fronting a renderable provider, whatever artifact it is asked for.
func proxyKindOf(proxy *gwmodels.LLMProxy) *LLMProxyKind {
	return &LLMProxyKind{
		load: func(context.Context, gwmodels.ArtifactRef) (*gwmodels.LLMProxy, *gwmodels.LLMProvider, error) {
			return proxy, renderableProvider(), nil
		},
		vhostOf: func(*gorm.DB, gwmodels.ArtifactRef) string { return proxyVhost(proxy) },
	}
}

func (f *statusPathFixture) proxyService(proxy *gwmodels.LLMProxy) *LLMProxyDeploymentService {
	kind := proxyKindOf(proxy)
	return NewLLMProxyDeploymentService(sentinelTx, f.repo, &gwrepomocks.LLMProxyRepositoryMock{
		GetByIDFunc:    func(string, string) (*gwmodels.LLMProxy, error) { return proxy, nil },
		GetByIDCtxFunc: func(context.Context, string, string) (*gwmodels.LLMProxy, error) { return proxy, nil },
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

	got, err := f.providerService(provider).DeployLLMProvider("provider-handle", &gwmodels.DeployAPIRequest{
		Name: "ignored", Metadata: map[string]any{"ignored": true}, GatewayID: f.gateway.UUID.String(),
	}, "ou-1")

	require.NoError(t, err)
	assert.Equal(t, gwmodels.DeploymentStatusDeploying, *got.Status)
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
		_, err := f.providerService(provider).DeployLLMProvider("provider-handle", &gwmodels.DeployAPIRequest{
			Base: base, GatewayID: f.gateway.UUID.String(),
		}, "ou-1")
		assert.NoError(t, err, "base %q", base)
	}
	for _, base := range []string{uuid.NewString(), "not-a-uuid"} {
		provider := renderableProvider()
		f := newStatusPathFixture(t, provider.UUID)
		_, err := f.providerService(provider).DeployLLMProvider("provider-handle", &gwmodels.DeployAPIRequest{
			Base: base, GatewayID: f.gateway.UUID.String(),
		}, "ou-1")
		assert.ErrorIs(t, err, gwerr.ErrDeploymentBaseNotCurrent, "base %q", base)
		assert.Empty(t, f.repo.StartDeployCalls())
	}
}

func TestRestoreOfAnUnknownDeploymentIsNotFound(t *testing.T) {
	provider := renderableProvider()
	f := newStatusPathFixture(t, provider.UUID)
	f.repo.GetWithContentFunc = func(string, string, string) (*gwmodels.Deployment, error) {
		return nil, gwerr.ErrDeploymentNotFound
	}
	svc := f.providerService(provider)

	_, err := svc.RestoreLLMProviderDeployment("provider-handle", uuid.NewString(), f.gateway.UUID.String(), "ou-1")
	assert.ErrorIs(t, err, gwerr.ErrDeploymentNotFound)
	_, err = svc.RestoreLLMProviderDeployment("provider-handle", "not-a-uuid", f.gateway.UUID.String(), "ou-1")
	assert.ErrorIs(t, err, gwerr.ErrDeploymentNotFound)
	assert.Empty(t, f.repo.StartRestoreCalls())
}

func TestRestoreLLMProviderSendsTheUUID(t *testing.T) {
	provider := &gwmodels.LLMProvider{UUID: uuid.New()}
	f := newStatusPathFixture(t, provider.UUID)
	f.row.Status = gwmodels.DeploymentStatusFailed

	got, err := f.providerService(provider).RestoreLLMProviderDeployment("provider-handle",
		f.deployment.DeploymentID.String(), f.gateway.UUID.String(), "ou-1")

	require.NoError(t, err)
	assert.Equal(t, gwmodels.DeploymentStatusDeploying, *got.Status)
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
		provider := &gwmodels.LLMProvider{UUID: uuid.New()}
		f := newStatusPathFixture(t, provider.UUID)

		_, err := f.providerService(provider).RestoreLLMProviderDeployment("provider-handle",
			strings.ToUpper(f.deployment.DeploymentID.String()), f.gateway.UUID.String(), "ou-1")

		require.NoError(t, err)
		_, payload := f.payload(t)
		assert.Equal(t, f.deployment.DeploymentID.String(), payload["deploymentId"])
	})
	t.Run("LLM proxy", func(t *testing.T) {
		proxy := &gwmodels.LLMProxy{UUID: uuid.New()}
		f := newStatusPathFixture(t, proxy.UUID)

		_, err := f.proxyService(proxy).RestoreLLMProxyDeployment("proxy-handle",
			strings.ToUpper(f.deployment.DeploymentID.String()), f.gateway.UUID.String(), "ou-1")

		require.NoError(t, err)
		_, payload := f.payload(t)
		assert.Equal(t, f.deployment.DeploymentID.String(), payload["deploymentId"])
	})
}

func TestDeployLLMProxyStartsDeployingAndSendsTheHandle(t *testing.T) {
	proxy := &gwmodels.LLMProxy{UUID: uuid.New()}
	f := newStatusPathFixture(t, proxy.UUID)

	got, err := f.proxyService(proxy).DeployLLMProxy("proxy-handle", &gwmodels.DeployAPIRequest{
		Base: "current", GatewayID: f.gateway.UUID.String(),
	}, "ou-1")

	require.NoError(t, err)
	assert.Equal(t, gwmodels.DeploymentStatusDeploying, *got.Status)
	event, payload := f.payload(t)
	assert.Equal(t, eventhub.EventType("llmproxy.deployed"), event.EventType)
	assert.Equal(t, "proxy-handle", payload["proxyId"])
	assertPerformedAt(t, f.row.PerformedAt, payload)
}

func TestUndeployLLMProxy(t *testing.T) {
	t.Run("sends an empty deployment id", func(t *testing.T) {
		proxy := &gwmodels.LLMProxy{UUID: uuid.New()}
		f := newStatusPathFixture(t, proxy.UUID)
		deployed := gwmodels.DeploymentStatusDeployed
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
		proxy := &gwmodels.LLMProxy{UUID: uuid.New()}
		f := newStatusPathFixture(t, proxy.UUID)
		pending := gwmodels.DeploymentStatusPending
		f.deployment.Status = &pending
		f.row.Status = pending

		got, err := f.proxyService(proxy).UndeployLLMProxyDeployment(context.Background(), "proxy-handle",
			f.deployment.DeploymentID.String(), f.gateway.UUID.String(), "ou-1")

		require.NoError(t, err)
		assert.Equal(t, gwmodels.DeploymentStatusUndeployed, *got.Status)
		assert.Empty(t, f.hub.published)
	})
}

// A deployment the gateway is to serve, or may still hold, can't be deleted: the status row would cascade away with it
// and orphan the artifact on the gateway. Only undeployed and archived deployments can be.
func TestDeleteRefusesThePlacedOrHeldDeployment(t *testing.T) {
	type rowShape struct {
		status, desired gwmodels.DeploymentStatus
		owed            bool
		archived        bool
	}
	placed := map[string]rowShape{
		"deploying":       {status: gwmodels.DeploymentStatusDeploying, desired: gwmodels.DeploymentStatusDeployed},
		"deployed":        {status: gwmodels.DeploymentStatusDeployed, desired: gwmodels.DeploymentStatusDeployed},
		"failed deploy":   {status: gwmodels.DeploymentStatusFailed, desired: gwmodels.DeploymentStatusDeployed},
		"undeploying":     {status: gwmodels.DeploymentStatusUndeploying, desired: gwmodels.DeploymentStatusUndeployed},
		"failed undeploy": {status: gwmodels.DeploymentStatusFailed, desired: gwmodels.DeploymentStatusUndeployed},
		"undeployed, heading for DEPLOYED": {
			status: gwmodels.DeploymentStatusUndeployed, desired: gwmodels.DeploymentStatusDeployed,
		},
		"undeployed, a deploy owed": {
			status: gwmodels.DeploymentStatusUndeployed, desired: gwmodels.DeploymentStatusUndeployed, owed: true,
		},
	}
	notPlaced := map[string]rowShape{
		"undeployed": {status: gwmodels.DeploymentStatusUndeployed, desired: gwmodels.DeploymentStatusUndeployed},
		"archived":   {archived: true},
	}
	withRow := func(f *statusPathFixture, shape rowShape) {
		if shape.archived {
			archived := gwmodels.DeploymentStatusArchived
			f.deployment.Status = &archived
			return
		}
		row := &gwmodels.DeploymentStatusRecord{Status: shape.status, StatusDesired: shape.desired}
		if shape.owed {
			due := time.Now()
			row.NextAttemptAt = &due
		}
		f.deployment.SetStatusRow(row)
	}
	deleteProxyDeployment := func(t *testing.T, shape rowShape) (deletes int, err error) {
		proxyID := uuid.New()
		f := newStatusPathFixture(t, proxyID)
		withRow(f, shape)
		f.repo.DeleteFunc = func(string, string, string) error { return nil }
		err = f.proxyService(&gwmodels.LLMProxy{UUID: proxyID}).DeleteLLMProxyDeployment("proxy-handle",
			f.deployment.DeploymentID.String(), "ou-1")
		return len(f.repo.DeleteCalls()), err
	}

	for name, shape := range placed {
		t.Run(name, func(t *testing.T) {
			deletes, err := deleteProxyDeployment(t, shape)
			assert.ErrorIs(t, err, gwerr.ErrDeploymentIsDeployed)
			assert.Zero(t, deletes)
		})
	}
	for name, shape := range notPlaced {
		t.Run(name, func(t *testing.T) {
			deletes, err := deleteProxyDeployment(t, shape)
			assert.NoError(t, err)
			assert.Equal(t, 1, deletes)
		})
	}
}

// Restoring the deployment the gateway is already deploying or serving is refused, as in api-platform. A restore
// over any other status goes ahead, and so does one that cancels a redeploy still owed.
func TestRestoreOfTheCurrentDeployment(t *testing.T) {
	due := time.Now()
	for name, tc := range map[string]struct {
		row     gwmodels.DeploymentStatusRecord
		refused bool
	}{
		"deploying":               {gwmodels.DeploymentStatusRecord{Status: gwmodels.DeploymentStatusDeploying}, true},
		"deployed":                {gwmodels.DeploymentStatusRecord{Status: gwmodels.DeploymentStatusDeployed}, true},
		"deployed, redeploy owed": {gwmodels.DeploymentStatusRecord{Status: gwmodels.DeploymentStatusDeployed, NextAttemptAt: &due}, false},
		"failed":                  {gwmodels.DeploymentStatusRecord{Status: gwmodels.DeploymentStatusFailed}, false},
		"undeploying":             {gwmodels.DeploymentStatusRecord{Status: gwmodels.DeploymentStatusUndeploying}, false},
		"undeployed":              {gwmodels.DeploymentStatusRecord{Status: gwmodels.DeploymentStatusUndeployed}, false},
	} {
		t.Run(name, func(t *testing.T) {
			provider := &gwmodels.LLMProvider{UUID: uuid.New()}
			f := newStatusPathFixture(t, provider.UUID)
			f.row = tc.row
			f.row.DeploymentID = &f.deployment.DeploymentID

			_, err := f.providerService(provider).RestoreLLMProviderDeployment("provider-handle",
				f.deployment.DeploymentID.String(), f.gateway.UUID.String(), "ou-1")

			if tc.refused {
				assert.ErrorIs(t, err, gwerr.ErrDeploymentAlreadyDeployed)
				assert.Empty(t, f.repo.StartRestoreCalls())
				return
			}
			require.NoError(t, err)
			assert.Len(t, f.repo.StartRestoreCalls(), 1)
		})
	}
}
