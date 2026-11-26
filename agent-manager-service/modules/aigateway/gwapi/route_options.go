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

package gwapi

import (
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwaudit"
)

// routeOptions are what the gateway's public routes record and emit besides
// their permission, keyed by their pattern relative to /m/ai-gateway: the
// audit action of every route the trail records, and the webhook event of the
// routes that raise one. A route not listed records under its permission (a
// change) or not at all (a read). Routes that mint, rotate or revoke
// credentials, or change what a gateway trusts, are fail-closed: their effect
// lands outside the database, so they refuse to run unrecorded. The routes a
// deployment's control plane drives on a tenant's behalf allow an org
// override.
var routeOptions = map[string][]modkit.RouteOption{
	"GET /orgs/{orgName}/gateways": {modkit.AllowOrgOverride()},

	// Document routes keep v1's audit actions and webhook events, so reports don't split across the migration.
	"POST /llm-providers":                {modkit.Audit("llm-providers.create"), modkit.Emits(gwaudit.LLMProviderCreated)},
	"PUT /llm-providers/{providerId}":    {modkit.Audit("llm-providers.update"), modkit.Emits(gwaudit.LLMProviderUpdated)},
	"DELETE /llm-providers/{providerId}": {modkit.Audit("llm-providers.delete"), modkit.Emits(gwaudit.LLMProviderDeleted)},

	"POST /mcp-proxies":                                  {modkit.Audit("mcp-servers.create"), modkit.Emits(gwaudit.MCPServerCreated)},
	"PUT /mcp-proxies/{proxyId}":                         {modkit.Audit("mcp-servers.update"), modkit.Emits(gwaudit.MCPServerUpdated)},
	"DELETE /mcp-proxies/{proxyId}":                      {modkit.Audit("mcp-servers.delete"), modkit.Emits(gwaudit.MCPServerDeleted)},
	"POST /mcp-proxies/fetch-server-info":                {modkit.Audit("mcp-servers.fetch-server-info")},
	"POST /mcp-proxies/{proxyId}/scopes":                 {modkit.Audit("scopes.create")},
	"PUT /mcp-proxies/{proxyId}/scopes/{scopeAction}":    {modkit.Audit("scopes.update")},
	"DELETE /mcp-proxies/{proxyId}/scopes/{scopeAction}": {modkit.Audit("scopes.delete")},

	"DELETE /orgs/{orgName}/environments/{envName}/agent-identities/groups/{groupID}":                {modkit.Audit("agent-identities.delete")},
	"DELETE /orgs/{orgName}/environments/{envName}/agent-identities/roles/{roleID}":                  {modkit.Audit("agent-identities.delete")},
	"DELETE /orgs/{orgName}/gateways/{gatewayID}":                                                    {modkit.Audit("gateways.delete"), modkit.Emits(gwaudit.GatewayDeleted), modkit.AuditFailClosed()},
	"DELETE /orgs/{orgName}/gateways/{gatewayID}/environments/{envID}":                               {modkit.Audit("gateways.unassign-environment")},
	"DELETE /orgs/{orgName}/gateways/{gatewayID}/identity-providers/{name}":                          {modkit.Audit("gateways.remove-identity-provider"), modkit.AuditFailClosed()},
	"DELETE /orgs/{orgName}/gateways/{gatewayID}/tokens/{tokenID}":                                   {modkit.Audit("gateway-tokens.revoke"), modkit.AuditFailClosed()},
	"DELETE /orgs/{orgName}/llm-provider-templates/{templateId}":                                     {modkit.Audit("llm-provider-templates.delete")},
	"DELETE /orgs/{orgName}/llm-providers/{id}/api-keys/{keyName}":                                   {modkit.Audit("api-keys.revoke"), modkit.AuditFailClosed()},
	"DELETE /orgs/{orgName}/llm-providers/{providerId}/deployments/{deploymentId}":                   {modkit.Audit("llm-providers.delete-deployment")},
	"DELETE /agents/{agentName}/a2a-proxy":                                                           {modkit.Audit("agents.remove-a2a-proxy"), modkit.AuditFailClosed()},
	"DELETE /agents/{agentName}/agent-card/source":                                                   {modkit.Audit("agents.remove-card-source"), modkit.AuditFailClosed()},
	"DELETE /agents/{agentName}/api-keys/{keyName}":                                                  {modkit.Audit("api-keys.revoke"), modkit.Emits(gwaudit.AgentAPIKeyRevoked), modkit.AuditFailClosed()},
	"DELETE /agents/{agentName}/mcp-configs/{configId}":                                              {modkit.Audit("agent-configs.delete")},
	"DELETE /agents/{agentName}/mcp-configs/{configId}/api-keys/{keyName}":                           {modkit.Audit("api-keys.revoke"), modkit.AuditFailClosed()},
	"DELETE /agents/{agentName}/model-configs/{configId}":                                            {modkit.Audit("agent-configs.delete")},
	"DELETE /agents/{agentName}/model-configs/{configId}/api-keys/{keyName}":                         {modkit.Audit("api-keys.revoke"), modkit.AuditFailClosed()},
	"DELETE /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/api-keys/{keyName}":                 {modkit.Audit("api-keys.revoke"), modkit.AuditFailClosed()},
	"DELETE /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/deployments/{deploymentId}":         {modkit.Audit("llm-proxies.delete-deployment")},
	"DELETE /orgs/{orgName}/projects/{projName}/llm-proxies/{proxyId}":                               {modkit.Audit("llm-proxies.delete")},
	"DELETE /orgs/{orgName}/self-hosted-gateways/{gatewayID}":                                        {modkit.Audit("gateways.delete"), modkit.AuditFailClosed()},
	"DELETE /orgs/{orgName}/self-hosted-gateways/{gatewayID}/tokens/{tokenID}":                       {modkit.Audit("gateway-tokens.revoke"), modkit.AuditFailClosed()},
	"GET /orgs/{orgName}/environments/{environmentId}/identity-providers":                            {modkit.Audit("environments.list-identity-providers")},
	"GET /orgs/{orgName}/gateways/{gatewayID}/identity-providers":                                    {modkit.Audit("gateways.list-identity-providers")},
	"GET /orgs/{orgName}/gateways/{gatewayID}/tokens":                                                {modkit.Audit("gateway-tokens.list")},
	"GET /orgs/{orgName}/llm-providers/{id}/api-keys":                                                {modkit.Audit("api-keys.list")},
	"GET /agents/{agentName}/api-keys":                                                               {modkit.Audit("api-keys.list")},
	"GET /agents/{agentName}/mcp-configs/{configId}/api-keys":                                        {modkit.Audit("api-keys.list")},
	"GET /agents/{agentName}/model-configs/{configId}/api-keys":                                      {modkit.Audit("api-keys.list")},
	"GET /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/api-keys":                              {modkit.Audit("api-keys.list")},
	"GET /orgs/{orgName}/self-hosted-gateways/{gatewayID}/tokens":                                    {modkit.Audit("gateway-tokens.list")},
	"GET /platform/gateways/failure-summary":                                                         {modkit.Audit("gateways.read"), modkit.AuditCoalesce(syncWindow)},
	"POST /orgs/{orgName}/environments/{envName}/agent-identities/groups":                            {modkit.Audit("agent-identities.create")},
	"POST /orgs/{orgName}/environments/{envName}/agent-identities/groups/{groupID}/members/add":      {modkit.Audit("agent-identities.add-group-member")},
	"POST /orgs/{orgName}/environments/{envName}/agent-identities/groups/{groupID}/members/remove":   {modkit.Audit("agent-identities.remove-group-member")},
	"POST /orgs/{orgName}/environments/{envName}/agent-identities/roles":                             {modkit.Audit("agent-identities.create")},
	"POST /orgs/{orgName}/environments/{envName}/agent-identities/roles/{roleID}/assignments/add":    {modkit.Audit("agent-identities.assign-role")},
	"POST /orgs/{orgName}/environments/{envName}/agent-identities/roles/{roleID}/assignments/remove": {modkit.Audit("agent-identities.unassign-role")},
	"POST /orgs/{orgName}/gateways":                                                                  {modkit.Audit("gateways.create"), modkit.EmitsWithoutData(gwaudit.GatewayCreated)},
	"POST /orgs/{orgName}/gateways/{gatewayID}/environments/{envID}":                                 {modkit.Audit("gateways.assign-environment")},
	"POST /orgs/{orgName}/gateways/{gatewayID}/tokens":                                               {modkit.Audit("gateway-tokens.rotate"), modkit.AuditFailClosed()},
	"POST /orgs/{orgName}/llm-provider-templates":                                                    {modkit.Audit("llm-provider-templates.create")},
	"POST /orgs/{orgName}/llm-providers/{id}/api-keys":                                               {modkit.Audit("api-keys.create"), modkit.AuditFailClosed()},
	"POST /orgs/{orgName}/llm-providers/{providerId}/deployments":                                    {modkit.Audit("llm-providers.deploy"), modkit.Emits(gwaudit.LLMProviderDeployed)},
	"POST /orgs/{orgName}/llm-providers/{providerId}/deployments/restore":                            {modkit.Audit("llm-providers.restore")},
	"POST /orgs/{orgName}/llm-providers/{providerId}/deployments/undeploy":                           {modkit.Audit("llm-providers.undeploy"), modkit.Emits(gwaudit.LLMProviderUndeployed)},
	"POST /agents/{agentName}/a2a-proxy/refresh":                                                     {modkit.Audit("agents.refresh-a2a-proxy")},
	"POST /agents/{agentName}/agent-card/refresh":                                                    {modkit.Audit("agents.refresh-card")},
	"POST /agents/{agentName}/api-keys":                                                              {modkit.Audit("api-keys.create"), modkit.EmitsWithoutData(gwaudit.AgentAPIKeyCreated), modkit.AuditFailClosed()},
	"POST /agents/{agentName}/api-keys/test":                                                         {modkit.Audit("api-keys.issue-test")},
	"POST /agents/{agentName}/mcp-configs":                                                           {modkit.Audit("agent-configs.update")},
	"POST /agents/{agentName}/mcp-configs/{configId}/api-keys":                                       {modkit.Audit("api-keys.create"), modkit.AuditFailClosed()},
	"POST /agents/{agentName}/model-configs":                                                         {modkit.Audit("agent-configs.update")},
	"POST /agents/{agentName}/model-configs/{configId}/api-keys":                                     {modkit.Audit("api-keys.create"), modkit.AuditFailClosed()},
	"POST /orgs/{orgName}/projects/{projName}/llm-proxies":                                           {modkit.Audit("llm-proxies.create")},
	"POST /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/api-keys":                             {modkit.Audit("api-keys.create"), modkit.AuditFailClosed()},
	"POST /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/deployments":                          {modkit.Audit("llm-proxies.deploy")},
	"POST /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/deployments/restore":                  {modkit.Audit("llm-proxies.restore")},
	"POST /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/deployments/undeploy":                 {modkit.Audit("llm-proxies.undeploy")},
	"POST /orgs/{orgName}/self-hosted-gateways":                                                      {modkit.Audit("gateways.create")},
	"POST /orgs/{orgName}/self-hosted-gateways/{gatewayID}/tokens":                                   {modkit.Audit("gateway-tokens.rotate"), modkit.AuditFailClosed()},
	"PUT /orgs/{orgName}/environments/{envName}/agent-identities/groups/{groupID}":                   {modkit.Audit("agent-identities.update")},
	"PUT /orgs/{orgName}/environments/{envName}/agent-identities/roles/{roleID}":                     {modkit.Audit("agent-identities.update")},
	"PUT /orgs/{orgName}/gateways/{gatewayID}":                                                       {modkit.Audit("gateways.update"), modkit.Emits(gwaudit.GatewayUpdated)},
	"PUT /orgs/{orgName}/gateways/{gatewayID}/identity-providers/{name}":                             {modkit.Audit("gateways.set-identity-provider"), modkit.AuditFailClosed(), modkit.AllowOrgOverride()},
	"PUT /orgs/{orgName}/llm-provider-templates/{templateId}":                                        {modkit.Audit("llm-provider-templates.update")},
	"PUT /orgs/{orgName}/llm-providers/{id}/api-keys/{keyName}":                                      {modkit.Audit("api-keys.rotate"), modkit.AuditFailClosed()},
	"PUT /agents/{agentName}/deploy-settings":                                                        {modkit.Audit("agents.update-deploy-settings"), modkit.Emits(gwaudit.AgentConfigurationSet)},
	"PUT /agents/{agentName}/a2a-proxy":                                                              {modkit.Audit("agents.set-a2a-proxy"), modkit.AuditFailClosed()},
	"PUT /agents/{agentName}/agent-card/source":                                                      {modkit.Audit("agents.set-card-source")},
	"PUT /agents/{agentName}/api-keys/{keyName}":                                                     {modkit.Audit("api-keys.rotate"), modkit.EmitsWithoutData(gwaudit.AgentAPIKeyRotated), modkit.AuditFailClosed()},
	"PUT /agents/{agentName}/mcp-configs/{configId}":                                                 {modkit.Audit("agent-configs.update")},
	"PUT /agents/{agentName}/mcp-configs/{configId}/api-keys/{keyName}":                              {modkit.Audit("api-keys.rotate"), modkit.AuditFailClosed()},
	"PUT /agents/{agentName}/model-configs/{configId}":                                               {modkit.Audit("agent-configs.update")},
	"PUT /agents/{agentName}/model-configs/{configId}/api-keys/{keyName}":                            {modkit.Audit("api-keys.rotate"), modkit.AuditFailClosed()},
	"PUT /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/api-keys/{keyName}":                    {modkit.Audit("api-keys.rotate"), modkit.AuditFailClosed()},
	"PUT /orgs/{orgName}/projects/{projName}/llm-proxies/{proxyId}":                                  {modkit.Audit("llm-proxies.update")},
	"PUT /orgs/{orgName}/self-hosted-gateways/{gatewayID}":                                           {modkit.Audit("gateways.update")},
}

