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
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// testdata/golden/a2a_agent_*.yaml were first produced by the a20829291 writer from this fixture (main's
// publication reconciler inputs, attemptPublish); they are hand-edited since: metadata.name is now agentGatewayName.

// goldenA2AConfig is an A2A agent's environment config with every part of the Agent render: API-key security,
// agent CORS and its own card CORS.
func goldenA2AConfig() *models.AgentConfig {
	cardCORS, cardCredentials := true, false
	return &models.AgentConfig{
		OUID: "ou-golden", ProjectName: "checkout", AgentName: "trip-planner", EnvironmentName: "dev",
		EnableApiKeySecurity: true,
		CORSEnabled:          true,
		CORSAllowOrigins:     []string{"https://client.example"},
		CORSAllowMethods:     []string{"GET", "POST", "OPTIONS"},
		CORSAllowHeaders:     []string{"Content-Type", "X-API-Key"},
		OAuthHeaderName:      "Authorization", OAuthAuthHeaderPrefix: "Bearer", OAuthForwardToken: true,
		CardCORSEnabled:          &cardCORS,
		CardCORSAllowOrigins:     []string{"*"},
		CardCORSAllowHeaders:     []string{"Content-Type"},
		CardCORSAllowCredentials: &cardCredentials,
	}
}

var goldenA2AEnvUUID = uuid.MustParse("11111111-2222-3333-4444-555555555555")

const (
	goldenA2ACardURL  = "https://agent.example/team/.well-known/agent-card.json"
	goldenA2ACardBody = `{"name":"Trip Planner","skills":[],"supportedInterfaces":[` +
		`{"protocolBinding":"JSONRPC","url":"https://agent.example/team/a2a/jsonrpc","protocolVersion":"1.0"},` +
		`{"protocolBinding":"HTTP+JSON","url":"https://agent.example/team/a2a/rest","protocolVersion":"1.0"}]}`
)

// goldenA2AProxy is the external agent's proxy, with an upstream credential.
func goldenA2AProxy() *models.A2AProxy {
	return &models.A2AProxy{
		OUID: "ou-golden", ProjectName: "checkout", AgentName: "trip-planner", EnvironmentName: "dev",
		EnvironmentUUID: goldenA2AEnvUUID, UpstreamAuthHeader: "X-Origin-Key",
		UpstreamAuthSecret: "VcsBaZXo9TyzcVjkEME432aIVqbceeyGcIdue/YLzutNtkkJoVTThPU=", // gitleaks:allow
	}
}

// goldenA2AKind renders the golden agent: as a platform agent through its exposure when proxy is nil, else as an
// external agent through proxy.
func goldenA2AKind(t *testing.T, proxy *models.A2AProxy, fetcher A2ACardFetcher) *A2AAgentKind {
	t.Helper()
	cfg := goldenA2AConfig()
	exposures := &repomocks.PlatformAgentExposureRepositoryMock{
		GetByArtifactFunc: func(_ context.Context, ouID string, artifactUUID uuid.UUID) (*models.PlatformAgentExposure, error) {
			if proxy != nil {
				return nil, repositories.ErrPlatformAgentExposureNotFound
			}
			return &models.PlatformAgentExposure{
				OUID: ouID, ProjectName: cfg.ProjectName, AgentName: cfg.AgentName, EnvironmentName: cfg.EnvironmentName,
				EnvironmentUUID: goldenA2AEnvUUID, ArtifactUUID: artifactUUID, Protocol: models.AgentExposureProtocolA2A,
			}, nil
		},
	}
	proxies := &repomocks.A2AProxyRepositoryMock{
		GetByArtifactFunc: func(context.Context, string, uuid.UUID) (*models.A2AProxy, error) { return proxy, nil },
		LockAgentEnvFunc:  func(context.Context, string, string, string, uuid.UUID) error { return nil },
		GetByEnvUUIDFunc: func(context.Context, string, string, string, uuid.UUID) (*models.A2AProxy, error) {
			return proxy, nil
		},
	}
	proxies.WithTxFunc = func(*gorm.DB) repositories.A2AProxyRepository { return proxies }
	cards := &repomocks.A2AAgentCardRepositoryMock{
		GetFunc: func(context.Context, string, string, string, string) (*models.A2AAgentCard, error) {
			return &models.A2AAgentCard{SourceURL: goldenA2ACardURL}, nil
		},
	}
	cards.WithTxFunc = func(*gorm.DB) repositories.A2AAgentCardRepository { return cards }
	configs := &repomocks.AgentConfigRepositoryMock{
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentConfig, error) { return cfg, nil },
	}
	oc := &repomocks.PlatformRepositoryMock{}
	return NewA2AAgentKind(sentinelTx, NewPlatformAgentExposures(exposures, nil, oc), proxies, cards, nil, configs, oc, fetcher, nil, nil,
		bytes.Repeat([]byte{7}, 32), testLogger())
}

func goldenA2AArtifact() models.ArtifactRef {
	return models.ArtifactRef{
		OUID: "ou-golden", ArtifactUUID: uuid.New(), Kind: models.KindAgent,
		Handle: agentEnvAPIArtifactHandle("checkout", "trip-planner", goldenA2AEnvUUID.String()),
	}
}

func TestA2AAgentKindRendersTodaysYAMLForAnExternalAgent(t *testing.T) {
	fetcher := &fakeA2ACardFetcher{
		FetchOriginFunc: func(context.Context, string, *A2ACardFetchHeader) (json.RawMessage, error) {
			return json.RawMessage(goldenA2ACardBody), nil
		},
	}

	rendered, err := goldenA2AKind(t, goldenA2AProxy(), fetcher).Render(context.Background(), goldenA2AArtifact())

	require.NoError(t, err)
	assert.Equal(t, readGolden(t, "a2a_agent_external.yaml"), string(rendered.Content))
	require.Len(t, fetcher.calls, 1)
	assert.Equal(t, goldenA2ACardURL, fetcher.calls[0].URL)
	assert.Equal(t, &A2ACardFetchHeader{Name: "X-Origin-Key", Value: "origin-secret"}, fetcher.calls[0].Header,
		"the origin card is fetched with the stored credential")
}

// A save's DeployRecord passes Targets a ctx carrying its transaction: the ingress gateway is read through it, not on
// the pool (the pool mock answers no reads).
func TestA2AAgentKindTargetsReadThroughTheSavesTransaction(t *testing.T) {
	env := uuid.New()
	ingress := &models.Gateway{UUID: uuid.New(), GatewayFunctionalityType: models.GatewayRoleIngress}
	txGateways := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(f repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			require.NotNil(t, f.EnvironmentID)
			assert.Equal(t, env.String(), *f.EnvironmentID)
			return []*models.Gateway{ingress}, nil
		},
	}
	pool := &repomocks.GatewayRepositoryMock{WithTxFunc: func(tx *gorm.DB) repositories.GatewayRepository {
		assert.Same(t, txSentinel, tx)
		return txGateways
	}}
	kind := NewA2AAgentKind(nil, NewPlatformAgentExposures(nil, pool, nil), nil, nil, nil, nil, nil, nil, nil, nil, nil, testLogger())

	gateway, ok, err := kind.Targets(db.CtxWithTx(context.Background(), txSentinel), reconcile.Record{
		ArtifactRef:     models.ArtifactRef{OUID: "ou-1", ArtifactUUID: uuid.New(), Kind: models.KindAgent},
		EnvironmentUUID: env,
	})

	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, ingress.UUID, gateway)
}
