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
    signInUrl: '$AUTH_BASE_URL/gate',
    afterSignInUrl: '$SIGN_IN_REDIRECT_URL',
    afterSignOutUrl: '$SIGN_OUT_REDIRECT_URL',
    scopes: ('$AUTH_SCOPES'.trim() || 'openid profile email').split(/\s+/).filter(Boolean),
    platform: 'AsgardeoV2',
    tokenValidation: {
      idToken: {
        validate: '$VALIDATE_ID_TOKEN' === 'true',
        clockTolerance: Number('$CLOCK_TOLERANCE') || 300,
      },
    },
    storage: 'localStorage',
  },
  disableAuth: '$DISABLE_AUTH' === 'true',
  apiBaseUrl: '$API_BASE_URL',
  obsApiBaseUrl: '$OBS_API_BASE_URL',
  gatewayControlPlaneUrl: '$GATEWAY_CONTROL_PLANE_URL',
  gatewayVersion: '$GATEWAY_VERSION',
  instrumentationUrl: '$INSTRUMENTATION_URL',
  guardrailsCatalogUrl: '$GUARDRAILS_CATALOG_URL',
  guardrailsDefinitionBaseUrl: '$GUARDRAILS_DEFINITION_BASE_URL',
};