// syncWindow bounds how often a polled route is recorded per caller. Gateways
// poll the bulk key-sync routes on a timer, and monitors poll the fleet
// failure summary; one record per caller per window still says who read what
// and when it started.
const syncWindow = 5 * time.Minute

// internalRouteOptions are the audit actions of the controller-facing routes
// below /api/internal/v1. Reads not listed are not recorded.
var internalRouteOptions = map[string][]modkit.RouteOption{
	"GET /llm-providers/api-keys":         {modkit.Audit("api-keys.sync"), modkit.AuditCoalesce(syncWindow)},
	"GET /llm-proxies/api-keys":           {modkit.Audit("api-keys.sync"), modkit.AuditCoalesce(syncWindow)},
	"GET /apis/api-keys":                  {modkit.Audit("api-keys.sync"), modkit.AuditCoalesce(syncWindow)},
	"POST /deployments/fetch-batch":       {modkit.Audit("deployments.sync")},
	"POST /artifacts/exists":              {modkit.Audit("artifacts.check-exists")},
	"POST /gateways/{gatewayId}/manifest": {modkit.Audit("gateways.push-manifest")},
	// Both hand plaintext credentials to the gateway, as deployments.sync does for a gateway that takes them
	// inline, and are recorded the same way. A gateway lists them once per connection.
	"GET /secrets":                {modkit.Audit("secrets.sync")},
	"GET /secrets/{handle}/value": {modkit.Audit("secrets.read")},
}
