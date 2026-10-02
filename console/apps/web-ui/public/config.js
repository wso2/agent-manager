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

window.__RUNTIME_CONFIG__ = {
  authConfig: {
    baseUrl: 'http://thunder.amp.localhost:8080',
    clientId: 'amp-console-client',
    organizationHandle: (''.trim() || 'default'),
    signInUrl: 'http://thunder.amp.localhost:8080/gate',
    afterSignInUrl: 'http://localhost:3000/login',
    afterSignOutUrl: 'http://localhost:3000/login',
    scopes: ('openid profile email amp:org:view amp:org:modify-settings amp:org:invite-member amp:org:remove-member amp:org:assign-role amp:org:manage-idp amp:org:manage-service-account amp:project:create amp:project:read amp:project:update amp:project:delete amp:environment:create amp:environment:read amp:environment:update amp:environment:delete amp:gateway:create amp:gateway:read amp:gateway:update amp:gateway:delete amp:gateway:token-manage amp:data-plane:read amp:deployment-pipeline:create amp:deployment-pipeline:read amp:deployment-pipeline:update amp:deployment-pipeline:delete amp:git-secret:create amp:git-secret:read amp:git-secret:delete amp:llm-provider-template:create amp:llm-provider-template:read amp:llm-provider-template:update amp:llm-provider-template:delete amp:llm-provider:create amp:llm-provider:read amp:llm-provider:update amp:llm-provider:delete amp:llm-provider:configure-guardrail amp:llm-provider:connect amp:llm-provider:deploy amp:llm-provider:api-key-manage amp:mcp-server:create amp:mcp-server:read amp:mcp-server:update amp:mcp-server:delete amp:mcp-server:configure-guardrail amp:mcp-server:connect amp:mcp-server:api-key-manage amp:llm-proxy:create amp:llm-proxy:read amp:llm-proxy:update amp:llm-proxy:delete amp:llm-proxy:deploy amp:llm-proxy:api-key-manage amp:evaluator:create amp:evaluator:read amp:evaluator:update amp:evaluator:delete amp:agent:create amp:agent:read amp:agent:update amp:agent:delete amp:agent:build amp:agent:env-non-production amp:agent:env-production amp:agent:rollback amp:agent:suspend amp:agent:token-manage amp:agent:api-key-manage amp:monitor:create amp:monitor:read amp:monitor:update amp:monitor:delete amp:monitor:execute amp:monitor:score-read amp:monitor:score-publish amp:alerting:read amp:alerting:manage amp:observability:trace-read amp:observability:log-read amp:observability:build-log-read amp:observability:metric-read amp:role:create amp:role:read amp:role:update amp:role:delete amp:group:create amp:group:read amp:group:update amp:group:delete amp:catalog:read amp:repository:read amp:agent-kind:read amp:agent-kind:create amp:agent-kind:update amp:agent-kind:delete amp:profile:read amp:profile:update-attributes amp:scope:create amp:scope:read amp:scope:update amp:scope:delete amp:agent-identity:read amp:agent-identity:create amp:agent-identity:update amp:agent-identity:delete'.trim() || 'openid profile email').split(/\s+/).filter(Boolean),
    // RFC 8707 resource indicator. Names the resource server a permission-bearing
    // token binds to ("urn:wso2:amp"). Left empty the sign-in request is
    // byte-identical to before and the token audience stays the client_id.
    //
    // It has to reach /oauth2/authorize, not just /oauth2/token: the sign-in
    // flow's AuthorizationExecutor reads it to decide which resource server to
    // evaluate the requested permission scopes against, and drops every
    // permission scope it cannot resolve there. A token-time resource only
    // narrows what the authorization code already carries, so it arrives too
    // late. signInOptions is forwarded verbatim as custom authorize params,
    // which is exactly what is needed here.
    //
    // Deliberately NOT also set on tokenRequest: the authorize-time resource is
    // persisted onto the authorization code and the token endpoint falls back to
    // it, so a token-side copy is redundant and actively unsafe — the SDK
    // replays those params with no empty-value guard, and a blank one reaches
    // the token endpoint as `resource=`, which Thunder rejects with
    // invalid_target "must be an absolute URI".
    //
    // Only set this alongside the matching amp: scopes in AUTH_SCOPES, and only
    // once the agent-manager-service audience allowlist (KEY_MANAGER_AUDIENCE)
    // accepts the value — the token's aud becomes the resource server identifier
    // instead of the client_id.
    ...('urn:wso2:amp'.trim() && {
      signInOptions: { resource: 'urn:wso2:amp'.trim() },
    }),
    tokenValidation: {
      idToken: {
        validate: '' === 'true',
        clockTolerance: Number('') || 300,
      },
    },
    tokenLifecycle: {
      refreshToken: {
        autoRefresh: true,
      },
    },
    rpInitiatedLogout: false,
    storage: 'localStorage',
  },
  disableAuth: 'false' === 'true',
  rbacEnabled: 'true' === 'true',
  apiBaseUrl: 'http://localhost:9000',
  configDiscoveryBaseUrl: '',
  gatewayControlPlaneUrl: 'http://localhost:9243',
  gatewayVersion: 'v0.11.0',
  ampVersion: 'v0.16.0',
  instrumentationUrl: 'http://default-default.gateway.localhost:19080/otel',
  agentManagerInternalBaseUrl: 'http://host.docker.internal:9000',
  agentManagerInternalCpHost: 'host.docker.internal:9243',
  thunderHostBaseDomain: 'amp.localhost',
  tlsEnabled: 'false' === 'true',
  guardrailsCatalogUrl: 'https://db720294-98fd-40f4-85a1-cc6a3b65bc9a-prod.e1-us-east-azure.choreoapis.dev/api-platform/policy-hub-api/policy-hub-public/v1.0/policies?categories=Guardrails,AI&limit=100',
  guardrailsDefinitionBaseUrl: 'https://db720294-98fd-40f4-85a1-cc6a3b65bc9a-prod.e1-us-east-azure.choreoapis.dev/api-platform/policy-hub-api/policy-hub-public/v1.0/policies',
  guardrailCapabilities: {
    awsBedrock: '' === 'true',
    azureContentSafety: '' === 'true',
    graniteGuardian: '' === 'true',
    nemoGuard: '' === 'true',
    semanticGuardrails: '' === 'true',
  },
  featureFlags: {
    enablePrivateRepoSupport: 'true' === 'true',
    enableIdentityProviderManagedMode: '' === 'true',
    enableProfileManagement: 'true' === 'true',
    enableUserManagement: 'true' === 'true',
    enableAgentIdentity: true,
  },
  docsUrl: 'https://wso2.com/agent-platform/docs/latest/',
  footerLinks: {
    privacyPolicyUrl: 'https://wso2.com/agent-platform/agent-manager/',
    termsOfUseUrl: 'https://wso2.com/agent-platform/agent-manager/',
  },
  instrumentationDocLinks: {
    manualInstrumentation: '/guides/amp-instrumentation/#manual-instrumentation',
    versionMapping: '/guides/amp-instrumentation/#amp-instrumentation-version-mapping',
  },
};
