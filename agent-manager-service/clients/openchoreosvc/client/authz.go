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

package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
)

// EnsureClusterRoleBinding creates a ClusterAuthzRoleBinding for clientID → roleName.
// Idempotent: a 409 Conflict means the binding already exists and is treated as success.
func (c *openChoreoClient) EnsureClusterRoleBinding(ctx context.Context, clientID, roleName string) error {
	effect := gen.ClusterAuthzRoleBindingSpecEffectAllow
	roleRefKind := gen.ClusterAuthzRoleMappingRoleRefKindClusterAuthzRole
	bindingName := "amp-publisher-" + clientID + "-scheduler"

	body := gen.ClusterAuthzRoleBinding{
		Metadata: gen.ObjectMeta{Name: bindingName},
		Spec: &gen.ClusterAuthzRoleBindingSpec{
			Effect: &effect,
			Entitlement: gen.AuthzEntitlementClaim{
				Claim: "sub",
				Value: clientID,
			},
			RoleMappings: []gen.ClusterAuthzRoleMapping{
				{
					RoleRef: struct {
						Kind gen.ClusterAuthzRoleMappingRoleRefKind `json:"kind"`
						Name string                                 `json:"name"`
					}{
						Kind: roleRefKind,
						Name: roleName,
					},
				},
			},
		},
	}

	resp, err := c.ocClient.CreateClusterRoleBindingWithResponse(ctx, body)
	if err != nil {
		return fmt.Errorf("failed to create ClusterAuthzRoleBinding for %s: %w", clientID, err)
	}

	switch resp.StatusCode() {
	case http.StatusCreated, http.StatusOK:
		return nil
	case http.StatusConflict:
		// Binding already exists — idempotent success
		return nil
	default:
		return handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON400: resp.JSON400,
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON500: resp.JSON500,
		})
	}
}
