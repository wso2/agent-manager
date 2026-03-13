/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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
    signInRedirectURL: 'null',
    signOutRedirectURL: 'null',
    clientID: 'null',
    baseUrl: 'null',
    scope: ['openid', 'profile'],
    storage: 'sessionStorage',
    // Disable strict ID token validation for providers with non-standard issuers 
    // (e.g., Thunder uses "thunder" instead of a URL)
    // Set VALIDATE_ID_TOKEN=true for providers that comply with OIDC standards (e.g., Asgardeo)
    validateIDToken: '' === 'true',
    // Clock tolerance (in seconds) to handle time skew between client and server
    // Prevents token validation failures due to minor time differences
    clockTolerance: 300
  },
  disableAuth: 'true' === 'true',
  apiBaseUrl: 'http://localhost:9000',
  gatewayControlPlaneUrl: 'http://localhost:9243',
  gatewayVersion: '',
  instrumentationUrl: '',
  guardrailsCatalogUrl: 'https://db720294-98fd-40f4-85a1-cc6a3b65bc9a-prod.e1-us-east-azure.choreoapis.dev/api-platform/policy-hub-api/policy-hub-public/v1.0/policies?categories=Guardrails%2CAI',
  guardrailsDefinitionBaseUrl: 'http://localhost:9000/api/v1/guardrails/definitions',
};

