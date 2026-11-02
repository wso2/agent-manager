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
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
)

const (
	apiVersionMCPProxy              = "gateway.api-platform.wso2.com/v1"
	kindMCPProxy                    = "Mcp"
	mcpSetHeadersPolicyName         = "set-headers"
	mcpRemoveHeadersPolicyName      = "remove-headers"
	mcpLogMessagePolicyName         = "log-message"
	mcpBackendAuthPolicyName        = mcpSetHeadersPolicyName
	mcpBackendAuthPolicyVersion     = "v1"
	mcpBackendAuthPolicyDisplayName = "Backend Authentication Header"
	mcpAuthPolicyName               = "mcp-auth"
	mcpAuthPolicyVersion            = "v1"
	mcpAuthzPolicyName              = "mcp-authz"
	mcpAuthzPolicyVersion           = "v1"
	mcpIdentityIssuerKeyManager     = "ThunderKeyManager"
)

type mcpPolicyMergeStrategy string

const (
	mcpPolicyMergeStrategyMerge    mcpPolicyMergeStrategy = "merge"
	mcpPolicyMergeStrategyOverride mcpPolicyMergeStrategy = "override"
)

var mcpPolicyMergeStrategies = map[string]mcpPolicyMergeStrategy{
	mcpSetHeadersPolicyName:    mcpPolicyMergeStrategyMerge,
	mcpRemoveHeadersPolicyName: mcpPolicyMergeStrategyMerge,
	mcpLogMessagePolicyName:    mcpPolicyMergeStrategyMerge,
}

// MCPProxyDeploymentYAML represents the deployment YAML consumed by the gateway.
type MCPProxyDeploymentYAML struct {
	ApiVersion string                 `yaml:"apiVersion" json:"apiVersion"`
	Kind       string                 `yaml:"kind" json:"kind"`
	Metadata   DeploymentMetadata     `yaml:"metadata" json:"metadata"`
	Spec       MCPProxyDeploymentSpec `yaml:"spec" json:"spec"`
}

// MCPProxyDeploymentSpec represents the deployment spec section.
type MCPProxyDeploymentSpec struct {
	DisplayName string             `yaml:"displayName" json:"displayName"`
	Version     string             `yaml:"version" json:"version"`
	Context     string             `yaml:"context" json:"context"`
	Vhost       *string            `yaml:"vhost" json:"vhost"`
	Upstream    MCPProxyUpstream   `yaml:"upstream" json:"upstream"`
	Resilience  *models.Resilience `yaml:"resilience,omitempty" json:"resilience,omitempty"`
	SpecVersion string             `yaml:"specVersion" json:"specVersion"`
	Policies    []models.MCPPolicy `yaml:"policies,omitempty" json:"policies,omitempty"`
}

// MCPProxyUpstream represents the flat upstream shape expected by the gateway.
type MCPProxyUpstream struct {
	URL string `yaml:"url,omitempty" json:"url,omitempty"`
	Ref string `yaml:"ref,omitempty" json:"ref,omitempty"`
}

// deployMCPArtifact deploys one (endpoint, environment) artifact, saved in its binding in tx, on gateway, or, with
// no gateway, only validates it by rendering.
func (s *MCPProxyService) deployMCPArtifact(
	ctx context.Context, tx *gorm.DB, artifact *models.MCPProxy, ouID string, env uuid.UUID, gateway *uuid.UUID,
) error {
	record := reconcile.Record{
		ArtifactRef: models.ArtifactRef{
			OUID: ouID, ArtifactUUID: artifact.UUID, Kind: models.KindMCPMapping, Handle: artifact.Handle,
		},
		EnvironmentUUID:  env,
		RequestedGateway: gateway,
	}
	_, err := s.core.DeployRecord(ctx, tx, s.kind, record)
	return err
}

