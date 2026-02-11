//
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
//

package client

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/apiplatformsvc/gen"
)

// convertToGenFunctionalityType converts client FunctionalityType to generated type
func convertToGenFunctionalityType(ft FunctionalityType) gen.CreateGatewayRequestFunctionalityType {
	switch ft {
	case FunctionalityTypeAI:
		return gen.CreateGatewayRequestFunctionalityTypeAi
	case FunctionalityTypeEvent:
		return gen.CreateGatewayRequestFunctionalityTypeEvent
	default:
		return gen.CreateGatewayRequestFunctionalityTypeRegular
	}
}

// convertFromGenGatewayResponse converts generated GatewayResponse to client type
func convertFromGenGatewayResponse(gw *gen.GatewayResponse) *GatewayResponse {
	if gw == nil {
		return nil
	}

	result := &GatewayResponse{}

	// Handle UUID
	if gw.Id != nil {
		result.ID = gw.Id.String()
	}

	// Handle string fields
	if gw.Name != nil {
		result.Name = *gw.Name
	}
	if gw.DisplayName != nil {
		result.DisplayName = *gw.DisplayName
	}
	if gw.Description != nil {
		result.Description = *gw.Description
	}
	if gw.Vhost != nil {
		result.Vhost = *gw.Vhost
	}

	// Handle functionality type
	if gw.FunctionalityType != nil {
		result.FunctionalityType = string(*gw.FunctionalityType)
	}

	// Handle boolean fields
	if gw.IsCritical != nil {
		result.IsCritical = *gw.IsCritical
	}
	if gw.IsActive != nil {
		result.IsActive = *gw.IsActive
	}

	// Handle timestamps
	if gw.CreatedAt != nil {
		result.CreatedAt = *gw.CreatedAt
	}
	if gw.UpdatedAt != nil {
		result.UpdatedAt = *gw.UpdatedAt
	}

	// Handle properties
	if gw.Properties != nil {
		result.Properties = *gw.Properties
	} else {
		result.Properties = make(map[string]interface{})
	}

	return result
}

// ptrString converts string to pointer
func ptrString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ptrBool converts bool to pointer
func ptrBool(b bool) *bool {
	return &b
}

// ptrTime converts time.Time to pointer
func ptrTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// derefString safely dereferences a string pointer
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// derefBool safely dereferences a bool pointer
func derefBool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

// derefTime safely dereferences a time.Time pointer
func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// parseUUID converts a string to openapi_types.UUID (gen.GatewayID)
func parseUUID(s string) (openapi_types.UUID, error) {
	parsed, err := uuid.Parse(s)
	if err != nil {
		return openapi_types.UUID{}, fmt.Errorf("invalid UUID format: %w", err)
	}
	return openapi_types.UUID(parsed), nil
}
