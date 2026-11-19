/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import type { FeatureAccessConfig } from '@agent-management-platform/am-core-ui';

type ScopeRule = { anyOf: string[] };

const crud = (resource: string) => [`amp:${resource}:create`, `amp:${resource}:update`, `amp:${resource}:delete`];

/** Read = the resource's read scope; write = any of its create, update or delete scopes. */
const resource = (name: string) => ({
  read: { anyOf: [`amp:${name}:read`] },
  write: { anyOf: crud(name) },
});

/** A resource of the AI Gateway module, whose scopes are amp:ai-gateway:<resource>.<action>. */
const gatewayScope = (name: string, action: string) => `amp:ai-gateway:${name}.${action}`;
const gatewayResource = (name: string) => ({
  read: { anyOf: [gatewayScope(name, 'read')] },
  write: { anyOf: ['create', 'update', 'delete'].map((action) => gatewayScope(name, action)) },
});

const project = resource('project');
const agent = resource('agent');
const agentKind = resource('agent-kind');
const agentIdentity = gatewayResource('agent-identities');
const environment = resource('environment');
const gateway = gatewayResource('gateways');
const pipeline = resource('deployment-pipeline');
const llmProvider = gatewayResource('llm-providers');
const llmProviderTemplate = gatewayResource('llm-provider-templates');
const llmProxy = gatewayResource('llm-proxies');
const mcpServer = gatewayResource('mcp-servers');
const evaluator = resource('evaluator');
const monitor = resource('monitor');
const members = ['amp:org:invite-member', 'amp:org:remove-member'];
const roleScopes = ['amp:role:read', ...crud('role')];
const groupScopes = ['amp:group:read', ...crud('group')];

/** Which token scopes grant each access key. */
export const SCOPE_RULES: Record<keyof FeatureAccessConfig, ScopeRule> = {
  project_read: project.read,
  project_write: project.write,
  agent_read: agent.read,
  agent_write: agent.write,
  agent_create: { anyOf: ['amp:agent:create'] },
  agent_delete: { anyOf: ['amp:agent:delete'] },
  agent_kind_read: agentKind.read,
  agent_kind_write: agentKind.write,
  agent_identity_read: agentIdentity.read,
  agent_identity_write: agentIdentity.write,
  env_read: environment.read,
  env_write: environment.write,
  gateway_read: gateway.read,
  gateway_write: gateway.write,
  gateway_token_write: { anyOf: [gatewayScope('gateways', 'token-manage')] },
  pipeline_read: pipeline.read,
  pipeline_write: pipeline.write,
  llm_provider_read: llmProvider.read,
  llm_provider_write: llmProvider.write,
  llm_provider_api_key_manage: { anyOf: [gatewayScope('llm-providers', 'api-key-manage')] },
  llm_provider_template_read: llmProviderTemplate.read,
  llm_provider_template_write: llmProviderTemplate.write,
  llm_proxy_read: llmProxy.read,
  llm_proxy_write: llmProxy.write,
  mcp_server_read: mcpServer.read,
  mcp_server_write: mcpServer.write,
  // Subscription plans (the AI Hub module, on by default).
  subscription_plan_read: { anyOf: ['amp:aihub:plan-read'] },
  subscription_plan_write: {
    anyOf: ['amp:aihub:plan-create', 'amp:aihub:plan-update', 'amp:aihub:plan-delete'],
  },
  evaluator_read: evaluator.read,
  evaluator_write: evaluator.write,
  monitor_read: monitor.read,
  monitor_write: monitor.write,
  monitor_execute: { anyOf: ['amp:monitor:execute'] },
  // The Users page has no read scope of its own; either member scope opens it.
  member_read: { anyOf: members },
  member_invite: { anyOf: ['amp:org:invite-member'] },
  member_remove: { anyOf: ['amp:org:remove-member'] },
  // Any role or group scope opens its page, as the nav did before.
  role_read: { anyOf: roleScopes },
  role_write: { anyOf: crud('role') },
  group_read: { anyOf: groupScopes },
  group_write: { anyOf: crud('group') },
  agent_deploy: { anyOf: [gatewayScope('deploy', 'non-production')] },
  agent_deploy_production: { anyOf: [gatewayScope('deploy', 'production')] },
  agent_suspend: { anyOf: ['amp:agent:suspend'] },
  agent_build: { anyOf: ['amp:agent:build'] },
  agent_api_key_manage: { anyOf: [gatewayScope('agent-api-keys', 'manage')] },
  agent_token_manage: { anyOf: ['amp:agent:token-manage'] },
};

export function accessFromScopes(scopes: ReadonlySet<string>): FeatureAccessConfig {
  const entries = Object.entries(SCOPE_RULES).map(([key, rule]) => [
    key,
    rule.anyOf.some((scope) => scopes.has(scope)),
  ]);
  return Object.fromEntries(entries) as FeatureAccessConfig;
}
