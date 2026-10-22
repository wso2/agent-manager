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
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	connectOU      = "org-1"
	connectProject = "checkout"
	connectAgent   = "weather"
	connectEnv     = "dev"
)

var connectEnvUUID = uuid.MustParse("6f1c1f43-6d8f-4a43-9d64-3e8c35f0a1b2")

// connectCardServiceStub stands in for the card service; no generated mock exists for in-package service interfaces.
type connectCardServiceStub struct {
	A2AAgentCardService
	GetFunc func(ctx context.Context, ouID, projectName, agentName, envName string) (*models.A2AAgentCard, error)
}

func (s *connectCardServiceStub) GetA2AAgentCard(ctx context.Context, ouID, projectName, agentName, envName string) (*models.A2AAgentCard, error) {
	return s.GetFunc(ctx, ouID, projectName, agentName, envName)
}

type connectHarness struct {
	m       *publisherMocks
	cards   *connectCardServiceStub
	configs *repomocks.AgentConfigRepositoryMock
	oc      *clientmocks.OpenChoreoClientMock
	svc     A2ACatalogService
}

func platformConnectCard() *models.A2AAgentCard {
	return &models.A2AAgentCard{
		Source:          models.A2AAgentCardSourcePlatform,
		SourceURL:       "http://weather.internal:8080/.well-known/agent-card.json",
		EnvironmentUUID: connectEnvUUID,
		Card:            json.RawMessage(`{"supportedInterfaces":[{"protocolBinding":"JSONRPC","url":"http://weather.internal:8080/"}]}`),
	}
}

func externalConnectCard() *models.A2AAgentCard {
	return &models.A2AAgentCard{
		Source:          models.A2AAgentCardSourceExternal,
		SourceURL:       "https://agent.example/.well-known/agent-card.json",
		EnvironmentUUID: connectEnvUUID,
		Card:            json.RawMessage(`{"supportedInterfaces":[{"protocolBinding":"JSONRPC","url":"https://agent.example/rpc"}]}`),
	}
}

func newConnectHarness(t *testing.T) *connectHarness {
	t.Helper()
	m := newPublisherMocks(t)
	m.gateways.ListWithFiltersFunc = func(repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
		return []*models.Gateway{{Vhost: "gw.example.com", GatewayFunctionalityType: models.GatewayRoleIngress}}, nil
	}
	h := &connectHarness{
		m:       m,
		cards:   &connectCardServiceStub{},
		configs: &repomocks.AgentConfigRepositoryMock{},
		oc: &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(context.Context, string, string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: connectEnvUUID.String(), Name: connectEnv, IsProduction: false}, nil
			},
		},
	}
	h.cardIs(platformConnectCard())
	h.svc = NewA2ACatalogService(&repomocks.A2ACatalogRepositoryMock{}, h.cards, m.proxies, m.publisher(), h.configs, m.gateways, h.oc, discardLogger())
	return h
}

func (h *connectHarness) cardIs(card *models.A2AAgentCard) {
	h.cards.GetFunc = func(context.Context, string, string, string, string) (*models.A2AAgentCard, error) { return card, nil }
}

func (h *connectHarness) publicationIs(status models.A2APublicationStatus, lastError string) {
	h.m.pubs.GetFunc = func(context.Context, string, string, string, uuid.UUID) (*models.A2APublication, error) {
		return &models.A2APublication{Status: status, LastError: lastError}, nil
	}
}

func (h *connectHarness) configIs(cfg *models.AgentConfig, err error) {
	h.configs.GetFunc = func(context.Context, string, string, string, string) (*models.AgentConfig, error) { return cfg, err }
}

func (h *connectHarness) proxied() {
	h.m.proxies.GetByEnvUUIDFunc = func(context.Context, string, string, string, uuid.UUID) (*models.A2AProxy, error) {
		return &models.A2AProxy{}, nil
	}
}

func (h *connectHarness) connect() (*models.A2AConnectDetails, error) {
	return h.svc.GetA2AConnectDetails(context.Background(), connectOU, connectProject, connectAgent, connectEnv)
}

func TestConnectPlatformAgentWithAPIKey(t *testing.T) {
	h := newConnectHarness(t)
	h.publicationIs(models.A2APublicationStatusPublished, "")
	h.configIs(&models.AgentConfig{EnableApiKeySecurity: true}, nil)

	got, err := h.connect()

	require.NoError(t, err)
	assert.Equal(t, models.A2AAgentCardSourcePlatform, got.Kind)
	assert.False(t, got.Proxied)
	assert.Equal(t, "https://gw.example.com/weather", got.GatewayURL)
	assert.Equal(t, models.A2AConnectSecurity{Mode: models.A2AConnectSecurityAPIKey, APIKeyHeader: "X-API-Key"}, got.Security)
	assert.Equal(t, models.A2AConnectPublication{Status: "published"}, got.Publication)
}

