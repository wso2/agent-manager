/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

/**
 * The gateway registers the built-in token issuer as `ThunderKeyManager`. That name is an
 * identifier (gateway config, MCP policies, API paths), so it stays as is; only the text shown
 * to people changes.
 */
const SYSTEM_IDENTITY_PROVIDER_LABELS: Readonly<Record<string, string>> = {
  ThunderKeyManager: 'DefaultKeyManager',
};

/** The text that names an identity provider (token issuer) on screen. */
export function identityProviderLabel(name: string | undefined): string {
  if (!name) {
    return '';
  }
  return SYSTEM_IDENTITY_PROVIDER_LABELS[name] ?? name;
}
