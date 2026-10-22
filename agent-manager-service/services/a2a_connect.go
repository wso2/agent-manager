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
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// GetA2AConnectDetails reports how a client reaches the agent in one environment. A direct external agent
// is answered from its card alone: its config, gateway and publication are not the platform's to report.
func (s *a2aCatalogService) GetA2AConnectDetails(
	ctx context.Context, ouID, projectName, agentName, envName string,
) (*models.A2AConnectDetails, error) {
	card, err := s.cardService.GetA2AAgentCard(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		return nil, err
	}
	env, err := s.ocClient.GetEnvironment(ctx, ouID, envName)
	if err != nil {
		if translated := translateEnvironmentError(err); errors.Is(translated, utils.ErrEnvironmentNotFound) {
			return nil, translated
		}
		return nil, fmt.Errorf("failed to read environment %s: %w", envName, err)
	}
	details := &models.A2AConnectDetails{Kind: card.Source, IsProduction: env.IsProduction}

	source := models.A2APublicationSourcePlatform
	if card.Source == models.A2AAgentCardSourceExternal {
		proxied, err := s.isA2AProxied(ctx, ouID, projectName, agentName, card.EnvironmentUUID)
		if err != nil {
			return nil, err
		}
		if !proxied {
			details.CardURL, details.JSONRPCURL = a2aConnectURLs("", nil, card)
			details.Security = models.A2AConnectSecurity{Mode: models.A2AConnectSecurityUnmanaged}
			details.Publication = models.A2AConnectPublication{Status: models.A2AConnectPublicationNone}
			return details, nil
		}
		details.Proxied = true
		source = models.A2APublicationSourceExternal
	}

	status, err := s.publisher.GatewayStatus(ctx, ouID, projectName, agentName, card.EnvironmentUUID, source)
	if err != nil {
		return nil, err
	}
	details.GatewayURL = status.GatewayURL
	details.CardURL, details.JSONRPCURL = a2aConnectURLs(status.GatewayURL, status.Transports, card)
	details.Publication = models.A2AConnectPublication{Status: string(status.Status), LastError: status.LastError}
	details.Security, err = s.a2aConnectSecurity(ctx, ouID, projectName, agentName, envName, card.EnvironmentUUID)
	if err != nil {
		return nil, err
	}
	return details, nil
}

func (s *a2aCatalogService) isA2AProxied(ctx context.Context, ouID, projectName, agentName string, envUUID uuid.UUID) (bool, error) {
	_, err := s.proxyRepo.GetByEnvUUID(ctx, ouID, projectName, agentName, envUUID)
	switch {
	case errors.Is(err, repositories.ErrA2AProxyNotFound):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("failed to read the A2A proxy: %w", err)
	}
	return true, nil
}

// a2aConnectURLs gives the card and JSON-RPC URLs a client uses; a jsonRPCURL of "" means there is no JSON-RPC endpoint.
// Recorded transports are authoritative; without them a declared URL under the gateway wins over the /rpc default,
// because a gateway-fetched card already declares the gateway's own URL.
func a2aConnectURLs(gatewayURL string, transports []A2ATransport, card *models.A2AAgentCard) (cardURL, jsonRPCURL string) {
	declared := a2aDeclaredJSONRPCURLs(card.Card)
	if gatewayURL == "" {
		if len(declared) == 0 {
			return card.SourceURL, ""
		}
		return card.SourceURL, declared[0]
	}
	base := strings.TrimRight(gatewayURL, "/")
	cardURL = base + a2aAgentCardPath
	if transports != nil {
		for _, transport := range transports {
			if transport.ProtocolBinding == a2aTransportJSONRPC {
				return cardURL, base + transport.PathPrefix
			}
		}
		return cardURL, ""
	}
	for _, url := range declared {
		if url == base || strings.HasPrefix(url, base+"/") {
			return cardURL, url
		}
	}
	return cardURL, base + a2aPathPrefixJSONRPC
}

// a2aDeclaredJSONRPCURLs lists the card's non-empty JSON-RPC interface URLs in card order, skipping malformed
// entries; a card that does not decode declares nothing.
func a2aDeclaredJSONRPCURLs(card json.RawMessage) []string {
	var doc struct {
		SupportedInterfaces []map[string]json.RawMessage `json:"supportedInterfaces"`
	}
	if len(card) == 0 || json.Unmarshal(card, &doc) != nil {
		return nil
	}
	var urls []string
	for _, iface := range doc.SupportedInterfaces {
		binding, _ := jsonString(iface["protocolBinding"])
		url, ok := jsonString(iface["url"])
		if binding == a2aTransportJSONRPC && ok && url != "" {
			urls = append(urls, url)
		}
	}
	return urls
}

// a2aConnectSecurity reports the environment config's security; OAuth wins over an API key, and a missing config is
// reported as unknown rather than guessed.
func (s *a2aCatalogService) a2aConnectSecurity(
	ctx context.Context, ouID, projectName, agentName, envName string, envUUID uuid.UUID,
) (models.A2AConnectSecurity, error) {
	cfg, err := s.agentConfigRepo.Get(ctx, ouID, projectName, agentName, envName)
	switch {
	case errors.Is(err, repositories.ErrAgentConfigNotFound):
		return models.A2AConnectSecurity{Mode: models.A2AConnectSecurityUnknown}, nil
	case err != nil:
		return models.A2AConnectSecurity{}, fmt.Errorf("failed to read the agent config: %w", err)
	case cfg.EnableOAuthSecurity:
		oauth, err := s.a2aConnectOAuth(cfg, envUUID)
		if err != nil {
			return models.A2AConnectSecurity{}, err
		}
		return models.A2AConnectSecurity{Mode: models.A2AConnectSecurityOAuth, OAuth: oauth}, nil
	case cfg.EnableApiKeySecurity:
		return models.A2AConnectSecurity{Mode: models.A2AConnectSecurityAPIKey, APIKeyHeader: client.APIKeyHeaderName}, nil
	}
	return models.A2AConnectSecurity{Mode: models.A2AConnectSecurityNone}, nil
}

// a2aConnectOAuth resolves the config's issuer names against the environment's identity providers; an unknown
// name keeps its place with an empty issuer.
func (s *a2aCatalogService) a2aConnectOAuth(cfg *models.AgentConfig, envUUID uuid.UUID) (*models.A2AConnectOAuth, error) {
	providers, err := s.gatewayRepo.ListIdentityProvidersByEnvironment(envUUID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to list identity providers: %w", err)
	}
	issuerByName := make(map[string]string, len(providers))
	for _, provider := range providers {
		issuerByName[provider.Name] = provider.Issuer
	}
	view := oauthConfigFromAgentConfig(cfg)
	issuers := make([]models.A2AConnectIssuer, 0, len(view.Issuers))
	for _, name := range view.Issuers {
		issuers = append(issuers, models.A2AConnectIssuer{Name: name, Issuer: issuerByName[name]})
	}
	audiences := view.Audiences
	if audiences == nil {
		audiences = []string{}
	}
	return &models.A2AConnectOAuth{
		Issuers:          issuers,
		Audiences:        audiences,
		HeaderName:       view.HeaderName,
		AuthHeaderPrefix: view.AuthHeaderPrefix,
	}, nil
}