// mcpProxyEnvArtifactHandle builds the stable, org-unique handle/name for the single
// gateway artifact an MCP proxy endpoint deploys for one environment. The org-level proxy
// handle is unique per org; the endpoint handle (unique within the proxy) and the
// environment-UUID suffix distinguish the per-(endpoint,environment) artifacts, so the
// triple satisfies the artifacts table's UNIQUE(handle, org) constraint.
func mcpProxyEnvArtifactHandle(proxyHandle, endpointHandle, envID string) string {
	suffix := strings.ReplaceAll(strings.TrimSpace(envID), "-", "")
	return fmt.Sprintf("%s-%s-%s", proxyHandle, strings.TrimSpace(endpointHandle), suffix)
}

// buildMCPProxyEnvArtifact flattens one endpoint's config into the flat, single-environment
// MCPProxy that the deployment YAML builder consumes. Unlike the agent-scoped mapping
// (buildAgentMCPConfigProxy) this is the proxy's OWN artifact: the context/version/vhost
// inherit from the parent proxy's shared metadata — shared by every agent that references
// it — and the artifact identity is the stable (endpoint, environment) ArtifactUUID.
func buildMCPProxyEnvArtifact(source *models.MCPProxy, endpoint *models.MCPProxyEndpoint, ee *models.MCPProxyEndpointEnvironment) *models.MCPProxy {
	proxyHandle := source.Handle
	if source.Artifact != nil && source.Artifact.Handle != "" {
		proxyHandle = source.Artifact.Handle
	}
	handle := mcpProxyEnvArtifactHandle(proxyHandle, endpoint.Handle, ee.EnvironmentUUID.String())

	version := source.Version
	if source.Artifact != nil && source.Artifact.Version != "" {
		version = source.Artifact.Version
	}
	if version == "" {
		version = source.Configuration.Version
	}

	cfg := endpoint.Configuration
	var upstream models.UpstreamConfig
	if cfg.Upstream != nil {
		endpointCopy := *cfg.Upstream
		upstream.Main = &endpointCopy
	}

	return &models.MCPProxy{
		UUID:        ee.ArtifactUUID,
		Description: source.Description,
		Status:      source.Status,
		Configuration: models.MCPProxyConfig{
			Name:         handle,
			Version:      version,
			Context:      source.Configuration.Context,
			Vhost:        source.Configuration.Vhost,
			SpecVersion:  source.Configuration.SpecVersion,
			Upstream:     upstream,
			Policies:     cfg.Policies,
			Capabilities: cfg.Capabilities,
			Security:     cfg.Security,
		},
		OrganizationName: source.OrganizationName,
		ID:               handle,
		Name:             handle,
		Handle:           handle,
		Version:          version,
	}
}

