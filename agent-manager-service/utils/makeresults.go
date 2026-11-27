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
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

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

func stringToPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
