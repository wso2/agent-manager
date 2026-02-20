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
    signInRedirectURL: '$SIGN_IN_REDIRECT_URL',
    signOutRedirectURL: '$SIGN_OUT_REDIRECT_URL',
    clientID: '$AUTH_CLIENT_ID',
    baseUrl: '$AUTH_BASE_URL',
    scope: ['openid', 'profile'],
    storage: 'sessionStorage',
    // Disable strict ID token validation for providers with non-standard issuers 
    // (e.g., Thunder uses "thunder" instead of a URL)
    // Set VALIDATE_ID_TOKEN=true for providers that comply with OIDC standards (e.g., Asgardeo)
    validateIDToken: '$VALIDATE_ID_TOKEN' === 'true',
    // Clock tolerance (in seconds) to handle time skew between client and server
    // Prevents token validation failures due to minor time differences
    clockTolerance: 300
  },
  disableAuth: '$DISABLE_AUTH' === 'true',
  apiBaseUrl: '$API_BASE_URL',
  instrumentationUrl: '$INSTRUMENTATION_URL',
};

