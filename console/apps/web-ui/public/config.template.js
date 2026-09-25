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
    baseUrl: '$AUTH_BASE_URL',
    clientId: '$AUTH_CLIENT_ID',
    organizationHandle: ('$AUTH_ORG_HANDLE'.trim() || 'default'),
    signInUrl: '$AUTH_BASE_URL/gate',
    afterSignInUrl: '$SIGN_IN_REDIRECT_URL',
    afterSignOutUrl: '$SIGN_OUT_REDIRECT_URL',
    scopes: ('$AUTH_SCOPES'.trim() || 'openid profile email').split(/\s+/).filter(Boolean),
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
    ...('$AUTH_RESOURCE'.trim() && {
      signInOptions: { resource: '$AUTH_RESOURCE'.trim() },
    }),
    tokenValidation: {
      idToken: {
        validate: '$VALIDATE_ID_TOKEN' === 'true',
        clockTolerance: Number('$CLOCK_TOLERANCE') || 300,
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
  disableAuth: '$DISABLE_AUTH' === 'true',
  apiBaseUrl: '$API_BASE_URL',
  configDiscoveryBaseUrl: '$CONFIG_DISCOVERY_BASE_URL',
  gatewayControlPlaneUrl: '$GATEWAY_CONTROL_PLANE_URL',
  gatewayVersion: '$GATEWAY_VERSION',
  ampVersion: '$AMP_VERSION',
  scriptBaseUrl: '$SCRIPT_BASE_URL',
  instrumentationUrl: '$INSTRUMENTATION_URL',
  agentManagerInternalBaseUrl: '$AGENT_MANAGER_INTERNAL_BASE_URL',
  agentManagerInternalCpHost: '$AGENT_MANAGER_INTERNAL_CP_HOST',
  idpHostBaseDomain: '$IDP_HOST_BASE_DOMAIN',
  tlsEnabled: '$TLS_ENABLED' === 'true',
  guardrailsCatalogUrl: '$GUARDRAILS_CATALOG_URL',
  guardrailsDefinitionBaseUrl: '$GUARDRAILS_DEFINITION_BASE_URL',
  guardrailCapabilities: {
    awsBedrock: '$GUARDRAIL_CAP_AWS_BEDROCK' === 'true',
    azureContentSafety: '$GUARDRAIL_CAP_AZURE_CONTENT_SAFETY' === 'true',
    graniteGuardian: '$GUARDRAIL_CAP_GRANITE_GUARDIAN' === 'true',
    nemoGuard: '$GUARDRAIL_CAP_NEMO_GUARD' === 'true',
    semanticGuardrails: '$GUARDRAIL_CAP_SEMANTIC_GUARDRAILS' === 'true',
  },
  featureFlags: {
    enablePrivateRepoSupport: '$FEATURE_FLAG_ENABLE_PRIVATE_REPO_SUPPORT' === 'true',
    enableIdentityProviderManagedMode: '$FEATURE_FLAG_ENABLE_IDENTITY_PROVIDER_MANAGED_MODE' === 'true',
    enableProfileManagement: '$FEATURE_FLAG_ENABLE_PROFILE_MANAGEMENT' === 'true',
    enableUserManagement: '$FEATURE_FLAG_ENABLE_USER_MANAGEMENT' === 'true',
    enableAgentIdentity: true,
  },
  maxRequestBodyBytes: '$MAX_REQUEST_BODY_BYTES',
  fileMountMaxFileBytes: '$FILE_MOUNT_MAX_FILE_BYTES',
  docsUrl: '$DOCS_URL',
  footerLinks: {
    privacyPolicyUrl: 'https://wso2.com/agent-platform/agent-manager/',
    termsOfUseUrl: 'https://wso2.com/agent-platform/agent-manager/',
  },
  instrumentationDocLinks: {
    manualInstrumentation: '/guides/amp-instrumentation/#manual-instrumentation',
    versionMapping: '/guides/amp-instrumentation/#amp-instrumentation-version-mapping',
  },
};