func TestConnectReportsAFailedPublication(t *testing.T) {
	h := newConnectHarness(t)
	h.publicationIs(models.A2APublicationStatusFailed, "boom")
	h.configIs(&models.AgentConfig{EnableApiKeySecurity: true}, nil)

	got, err := h.connect()

	require.NoError(t, err)
	assert.Equal(t, models.A2AConnectPublication{Status: "failed", LastError: "boom"}, got.Publication)
}

func TestConnectResolvesOAuthIssuers(t *testing.T) {
	h := newConnectHarness(t)
	h.publicationIs(models.A2APublicationStatusPublished, "")
	h.configIs(&models.AgentConfig{EnableOAuthSecurity: true, EnableApiKeySecurity: true, OAuthIssuers: []string{"thunder", "gone"}}, nil)
	var listedFor string
	h.m.gateways.ListIdentityProvidersByEnvironmentFunc = func(environmentID string) ([]models.GatewayIdentityProvider, error) {
		listedFor = environmentID
		return []models.GatewayIdentityProvider{{Name: "thunder", Issuer: "https://idp/x"}}, nil
	}

	got, err := h.connect()

	require.NoError(t, err)
	assert.Equal(t, connectEnvUUID.String(), listedFor)
	assert.Equal(t, models.A2AConnectSecurity{
		Mode: models.A2AConnectSecurityOAuth,
		OAuth: &models.A2AConnectOAuth{
			Issuers:          []models.A2AConnectIssuer{{Name: "thunder", Issuer: "https://idp/x"}, {Name: "gone", Issuer: ""}},
			Audiences:        []string{},
			HeaderName:       "Authorization",
			AuthHeaderPrefix: "Bearer",
		},
	}, got.Security)
}

func TestConnectSecurityNoneAndUnknown(t *testing.T) {
	cases := map[string]struct {
		cfg  *models.AgentConfig
		err  error
		want models.A2AConnectSecurityMode
	}{
		"neither flag":  {cfg: &models.AgentConfig{}, want: models.A2AConnectSecurityNone},
		"no config row": {err: repositories.ErrAgentConfigNotFound, want: models.A2AConnectSecurityUnknown},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newConnectHarness(t)
			h.publicationIs(models.A2APublicationStatusPublished, "")
			h.configIs(tc.cfg, tc.err)

			got, err := h.connect()

			require.NoError(t, err)
			assert.Equal(t, models.A2AConnectSecurity{Mode: tc.want}, got.Security)
		})
	}
}

func TestConnectDirectExternalAgentReadsNothingElse(t *testing.T) {
	h := newConnectHarness(t)
	card := externalConnectCard()
	h.cardIs(card)
	h.m.proxies.GetByEnvUUIDFunc = func(context.Context, string, string, string, uuid.UUID) (*models.A2AProxy, error) {
		return nil, repositories.ErrA2AProxyNotFound
	}
	h.configs.GetFunc = nil
	h.m.pubs.GetFunc = nil
	h.m.gateways.ListWithFiltersFunc = nil

	got, err := h.connect()

	require.NoError(t, err)
	assert.Equal(t, &models.A2AConnectDetails{
		Kind:        models.A2AAgentCardSourceExternal,
		CardURL:     card.SourceURL,
		JSONRPCURL:  "https://agent.example/rpc",
		Security:    models.A2AConnectSecurity{Mode: models.A2AConnectSecurityUnmanaged},
		Publication: models.A2AConnectPublication{Status: models.A2AConnectPublicationNone},
	}, got)
}

func TestConnectProxiedExternalAgent(t *testing.T) {
	h := newConnectHarness(t)
	h.cardIs(externalConnectCard())
	h.proxied()
	h.publicationIs(models.A2APublicationStatusPending, "")
	h.configIs(&models.AgentConfig{EnableApiKeySecurity: true}, nil)

	got, err := h.connect()

	require.NoError(t, err)
	assert.Equal(t, models.A2AAgentCardSourceExternal, got.Kind)
	assert.True(t, got.Proxied)
	assert.Equal(t, "https://gw.example.com/weather", got.GatewayURL)
	assert.Equal(t, models.A2AConnectPublication{Status: "pending"}, got.Publication)
	assert.Equal(t, models.A2AConnectSecurityAPIKey, got.Security.Mode)
}

