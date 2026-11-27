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
	"log/slog"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/utils/ssrf"
)

// validatePublicUpstreamURL is ssrf.ValidateURL, swappable in tests so they need no DNS.
var validatePublicUpstreamURL = ssrf.ValidateURL

// providerUpstreamURL is the provider's main upstream URL, or "" when it has none (a ref-only upstream).
func providerUpstreamURL(provider *models.LLMProvider) string {
	if provider == nil || provider.Configuration.Upstream == nil || provider.Configuration.Upstream.Main == nil {
		return ""
	}
	return strings.TrimSpace(provider.Configuration.Upstream.Main.URL)
}

// validateProviderUpstreamShape refuses a provider upstream URL that is not a plain http(s) URL. It does no network
// I/O: whether the host must be public depends on where the provider is deployed (requirePublicUpstreamOn).
func validateProviderUpstreamShape(provider *models.LLMProvider) error {
	upstreamURL := providerUpstreamURL(provider)
	if upstreamURL == "" {
		return nil
	}
	if _, err := ssrf.ValidateURLShape(upstreamURL); err != nil {
		return fmt.Errorf("%w: %w: upstream.main.url: %w", utils.ErrInvalidInput, utils.ErrInvalidURL, withoutURL(err))
	}
	return nil
}

// withoutURL drops the URL a *url.Error carries, keeping only its reason: the URL can hold userinfo (a credential),
// and these errors are logged and returned to the caller.
func withoutURL(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

// requirePublicUpstreamOn refuses upstreamURL when gateway is managed and the URL's host is not public.
//
// A managed gateway runs on the platform's network and calls the upstream as given, so an upstream on a loopback,
// private, link-local (cloud metadata) or other internal address would let a caller reach the platform's internal
// services through it — the same rule MCP proxies follow (gatewayNeedsPublicUpstream). A self-hosted gateway runs
// on the customer's own network, where a private upstream (an on-prem model server) is the normal case, so it is
// left alone. A ref-only upstream has no URL to check: the ref names an upstream the gateway itself defines.
//
// The resolver's detail can describe internal DNS, so it is logged and not returned.
func requirePublicUpstreamOn(ctx context.Context, gateway *models.Gateway, upstreamURL string) error {
	if gateway == nil || !gatewayNeedsPublicUpstream(gateway) || upstreamURL == "" {
		return nil
	}
	if err := validatePublicUpstreamURL(ctx, upstreamURL); err != nil {
		slog.Warn("LLM provider upstream refused for managed gateway",
			"gatewayID", gateway.UUID, "error", utils.SanitizeForLog(withoutURL(err).Error()))
		return fmt.Errorf("%w: %w: upstream.main.url must be a public http(s) address to deploy on managed gateway %q",
			utils.ErrInvalidInput, utils.ErrInvalidURL, gateway.Name)
	}
	return nil
}

// requirePublicUpstreamOnPlaced is requirePublicUpstreamOn for every gateway in gatewayIDs. A gateway that cannot be
// found is skipped: there is nothing on it for the URL to reach.
func requirePublicUpstreamOnPlaced(
	ctx context.Context, gatewayRepo repositories.GatewayRepository, ouID string, gatewayIDs []string, upstreamURL string,
) error {
	if upstreamURL == "" {
		return nil
	}
	for _, gatewayID := range gatewayIDs {
		gateway, err := gatewayRepo.GetByUUID(gatewayID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return fmt.Errorf("failed to get gateway %s: %w", gatewayID, err)
		}
		if gateway == nil || gateway.OUID != ouID {
			continue
		}
		if err := requirePublicUpstreamOn(ctx, gateway, upstreamURL); err != nil {
			return err
		}
	}
	return nil
}

// requirePublicRenderedUpstream is requirePublicUpstreamOn for the upstream in a rendered provider deployment: the
// configuration actually sent to the gateway, which a concurrent update may have changed since the provider was read.
func requirePublicRenderedUpstream(ctx context.Context, gateway *models.Gateway, content []byte) error {
	if gateway == nil || !gatewayNeedsPublicUpstream(gateway) {
		return nil
	}
	upstreamURL, err := renderedUpstreamURL(content)
	if err != nil {
		return err
	}
	return requirePublicUpstreamOn(ctx, gateway, upstreamURL)
}

// requirePublicUpstreamOnRequested is requirePublicUpstreamOn for the gateways a request names, before anything is
// saved. An id that is malformed, unknown or another organization's is skipped here: the deploy step reports it.
func requirePublicUpstreamOnRequested(
	ctx context.Context, gatewayRepo repositories.GatewayRepository, ouID string, gatewayIDs []string, upstreamURL string,
) error {
	valid := make([]string, 0, len(gatewayIDs))
	for _, id := range gatewayIDs {
		if _, err := uuid.Parse(id); err == nil {
			valid = append(valid, id)
		}
	}
	return requirePublicUpstreamOnPlaced(ctx, gatewayRepo, ouID, valid, upstreamURL)
}

// renderedUpstreamURL is the upstream URL in a rendered provider deployment, so a restore can be held to the same
// rule as a deploy: the snapshot may predate it.
func renderedUpstreamURL(content []byte) (string, error) {
	var rendered LLMProviderDeploymentYAML
	if err := yaml.Unmarshal(content, &rendered); err != nil {
		return "", fmt.Errorf("failed to read the deployment's upstream: %w", err)
	}
	return strings.TrimSpace(rendered.Spec.Upstream.URL), nil
}
