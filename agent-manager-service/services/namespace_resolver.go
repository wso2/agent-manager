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

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
)

// ThunderOrgNamespace returns the org namespace/handle used to address an
// env-Thunder instance (see thundersvc.ThunderReleaseName) for AgentID
// provisioning and injection. Deliberately config-pinned
// (AMP_ORG_NAMESPACE, default "default"), NOT resolved dynamically:
// an env-Thunder's Helm release/namespace is fixed when the platform admin
// deploys it, so a dynamic lookup that followed an org rename would start
// addressing a Thunder that doesn't exist and break every provisioned agent.
// Both the provisioning and injection services call this so they can't disagree.
func ThunderOrgNamespace() string {
	return config.GetConfig().OrgNamespace
}

// ResolveEnvThunderClient is the single place every caller resolves a
// ThunderClient: credential lookup is scoped by ouID (multi-tenant-safe),
// while addressing stays pinned to ThunderOrgNamespace(). Call this instead of
// resolver.Resolve directly, so the pairing can't drift between call sites.
func ResolveEnvThunderClient(ctx context.Context, resolver thundersvc.EnvThunderResolver, ouID, envName string) (thundersvc.ThunderClient, error) {
	return resolver.Resolve(ctx, ouID, ThunderOrgNamespace(), envName)
}

// ResolveEnvThunderIdentity is ResolveEnvThunderClient widened to the identity surface.
func ResolveEnvThunderIdentity(ctx context.Context, resolver thundersvc.EnvThunderResolver, ouID, envName string) (thundersvc.EnvIdentityClient, error) {
	return resolver.ResolveIdentity(ctx, ouID, ThunderOrgNamespace(), envName)
}
