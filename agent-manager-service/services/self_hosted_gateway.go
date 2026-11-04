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
	"net/url"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	selfHostedHandleMinLength = 3
	selfHostedHandleMaxLength = 64
)

var (
	selfHostedHandlePattern   = regexp.MustCompile(`^[a-z0-9-]+$`)
	slugSeparatorPattern      = regexp.MustCompile(`[\s_]+`)
	slugDisallowedPattern     = regexp.MustCompile(`[^a-z0-9-]`)
	slugRepeatedHyphenPattern = regexp.MustCompile(`-{2,}`)
)

// SelfHostedGatewayInput is the registration request for a self-hosted gateway.
// Environment is the environment UUID; the caller resolves its name.
type SelfHostedGatewayInput struct {
	DisplayName       string
	Handle            string
	Description       string
	Environment       string
	FunctionalityType string
	Version           string
	Endpoints         []string
	IsCritical        bool
	Properties        map[string]interface{}
}

// RegisterSelfHostedGateway registers an egress gateway the customer runs themselves,
// bound to the single environment named in the input. Version is accepted and ignored.
func (s *PlatformGatewayService) RegisterSelfHostedGateway(ouID, envName string, in SelfHostedGatewayInput) (*GatewayResponse, error) {
	if err := requireSelfHostedFunctionalityType(in.FunctionalityType); err != nil {
		return nil, err
	}
	handle, err := selfHostedHandle(envName, in.Handle, in.DisplayName)
	if err != nil {
		return nil, err
	}
	vhost, err := selfHostedVhost(in.Endpoints)
	if err != nil {
		return nil, err
	}
	return s.RegisterGateway(ouID, handle, in.DisplayName, in.Description, vhost, "", in.IsCritical,
		models.GatewayRoleEgress, in.Properties, []string{in.Environment}, models.GatewayHostingSelfHosted)
}

// ListSelfHostedGateways lists the organization's self-hosted gateways.
func (s *PlatformGatewayService) ListSelfHostedGateways(ouID string, limit, offset int) (*GatewayListResponse, error) {
	return s.ListGateways(&ouID, &GatewayListFilters{Hosting: models.GatewayHostingSelfHosted}, limit, offset)
}

// RequireSelfHosted returns the gateway when it belongs to the organization and is
// self-hosted. The self-hosted routes address a gateway by UUID only, so a name is not found
// without a lookup. A missing, other-organization or managed gateway is reported as not found,
// so a self-hosted route never reveals or touches a managed gateway.
func (s *PlatformGatewayService) RequireSelfHosted(gatewayID, ouID string) (*models.Gateway, error) {
	if _, err := uuid.Parse(gatewayID); err != nil {
		return nil, utils.ErrGatewayNotFound
	}
	gateway, err := normalizeGatewayLookup(s.gatewayRepo.GetByUUID(gatewayID))
	if err != nil {
		return nil, err
	}
	if gateway.OUID != ouID || gateway.IsManaged() {
		return nil, utils.ErrGatewayNotFound
	}
	return gateway, nil
}

// SelfHostedFunctionalityType is the requested functionality type as registration reads it: case and surrounding
// space ignored, and empty meaning "ai", the one kind a self-hosted gateway has.
func SelfHostedFunctionalityType(requested string) string {
	kind := strings.ToLower(strings.TrimSpace(requested))
	if kind == "" {
		return "ai"
	}
	return kind
}

func requireSelfHostedFunctionalityType(functionalityType string) error {
	if SelfHostedFunctionalityType(functionalityType) == "ai" {
		return nil
	}
	return utils.NewInvalidInputError("Self-hosted gateways support only the AI functionality type",
		"unsupported self-hosted gateway functionality type: "+functionalityType)
}

// selfHostedHandle returns "<envName>-<handle>", deriving the handle from the display
// name when none is given.
func selfHostedHandle(envName, handle, displayName string) (string, error) {
	suffix := strings.TrimSpace(handle)
	if suffix == "" {
		suffix = gatewaySlug(displayName)
	}
	full := envName + "-" + suffix
	if len(full) < selfHostedHandleMinLength || len(full) > selfHostedHandleMaxLength ||
		!selfHostedHandlePattern.MatchString(full) || strings.HasSuffix(full, "-") {
		return "", utils.NewInvalidInputError(
			"Gateway handle must be 3 to 64 lowercase letters, numbers and hyphens",
			"derived gateway handle is invalid: "+full)
	}
	return full, nil
}

func gatewaySlug(text string) string {
	slug := strings.ToLower(strings.TrimSpace(text))
	slug = slugSeparatorPattern.ReplaceAllString(slug, "-")
	slug = slugDisallowedPattern.ReplaceAllString(slug, "")
	slug = slugRepeatedHyphenPattern.ReplaceAllString(slug, "-")
	return strings.Trim(slug, "-")
}

// selfHostedVhost returns the first endpoint, trimmed, when it is an http(s) URL with a host name
// and no user information.
func selfHostedVhost(endpoints []string) (string, error) {
	invalid := utils.NewInvalidInputError("The gateway needs an http or https endpoint URL", "invalid self-hosted gateway endpoint")
	if len(endpoints) == 0 {
		return "", invalid
	}
	endpoint := strings.TrimSpace(endpoints[0])
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
		return "", invalid
	}
	return endpoint, nil
}
