// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package utils

import (
	"slices"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

func ConvertToAgentListResponse(components []*models.AgentResponse) []spec.AgentResponse {
	if len(components) == 0 {
		return []spec.AgentResponse{}
	}
	responses := make([]spec.AgentResponse, len(components))
	for i, component := range components {
		responses[i] = ConvertToAgentResponse(component)
	}
	return responses
}

func ConvertToAgentSummaryListResponse(summaries []*models.AgentSummary) []spec.AgentSummary {
	responses := make([]spec.AgentSummary, len(summaries))
	for i, summary := range summaries {
		responses[i] = spec.AgentSummary{
			Name:               summary.Name,
			DisplayName:        summary.DisplayName,
			ProjectName:        summary.ProjectName,
			ProjectDisplayName: summary.ProjectDisplayName,
		}
	}
	return responses
}

func ConvertToAgentResponse(component *models.AgentResponse) spec.AgentResponse {
	if component == nil {
		return spec.AgentResponse{}
	}

	return convertToExternalAgentResponse(component)
}

// convertToCreatedBy maps the best-effort agent-creator info resolved in
// AgentManagerService.GetAgent onto the wire type. Nil when unresolved.
func convertToCreatedBy(createdBy *models.AgentCreatedBy) *spec.AgentCreatedBy {
	if createdBy == nil {
		return nil
	}
	result := &spec.AgentCreatedBy{Id: createdBy.ID}
	if createdBy.Display != "" {
		result.Display = &createdBy.Display
	}
	return result
}

// PopulateConfigurationResponseFromAgentConfig fills the per-environment tracing, CORS, and
// endpoint-authentication fields on a ConfigurationResponse from the agent_configs row. Unlike
// GetAgent (lowest-environment only), this lets the console seed the drawer per environment. No-op
// when cfg is nil ("no config persisted yet").
func PopulateConfigurationResponseFromAgentConfig(resp *spec.ConfigurationResponse, cfg *models.AgentConfig) {
	if resp == nil || cfg == nil {
		return
	}
	resp.EnableAutoInstrumentation = spec.PtrBool(cfg.EnableAutoInstrumentation)
	resp.InstrumentationVersion = cfg.InstrumentationVersion
	resp.EnableApiKeySecurity = spec.PtrBool(cfg.EnableApiKeySecurity)
	resp.EnableOAuthSecurity = spec.PtrBool(cfg.EnableOAuthSecurity)
	resp.ResilienceTimeoutSeconds = cfg.ResilienceTimeoutSeconds
	resp.CorsConfig = &spec.CORSConfig{
		Enabled:          spec.PtrBool(cfg.CORSEnabled),
		AllowOrigin:      cfg.CORSAllowOrigins,
		AllowMethods:     cfg.CORSAllowMethods,
		AllowHeaders:     cfg.CORSAllowHeaders,
		AllowCredentials: spec.PtrBool(cfg.CORSAllowCredentials),
	}
	resp.OauthConfig = &spec.OAuthConfig{
		Issuers:          cfg.OAuthIssuers,
		Audiences:        cfg.OAuthAudiences,
		HeaderName:       &cfg.OAuthHeaderName,
		AuthHeaderPrefix: &cfg.OAuthAuthHeaderPrefix,
		ForwardToken:     &cfg.OAuthForwardToken,
	}
	card := cfg.EffectiveCardCORS()
	resp.AgentCardCorsConfig = &spec.AgentCardCORSConfig{
		Inherit:          spec.PtrBool(card.Inherited),
		Enabled:          spec.PtrBool(card.Enabled),
		AllowOrigin:      card.AllowOrigins,
		AllowHeaders:     card.AllowHeaders,
		AllowCredentials: spec.PtrBool(card.AllowCredentials),
	}
}

func convertToExternalAgentResponse(component *models.AgentResponse) spec.AgentResponse {
	response := spec.AgentResponse{
		Uuid:        component.UUID,
		Name:        component.Name,
		DisplayName: component.DisplayName,
		Description: component.Description,
		ProjectName: component.ProjectName,
		CreatedAt:   component.CreatedAt,
		Status:      &component.Status,
		Provisioning: spec.Provisioning{
			Type: component.Provisioning.Type,
		},
		AgentType: spec.AgentType{
			Type: component.Type.Type,
		},
		CreatedBy: convertToCreatedBy(component.CreatedBy),
	}
	if len(component.Labels) > 0 {
		response.SetLabels(component.Labels)
	}
	return response
}

func ConvertToEnvironmentListResponse(environments []*models.EnvironmentResponse) []spec.Environment {
	if len(environments) == 0 {
		return []spec.Environment{}
	}

	responses := make([]spec.Environment, len(environments))
	for i, env := range environments {
		responses[i] = ConvertToEnvironmentResponse(env)
	}

	return responses
}

func ConvertToEnvironmentResponse(env *models.EnvironmentResponse) spec.Environment {
	if env == nil {
		return spec.Environment{}
	}

	return spec.Environment{
		Uuid:         env.UUID,
		Name:         env.Name,
		DataplaneRef: env.DataplaneRef,
		IsProduction: env.IsProduction,
		CreatedAt:    env.CreatedAt,
		DisplayName:  &env.DisplayName,
		DnsPrefix:    &env.DNSPrefix,
	}
}

func ConvertToOrganizationResponse(org *models.OrganizationResponse) spec.OrganizationResponse {
	if org == nil {
		return spec.OrganizationResponse{}
	}

	return spec.OrganizationResponse{
		Name:        org.Name,
		CreatedAt:   org.CreatedAt,
		DisplayName: org.DisplayName,
		Description: org.Description,
		Handle:      stringToPtr(org.Handle),
	}
}

func ConvertToOrganizationListItems(org *models.OrganizationResponse) spec.OrganizationListItem {
	if org == nil {
		return spec.OrganizationListItem{}
	}

	return spec.OrganizationListItem{
		Name:        org.Name,
		DisplayName: org.DisplayName,
		Handle:      stringToPtr(org.Handle),
		CreatedAt:   org.CreatedAt,
	}
}

func ConvertToOrganizationListResponse(orgs []*models.OrganizationResponse) []spec.OrganizationListItem {
	if len(orgs) == 0 {
		return []spec.OrganizationListItem{}
	}

	responses := make([]spec.OrganizationListItem, len(orgs))
	for i, org := range orgs {
		responses[i] = ConvertToOrganizationListItems(org)
	}

	return responses
}

func ConvertToProjectResponse(project *models.ProjectResponse) spec.ProjectResponse {
	if project == nil {
		return spec.ProjectResponse{}
	}
	return spec.ProjectResponse{
		Uuid:               project.UUID,
		Name:               project.Name,
		DisplayName:        project.DisplayName,
		Description:        project.Description,
		CreatedAt:          project.CreatedAt,
		DeploymentPipeline: "",
	}
}

func ConvertToProjectListItem(project *models.ProjectResponse) spec.ProjectListItem {
	if project == nil {
		return spec.ProjectListItem{}
	}

	return spec.ProjectListItem{
		Uuid:        project.UUID,
		Name:        project.Name,
		DisplayName: project.DisplayName,
		Description: &project.Description,
		CreatedAt:   project.CreatedAt,
	}
}

func ConvertToProjectListResponse(projects []*models.ProjectResponse) []spec.ProjectListItem {
	if len(projects) == 0 {
		return []spec.ProjectListItem{}
	}

	responses := make([]spec.ProjectListItem, len(projects))
	for i, project := range projects {
		responses[i] = ConvertToProjectListItem(project)
	}

	return responses
}

// RedactSecretConfigValues returns a copy of cfg with the value of every sensitive
// env var and file mount omitted, so a secret submitted on create is never
// serialized back out. Mirrors the read path in GetAgentConfigurations, which
// blanks the same fields.
//
// The copy is not incidental: cfg points into the decoded request body, and for
// kind-based agents ApplySecretConfigDefaults has already written the kind's stored
// secret defaults into the backing slices. Redacting in place would make
// correctness depend on the order in which the service and the handler run.
func RedactSecretConfigValues(cfg *spec.Configurations) *spec.Configurations {
	if cfg == nil {
		return nil
	}
	redacted := *cfg
	redacted.Env = redactSensitiveEnvValues(cfg.Env)
	redacted.Files = redactSensitiveFileValues(cfg.Files)
	return &redacted
}

func redactSensitiveEnvValues(env []spec.EnvironmentVariable) []spec.EnvironmentVariable {
	redacted := slices.Clone(env)
	for i := range redacted {
		if BoolPointerAsBool(redacted[i].IsSensitive, false) {
			redacted[i].Value = nil
		}
	}
	return redacted
}

func redactSensitiveFileValues(files []spec.FileMount) []spec.FileMount {
	redacted := slices.Clone(files)
	for i := range redacted {
		if BoolPointerAsBool(redacted[i].IsSensitive, false) {
			redacted[i].Value = nil
		}
	}
	return redacted
}

func stringToPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