func TestConnectMissingPublicationMessages(t *testing.T) {
	cases := map[string]struct {
		card    *models.A2AAgentCard
		proxied bool
		want    string
	}{
		"platform": {
			card: platformConnectCard(),
			want: "no gateway publication is queued for this environment; redeploy the agent or save its deploy settings to publish it again",
		},
		"proxied external": {
			card:    externalConnectCard(),
			proxied: true,
			want:    "no gateway publication is queued for this environment; refresh to publish it again",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newConnectHarness(t)
			h.cardIs(tc.card)
			if tc.proxied {
				h.proxied()
			}
			h.m.pubs.GetFunc = func(context.Context, string, string, string, uuid.UUID) (*models.A2APublication, error) {
				return nil, repositories.ErrA2APublicationNotFound
			}
			h.configIs(&models.AgentConfig{}, nil)

			got, err := h.connect()

			require.NoError(t, err)
			assert.Equal(t, models.A2AConnectPublication{Status: "failed", LastError: tc.want}, got.Publication)
		})
	}
}

func TestConnectReportsProduction(t *testing.T) {
	h := newConnectHarness(t)
	h.publicationIs(models.A2APublicationStatusPublished, "")
	h.configIs(&models.AgentConfig{}, nil)
	h.oc.GetEnvironmentFunc = func(context.Context, string, string) (*models.EnvironmentResponse, error) {
		return &models.EnvironmentResponse{UUID: connectEnvUUID.String(), Name: connectEnv, IsProduction: true}, nil
	}

	got, err := h.connect()

	require.NoError(t, err)
	assert.True(t, got.IsProduction)
}

func TestConnectPassesCardAndEnvironmentSentinels(t *testing.T) {
	t.Run("card not found", func(t *testing.T) {
		h := newConnectHarness(t)
		h.cards.GetFunc = func(context.Context, string, string, string, string) (*models.A2AAgentCard, error) {
			return nil, utils.ErrAgentCardNotFound
		}

		got, err := h.connect()

		assert.Nil(t, got)
		assert.ErrorIs(t, err, utils.ErrAgentCardNotFound)
	})
	t.Run("environment not found", func(t *testing.T) {
		h := newConnectHarness(t)
		h.oc.GetEnvironmentFunc = func(context.Context, string, string) (*models.EnvironmentResponse, error) {
			return nil, utils.ErrNotFound
		}

		got, err := h.connect()

		assert.Nil(t, got)
		assert.ErrorIs(t, err, utils.ErrEnvironmentNotFound)
	})
}

func TestConnectDoesNotMaskDatabaseErrors(t *testing.T) {
	cases := map[string]func(h *connectHarness){
		"config read": func(h *connectHarness) {
			h.publicationIs(models.A2APublicationStatusPublished, "")
			h.configIs(nil, assert.AnError)
		},
		"proxy read": func(h *connectHarness) {
			h.cardIs(externalConnectCard())
			h.m.proxies.GetByEnvUUIDFunc = func(context.Context, string, string, string, uuid.UUID) (*models.A2AProxy, error) {
				return nil, assert.AnError
			}
		},
		"publication read": func(h *connectHarness) {
			h.m.pubs.GetFunc = func(context.Context, string, string, string, uuid.UUID) (*models.A2APublication, error) {
				return nil, assert.AnError
			}
		},
		"identity-provider listing": func(h *connectHarness) {
			h.publicationIs(models.A2APublicationStatusPublished, "")
			h.configIs(&models.AgentConfig{EnableOAuthSecurity: true, OAuthIssuers: []string{"thunder"}}, nil)
			h.m.gateways.ListIdentityProvidersByEnvironmentFunc = func(string) ([]models.GatewayIdentityProvider, error) {
				return nil, assert.AnError
			}
		},
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			h := newConnectHarness(t)
			arrange(h)

			got, err := h.connect()

			assert.Nil(t, got)
			require.ErrorIs(t, err, assert.AnError)
			assert.NotErrorIs(t, err, utils.ErrAgentCardNotFound)
		})
	}
}

func connectCardDeclaring(source models.A2AAgentCardSource, card string) *models.A2AAgentCard {
	c := &models.A2AAgentCard{Source: source, SourceURL: "https://agent.example/.well-known/agent-card.json"}
	if card != "" {
		c.Card = json.RawMessage(card)
	}
	return c
}