// ensureMCPProxyEnvArtifactRow creates, in tx, the artifacts row backing a per-environment gateway artifact when it
// does not already exist. The deployments and deployment_status foreign keys require the row to be present before a
// deployment can be recorded. The row is keyed by its stable UUID (deployProxy.UUID == the binding's ArtifactUUID),
// not its handle: a preserved artifact whose environment remapped to a different endpoint carries a new handle but
// the same UUID.
func (s *MCPProxyService) ensureMCPProxyEnvArtifactRow(tx *gorm.DB, deployProxy *models.MCPProxy, ouID string) error {
	if s.artifactRepo == nil {
		return nil
	}
	var existing int64
	if err := tx.Model(&models.Artifact{}).Where("uuid = ? AND ou_id = ?", deployProxy.UUID, ouID).
		Count(&existing).Error; err != nil {
		return fmt.Errorf("failed to check MCP proxy environment artifact: %w", err)
	}
	if existing > 0 {
		return nil
	}
	now := time.Now()
	return s.artifactRepo.Create(tx, &models.Artifact{
		UUID:      deployProxy.UUID,
		Handle:    deployProxy.Handle,
		Name:      deployProxy.Name,
		Version:   deployProxy.Version,
		Kind:      models.KindMCPMapping,
		OUID:      ouID,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

// mcpEnvDeploy is one (endpoint, environment) artifact of a save and the gateway it deploys on; nil when the
// environment has none.
type mcpEnvDeploy struct {
	artifact *models.MCPProxy
	env      uuid.UUID
	gateway  *uuid.UUID
	label    string
}

// deployMCPProxyEndpoints deploys (or refreshes) the single gateway artifact of every (endpoint, environment)
// binding of the proxy saved in tx, in tx. Any error is returned for the caller to abort the save: a render or
// placement error is the client's (4xx). With no gateway in an environment the artifact is only validated; mapping a
// gateway places it. Gateways are deployed in gateway order, the order their event locks are taken in.
func (s *MCPProxyService) deployMCPProxyEndpoints(ctx context.Context, tx *gorm.DB, proxy *models.MCPProxy, ouID string) error {
	var deploys []mcpEnvDeploy
	for i := range proxy.Endpoints {
		endpoint := &proxy.Endpoints[i]
		for j := range endpoint.Environments {
			ee := &endpoint.Environments[j]
			label := fmt.Sprintf("endpoint %q environment %q", endpoint.Handle, ee.EnvironmentUUID)
			if ee.ArtifactUUID == uuid.Nil {
				return fmt.Errorf("%s: missing artifact id", label)
			}
			gateway, err := s.mcpEnvGateway(ctx, tx, proxy, ee, ouID)
			if err != nil {
				return fmt.Errorf("%s: %w", label, err)
			}
			deploys = append(deploys, mcpEnvDeploy{
				artifact: buildMCPProxyEnvArtifact(proxy, endpoint, ee), env: ee.EnvironmentUUID, gateway: gateway, label: label,
			})
		}
	}
	for _, d := range deploys {
		if err := s.ensureMCPProxyEnvArtifactRow(tx, d.artifact, ouID); err != nil {
			return fmt.Errorf("%s: %w", d.label, err)
		}
	}
	if err := s.lockMCPStatusRows(ctx, tx, deploys, ouID); err != nil {
		return err
	}
	sort.SliceStable(deploys, func(i, j int) bool { return gatewayOrderKey(deploys[i].gateway) < gatewayOrderKey(deploys[j].gateway) })
	for _, d := range deploys {
		if err := s.deployMCPArtifact(ctx, tx, d.artifact, ouID, d.env, d.gateway); err != nil {
			return fmt.Errorf("%s: deploy: %w", d.label, err)
		}
	}
	return nil
}

// lockMCPStatusRows takes the save's status-row locks before its first event takes a gateway's, the order a render of
// one binding takes: deploying binding a publishes on the gateway before binding b's row is written.
func (s *MCPProxyService) lockMCPStatusRows(ctx context.Context, tx *gorm.DB, deploys []mcpEnvDeploy, ouID string) error {
	if s.deploymentRepo == nil {
		return nil
	}
	artifactUUIDs := make([]uuid.UUID, 0, len(deploys))
	for _, d := range deploys {
		artifactUUIDs = append(artifactUUIDs, d.artifact.UUID)
	}
	return s.deploymentRepo.LockStatusRows(ctx, tx, ouID, artifactUUIDs)
}

// mcpEnvGateway is the gateway the binding deploys on: anchored on the gateway it is already placed on, whatever the
// placement's status, so a second egress gateway in the environment never re-homes it, or the request's explicit
// gateway, validated here. Nil when the environment has no gateway. It reads through the save's transaction tx.
func (s *MCPProxyService) mcpEnvGateway(
	ctx context.Context, tx *gorm.DB, proxy *models.MCPProxy, ee *models.MCPProxyEndpointEnvironment, ouID string,
) (*uuid.UUID, error) {
	var placed []string
	if s.deploymentRepo != nil {
		ids, err := s.deploymentRepo.WithTx(tx).PlacedGateways(ctx, ouID, ee.ArtifactUUID)
		if err != nil {
			return nil, fmt.Errorf("failed to list existing deployments: %w", err)
		}
		placed = ids
	}
	gateway, err := resolveEgressGatewayForArtifact(s.gatewayRepo.WithTx(tx), ouID, ee.EnvironmentUUID, placed, ee.RequestedGatewayUUID)
	if errors.Is(err, errNoGatewayForEnvironment) {
		s.logger.Info("No gateway mapped to the environment; validating the MCP proxy endpoint only",
			"proxyUUID", proxy.UUID, "environment", ee.EnvironmentUUID)
		return nil, nil //nolint:nilnil // No gateway: validate only.
	}
	if err != nil {
		return nil, fmt.Errorf("resolve gateway: %w", err)
	}
	return &gateway.UUID, nil
}

// gatewayOrderKey sorts no gateway first.
func gatewayOrderKey(gateway *uuid.UUID) string {
	if gateway == nil {
		return ""
	}
	return gateway.String()
}

// RedeployMCPProxy re-renders the proxy's artifacts wherever they are placed, because an input outside the proxy
// changed (its scopes). Inline: the gateways get the new policies before the request returns. Every artifact is
// rendered first, so a configuration that doesn't render is returned before any redeploy is queued.
func (s *MCPProxyService) RedeployMCPProxy(ctx context.Context, proxy *models.MCPProxy, ouID string) error {
	artifacts := mcpProxyArtifacts(proxy, ouID)
	for _, artifact := range artifacts {
		if _, err := s.kind.Render(ctx, artifact); err != nil && !errors.Is(err, reconcile.ErrRecordGone) {
			return err
		}
	}
	var errs []error
	for _, artifact := range artifacts {
		errs = append(errs, s.core.MarkStale(ctx, artifact.Kind, ouID, artifact.ArtifactUUID, nil,
			"the proxy's scopes changed"))
	}
	return errors.Join(errs...)
}

// mcpProxyArtifacts are the proxy's per-(endpoint, environment) gateway artifacts.
func mcpProxyArtifacts(proxy *models.MCPProxy, ouID string) []models.ArtifactRef {
	var artifacts []models.ArtifactRef
	for _, endpoint := range proxy.Endpoints {
		for _, ee := range endpoint.Environments {
			if ee.ArtifactUUID == uuid.Nil {
				continue
			}
			artifacts = append(artifacts, models.ArtifactRef{OUID: ouID, ArtifactUUID: ee.ArtifactUUID, Kind: models.KindMCPMapping})
		}
	}
	return artifacts
}

// deleteMCPProxyEnvironmentArtifacts deletes the given per-environment gateway artifacts and their deployments, each
// in one transaction with its delete events. Used when environments are removed from a proxy and when the proxy
// itself is deleted. Best-effort: per-artifact failures are aggregated.
func (s *MCPProxyService) deleteMCPProxyEnvironmentArtifacts(ctx context.Context, artifactUUIDs []uuid.UUID, ouID string) error {
	var errs []error
	for _, artifactUUID := range artifactUUIDs {
		if artifactUUID == uuid.Nil {
			continue
		}
		ref := models.ArtifactRef{OUID: ouID, ArtifactUUID: artifactUUID, Kind: models.KindMCPMapping}
		if err := s.tx(ctx, func(tx *gorm.DB) error {
			events, err := s.core.PrepareDelete(ctx, tx, ref)
			if err != nil {
				return err
			}
			if err := deleteMCPArtifactRows(tx, artifactUUID, ouID); err != nil {
				return err
			}
			return events.Publish(ctx, tx)
		}); err != nil {
			errs = append(errs, fmt.Errorf("artifact %s: %w", artifactUUID, err))
		}
	}
	return errors.Join(errs...)
}

// deleteMCPArtifactRows deletes a per-environment gateway artifact with its deployments and status rows, in tx.
func deleteMCPArtifactRows(tx *gorm.DB, artifactUUID uuid.UUID, ouID string) error {
	if err := tx.Where("artifact_uuid = ? AND ou_id = ?", artifactUUID, ouID).
		Delete(&models.DeploymentStatusRecord{}).Error; err != nil {
		return err
	}
	if err := tx.Where("artifact_uuid = ? AND ou_id = ?", artifactUUID, ouID).
		Delete(&models.Deployment{}).Error; err != nil {
		return err
	}
	return tx.Where("uuid = ? AND ou_id = ?", artifactUUID, ouID).Delete(&models.Artifact{}).Error
}

// prepareMCPArtifactDeletes builds the artifacts' delete events in tx, before their rows are deleted.
func (s *MCPProxyService) prepareMCPArtifactDeletes(
	ctx context.Context, tx *gorm.DB, ouID string, artifactUUIDs []uuid.UUID,
) ([]reconcile.DeleteEvents, error) {
	events := make([]reconcile.DeleteEvents, 0, len(artifactUUIDs))
	for _, artifactUUID := range artifactUUIDs {
		ref := models.ArtifactRef{OUID: ouID, ArtifactUUID: artifactUUID, Kind: models.KindMCPMapping}
		artifactEvents, err := s.core.PrepareDelete(ctx, tx, ref)
		if err != nil {
			return nil, err
		}
		events = append(events, artifactEvents)
	}
	return events, nil
}

func (s *MCPProxyService) generateMCPProxyDeploymentYAML(proxy *models.MCPProxy, proxyHandle string, scopes []models.MCPProxyScope) (string, error) {
	return renderMCPProxy(proxy, proxyHandle, scopes)
}

// renderMCPProxy is one (endpoint, environment) artifact's gateway configuration; proxyHandle is the source proxy's
// handle the scope policies are prefixed with.
func renderMCPProxy(proxy *models.MCPProxy, proxyHandle string, scopes []models.MCPProxyScope) (string, error) {
	deployment, err := buildMCPProxyDeploymentYAML(proxy, proxyHandle, scopes)
	if err != nil {
		return "", err
	}
	yamlBytes, err := yaml.Marshal(deployment)
	if err != nil {
		return "", fmt.Errorf("failed to marshal MCP proxy deployment YAML: %w", err)
	}
	return string(yamlBytes), nil
}

func buildMCPProxyDeploymentYAML(proxy *models.MCPProxy, proxyHandle string, scopes []models.MCPProxyScope) (*MCPProxyDeploymentYAML, error) {
	contextValue := "/"
	if proxy.Configuration.Context != nil && strings.TrimSpace(*proxy.Configuration.Context) != "" {
		contextValue = *proxy.Configuration.Context
	}

	specVersion := proxy.Configuration.SpecVersion
	if strings.TrimSpace(specVersion) == "" {
		specVersion = mcpProtocolVersion
	}

	upstream := MCPProxyUpstream{}
	var upstreamAuth *models.UpstreamAuth
	if proxy.Configuration.Upstream.Main != nil {
		upstream.URL = strings.TrimSpace(proxy.Configuration.Upstream.Main.URL)
		upstream.Ref = strings.TrimSpace(proxy.Configuration.Upstream.Main.Ref)
		upstreamAuth = proxy.Configuration.Upstream.Main.Auth
	}
	if upstream.URL == "" && upstream.Ref == "" {
		return nil, fmt.Errorf("upstream URL or ref is required")
	}
	policies, err := appendMCPAPIKeyAuthPolicy(proxy.Configuration.Policies, proxy.Configuration.Security)
	if err != nil {
		return nil, err
	}
	policies = appendMCPIdentityAuthPolicies(policies, proxy.Configuration.Security, proxyHandle, scopes)
	policies = appendMCPBackendAuthPolicy(policies, upstreamAuth)
	policies = mergeMCPPoliciesForDeployment(normalizeMCPPoliciesForDeployment(policies))
	handle := proxy.Handle
	displayName := proxy.Name
	version := proxy.Version
	if proxy.Artifact != nil {
		handle = proxy.Artifact.Handle
		displayName = proxy.Artifact.Name
		version = proxy.Artifact.Version
	}
	if displayName == "" {
		displayName = proxy.Configuration.Name
	}
	if version == "" {
		version = proxy.Configuration.Version
	}
	if handle == "" {
		handle = proxy.UUID.String()
	}

	return &MCPProxyDeploymentYAML{
		ApiVersion: apiVersionMCPProxy,
		Kind:       kindMCPProxy,
		Metadata:   DeploymentMetadata{Name: handle},
		Spec: MCPProxyDeploymentSpec{
			DisplayName: displayName,
			Version:     version,
			Context:     contextValue,
			Vhost:       proxy.Configuration.Vhost,
			Upstream:    upstream,
			Resilience:  proxy.Configuration.Resilience,
			SpecVersion: specVersion,
			Policies:    policies,
		},
	}, nil
}

func appendMCPBackendAuthPolicy(policies []models.MCPPolicy, auth *models.UpstreamAuth) []models.MCPPolicy {
	if auth == nil || auth.Header == nil || strings.TrimSpace(*auth.Header) == "" {
		return policies
	}

	header := strings.TrimSpace(*auth.Header)
	headerParam := map[string]interface{}{"name": header}
	switch {
	case auth.SecretRef != nil && *auth.SecretRef != "":
		headerParam["secretRef"] = *auth.SecretRef
	case auth.Value != nil && *auth.Value != "":
		headerParam["value"] = *auth.Value
	default:
		return policies
	}

	out := make([]models.MCPPolicy, 0, len(policies)+1)
	out = append(out, policies...)
	out = append(out, models.MCPPolicy{
		Name:        mcpBackendAuthPolicyName,
		Version:     mcpBackendAuthPolicyVersion,
		DisplayName: mcpBackendAuthPolicyDisplayName,
		Params: map[string]interface{}{
			"request": map[string]interface{}{
				"headers": []interface{}{headerParam},
			},
		},
	})
	return out
}

func appendMCPAPIKeyAuthPolicy(policies []models.MCPPolicy, security *models.SecurityConfig) ([]models.MCPPolicy, error) {
	if security == nil || !isBoolTrue(security.Enabled) {
		return policies, nil
	}
	if security.APIKey == nil || !isBoolTrue(security.APIKey.Enabled) {
		return policies, nil
	}

	key := strings.TrimSpace(security.APIKey.Key)
	if key == "" {
		return nil, fmt.Errorf("invalid api key security configuration: key is required")
	}

	in := strings.ToLower(strings.TrimSpace(security.APIKey.In))
	if in != "header" && in != "query" {
		return nil, fmt.Errorf("invalid api key security configuration: in must be 'header' or 'query', got %q", security.APIKey.In)
	}

	out := make([]models.MCPPolicy, 0, len(policies)+1)
	out = append(out, policies...)
	out = append(out, models.MCPPolicy{
		Name:    apiKeyAuthPolicyName,
		Version: apiKeyAuthPolicyVersion,
		Params: map[string]interface{}{
			"key": key,
			"in":  in,
		},
	})
	return out, nil
}

// appendMCPIdentityAuthPolicies emits the Agent Identity gateway policies for a
// flattened per-environment artifact: mcp-auth (JWT validation against the
// ThunderKeyManager key manager) and mcp-authz (per-tool requiredScopes
// enforcement). mcp-auth's requiredScopes is advertisement only — it becomes
// scopes_supported in the protected resource metadata and the scope hint in the
// WWW-Authenticate challenge, so clients learn what to request; mcp-auth strips
// it before delegating to jwt-auth (wso2/gateway-controllers commit 7045133,
// mcp-auth v1.1.1). Tools with no covering scope get no mcp-authz rule — gateway
// default-permit means authenticated-only.
func appendMCPIdentityAuthPolicies(policies []models.MCPPolicy, security *models.SecurityConfig, proxyHandle string, scopes []models.MCPProxyScope) []models.MCPPolicy {
	if !mcpIdentityEnabled(security) {
		return policies
	}

	// Invert scope->tools into one mcp-authz rule per tool. ONE rule per tool is
	// load-bearing: the gateway ANDs multiple matching rules together but ORs the
	// scopes inside a rule (checkScopes in wso2/gateway-controllers
	// policies/mcp-authz/mcp-authz.go:515 passes on the FIRST matching token
	// scope). A tool covered by several scopes must therefore carry them all in
	// a single rule's requiredScopes. NOTE: that policy's definition YAML
	// *describes* all-of semantics; the shipped code is any-of. Re-verify on any
	// gateway policy version bump.
	toolScopes := map[string][]string{}
	scopeSet := map[string]struct{}{}
	for _, sc := range scopes {
		str := sc.ScopeString(proxyHandle)
		scopeSet[str] = struct{}{}
		for _, tool := range sc.Tools {
			toolScopes[tool] = append(toolScopes[tool], str)
		}
	}
	toolNames := make([]string, 0, len(toolScopes))
	for tool := range toolScopes {
		toolNames = append(toolNames, tool)
	}
	sort.Strings(toolNames)
	toolRules := make([]map[string]interface{}, 0, len(toolNames))
	for _, tool := range toolNames {
		reqs := toolScopes[tool]
		sort.Strings(reqs)
		toolRules = append(toolRules, map[string]interface{}{"name": tool, "requiredScopes": reqs})
	}

	authParams := map[string]interface{}{
		"issuers": []interface{}{mcpIdentityIssuerKeyManager},
	}
	// Every scope of the proxy, including ones covering no tool: the union is what
	// a client should ask for, not what any single call enforces.
	if len(scopeSet) > 0 {
		union := make([]string, 0, len(scopeSet))
		for sc := range scopeSet {
			union = append(union, sc)
		}
		sort.Strings(union)
		authParams["requiredScopes"] = union
	}

	out := make([]models.MCPPolicy, 0, len(policies)+2)
	out = append(out, policies...)
	out = append(out, models.MCPPolicy{Name: mcpAuthPolicyName, Version: mcpAuthPolicyVersion, Params: authParams})
	if len(toolRules) > 0 {
		out = append(out, models.MCPPolicy{Name: mcpAuthzPolicyName, Version: mcpAuthzPolicyVersion, Params: map[string]interface{}{"tools": toolRules}})
	}
	return out
}

// mcpIdentityEnabled reports whether identity security is enabled for the given
// endpoint security config. Shared by gateway emission and Thunder scope
// cleanup so the identity gate can't drift between the two.
func mcpIdentityEnabled(security *models.SecurityConfig) bool {
	return security != nil && isBoolTrue(security.Enabled) &&
		security.Identity != nil && isBoolTrue(security.Identity.Enabled)
}

func normalizeMCPPoliciesForDeployment(policies []models.MCPPolicy) []models.MCPPolicy {
	if len(policies) == 0 {
		return nil
	}

	out := make([]models.MCPPolicy, 0, len(policies))
	for _, policy := range policies {
		out = append(out, models.MCPPolicy{
			Name:               policy.Name,
			Version:            normalizePolicyVersionToMajor(policy.Version),
			DisplayName:        policy.DisplayName,
			ExecutionCondition: policy.ExecutionCondition,
			Params:             policy.Params,
		})
	}
	return out
}

func mergeMCPPoliciesForDeployment(policies []models.MCPPolicy) []models.MCPPolicy {
	if len(policies) < 2 {
		return policies
	}

	out := make([]models.MCPPolicy, 0, len(policies))
	policyIndexes := map[string]int{}
	for _, policy := range policies {
		key := mcpPolicyIdentityKey(policy)
		existingIndex, ok := policyIndexes[key]
		if !ok {
			policyIndexes[key] = len(out)
			out = append(out, policy)
			continue
		}

		switch mcpPolicyMergeStrategyFor(policy.Name) {
		case mcpPolicyMergeStrategyMerge:
			out[existingIndex] = mergeMCPPolicyForDeployment(out[existingIndex], policy)
		default:
			out[existingIndex] = policy
		}
	}
	return out
}

func mcpPolicyIdentityKey(policy models.MCPPolicy) string {
	return strings.TrimSpace(policy.Name) + "\x00" + strings.TrimSpace(policy.Version)
}

func mcpPolicyMergeStrategyFor(policyName string) mcpPolicyMergeStrategy {
	if strategy, ok := mcpPolicyMergeStrategies[strings.TrimSpace(policyName)]; ok {
		return strategy
	}
	return mcpPolicyMergeStrategyOverride
}

func mergeMCPPolicyForDeployment(base, next models.MCPPolicy) models.MCPPolicy {
	merged := next
	merged.Params = mergeMCPPolicyParams(base.Params, next.Params)
	return merged
}

func mergeMCPPolicyParams(base, next map[string]interface{}) map[string]interface{} {
	if len(base) == 0 {
		return next
	}
	if len(next) == 0 {
		return base
	}

	out := cloneStringInterfaceMap(base)
	for key, nextValue := range next {
		baseValue, exists := out[key]
		if !exists {
			out[key] = nextValue
			continue
		}
		out[key] = mergeMCPPolicyParamValue(baseValue, nextValue)
	}
	return out
}

func mergeMCPPolicyParamValue(base, next interface{}) interface{} {
	baseMap, baseMapOK := base.(map[string]interface{})
	nextMap, nextMapOK := next.(map[string]interface{})
	if baseMapOK && nextMapOK {
		return mergeMCPPolicyParams(baseMap, nextMap)
	}

	baseBool, baseBoolOK := base.(bool)
	nextBool, nextBoolOK := next.(bool)
	if baseBoolOK && nextBoolOK {
		return baseBool || nextBool
	}

	if merged, ok := mergeStringSlices(base, next); ok {
		return merged
	}

	if merged, ok := mergeInterfaceSlices(base, next); ok {
		return merged
	}

	return next
}

func mergeStringSlices(base, next interface{}) (interface{}, bool) {
	baseStrings, baseOK := base.([]string)
	nextStrings, nextOK := next.([]string)
	if !baseOK || !nextOK {
		return nil, false
	}

	out := make([]string, 0, len(baseStrings)+len(nextStrings))
	seen := map[string]struct{}{}
	for _, value := range append(baseStrings, nextStrings...) {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out, true
}

func mergeInterfaceSlices(base, next interface{}) (interface{}, bool) {
	baseItems, baseOK := base.([]interface{})
	nextItems, nextOK := next.([]interface{})
	if !baseOK || !nextOK {
		return nil, false
	}

	out := make([]interface{}, 0, len(baseItems)+len(nextItems))
	out = append(out, baseItems...)
	for _, item := range nextItems {
		if stringValue, ok := item.(string); ok && containsStringInterface(out, stringValue) {
			continue
		}
		out = append(out, item)
	}
	return out, true
}

func containsStringInterface(items []interface{}, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func cloneStringInterfaceMap(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func deploymentName(proxy *models.MCPProxy) string {
	if strings.TrimSpace(proxy.Handle) != "" {
		return fmt.Sprintf("%s-deployment", proxy.Handle)
	}
	if proxy.Artifact != nil && strings.TrimSpace(proxy.Artifact.Handle) != "" {
		return fmt.Sprintf("%s-deployment", proxy.Artifact.Handle)
	}
	return fmt.Sprintf("%s-deployment", proxy.UUID.String())
}
