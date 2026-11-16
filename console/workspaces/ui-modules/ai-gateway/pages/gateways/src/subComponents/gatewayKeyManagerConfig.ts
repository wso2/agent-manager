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

export type ManageIdentityProviderMode = "upsert" | "delete";

export interface KeyManagerInputs {
  name: string;
  issuer: string;
  jwksUri: string;
  skipTlsVerify: boolean;
}

/** TOML basic string, escaped so an entered value cannot break the document. */
function tomlString(value: string): string {
  return `"${value.replace(/\\/g, "\\\\").replace(/"/g, '\\"')}"`;
}

/**
 * The key manager entry a self-hosted gateway needs in its configs/config.toml
 * to accept tokens from an identity provider. The gateway reads it at start, so
 * the gateway is restarted after adding or removing it.
 */
export function buildKeyManagerConfig(i: KeyManagerInputs): string {
  return [
    "[[policy_configurations.jwtauth_v1.keymanagers]]",
    `name = ${tomlString(i.name || "<identity-provider-name>")}`,
    `issuer = ${tomlString(i.issuer || "<issuer>")}`,
    "[policy_configurations.jwtauth_v1.keymanagers.jwks.remote]",
    `uri = ${tomlString(i.jwksUri || "<jwks-uri>")}`,
    `skipTlsVerify = ${i.skipTlsVerify ? "true" : "false"}`,
  ].join("\n");
}