func TestA2AConnectURLs(t *testing.T) {
	const gatewayCardURL = "https://gw/agent/.well-known/agent-card.json"
	platformCard := connectCardDeclaring(models.A2AAgentCardSourcePlatform,
		`{"supportedInterfaces":[{"protocolBinding":"JSONRPC","url":"http://weather.internal:8080/"}]}`)
	proxiedCard := connectCardDeclaring(models.A2AAgentCardSourceExternal,
		`{"supportedInterfaces":[{"protocolBinding":"JSONRPC","url":"https://gw/agent/v1/jsonrpc"}]}`)
	directCard := connectCardDeclaring(models.A2AAgentCardSourceExternal,
		`{"supportedInterfaces":[{"protocolBinding":"JSONRPC","url":"https://agent.example/rpc"}]}`)
	unreadableCard := connectCardDeclaring(models.A2AAgentCardSourceExternal, `not json`)

	cases := map[string]struct {
		gatewayURL  string
		transports  []A2ATransport
		card        *models.A2AAgentCard
		wantCard    string
		wantJSONRPC string
	}{
		"recorded JSON-RPC at the root": {
			gatewayURL: "https://gw/agent", transports: []A2ATransport{{ProtocolBinding: "JSONRPC"}}, card: platformCard,
			wantCard: gatewayCardURL, wantJSONRPC: "https://gw/agent",
		},
		"recorded JSON-RPC under a prefix": {
			gatewayURL: "https://gw/agent", transports: []A2ATransport{{ProtocolBinding: "JSONRPC", PathPrefix: "/a2a/jsonrpc"}}, card: platformCard,
			wantCard: gatewayCardURL, wantJSONRPC: "https://gw/agent/a2a/jsonrpc",
		},
		"recorded HTTP+JSON only": {
			gatewayURL: "https://gw/agent", transports: []A2ATransport{{ProtocolBinding: "HTTP+JSON", PathPrefix: "/rest"}}, card: platformCard,
			wantCard: gatewayCardURL, wantJSONRPC: "",
		},
		"unrecorded with an internal declared URL": {
			gatewayURL: "https://gw/agent", card: platformCard,
			wantCard: gatewayCardURL, wantJSONRPC: "https://gw/agent/rpc",
		},
		"unrecorded with a declared URL under the gateway": {
			gatewayURL: "https://gw/agent", card: proxiedCard,
			wantCard: gatewayCardURL, wantJSONRPC: "https://gw/agent/v1/jsonrpc",
		},
		"trailing slash on the gateway URL": {
			gatewayURL: "https://gw/agent/", transports: []A2ATransport{{ProtocolBinding: "JSONRPC", PathPrefix: "/rpc"}}, card: platformCard,
			wantCard: gatewayCardURL, wantJSONRPC: "https://gw/agent/rpc",
		},
		"no gateway with a declared URL": {
			card: directCard, wantCard: directCard.SourceURL, wantJSONRPC: "https://agent.example/rpc",
		},
		"no gateway with an unreadable card": {
			card: unreadableCard, wantCard: unreadableCard.SourceURL, wantJSONRPC: "",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cardURL, jsonRPCURL := a2aConnectURLs(tc.gatewayURL, tc.transports, tc.card)

			assert.Equal(t, tc.wantCard, cardURL)
			assert.Equal(t, tc.wantJSONRPC, jsonRPCURL)
		})
	}
}

func TestConnectPlatformAgentWithoutAGateway(t *testing.T) {
	h := newConnectHarness(t)
	h.m.gateways.ListWithFiltersFunc = func(repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
		return []*models.Gateway{}, nil
	}
	h.publicationIs(models.A2APublicationStatusPending, "")
	h.configIs(&models.AgentConfig{EnableApiKeySecurity: true}, nil)

	got, err := h.connect()

	require.NoError(t, err)
	assert.Empty(t, got.GatewayURL)
	assert.Equal(t, platformConnectCard().SourceURL, got.CardURL)
	assert.Equal(t, "http://weather.internal:8080/", got.JSONRPCURL)
	assert.Equal(t, models.A2AConnectPublication{Status: "pending"}, got.Publication)
	assert.Equal(t, models.A2AConnectSecurityAPIKey, got.Security.Mode)
}

func TestA2ADeclaredJSONRPCURLsIsLenient(t *testing.T) {
	cases := map[string]struct {
		card json.RawMessage
		want []string
	}{
		"skips a non-string url": {
			card: json.RawMessage(`{"supportedInterfaces":[{"protocolBinding":"JSONRPC","url":5},{"protocolBinding":"JSONRPC","url":"https://x/rpc"}]}`),
			want: []string{"https://x/rpc"},
		},
		"interfaces not a list": {card: json.RawMessage(`{"supportedInterfaces":"x"}`)},
		"no card":               {},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, a2aDeclaredJSONRPCURLs(tc.card))
		})
	}
}
