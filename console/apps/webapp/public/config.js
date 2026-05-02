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
    baseUrl: 'null',
    clientId: 'null',
    signInUrl: 'null/gate',
    afterSignInUrl: 'null',
    afterSignOutUrl: 'null',
    scopes: (''.trim() || 'openid profile email').split(/\s+/).filter(Boolean),
    platform: 'AsgardeoV2',
    tokenValidation: {
      idToken: {
        validate: '' === 'true',
        clockTolerance: Number('') || 300,
      },
    },
    storage: 'localStorage',
  },
  disableAuth: 'true' === 'true',
  apiBaseUrl: 'http://localhost:9000',
  obsApiBaseUrl: 'http://localhost:9098',
  gatewayControlPlaneUrl: '',
  gatewayVersion: '',
  instrumentationUrl: '',
  guardrailsCatalogUrl: 'https://db720294-98fd-40f4-85a1-cc6a3b65bc9a-prod.e1-us-east-azure.choreoapis.dev/api-platform/policy-hub-api/policy-hub-public/v1.0/policies?categories=Guardrails',
  guardrailsDefinitionBaseUrl: 'https://db720294-98fd-40f4-85a1-cc6a3b65bc9a-prod.e1-us-east-azure.choreoapis.dev/api-platform/policy-hub-api/policy-hub-public/v1.0/policies',
};

