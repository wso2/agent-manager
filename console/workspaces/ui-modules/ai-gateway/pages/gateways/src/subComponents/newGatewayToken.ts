/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

/**
 * Hands a new gateway's registration token from the add page to the setup page.
 *
 * The token is shown only once, so it is kept in memory only: never in the URL, in
 * router state (which the browser keeps in session history), or in storage. A reload
 * loses it, and the setup page then offers Reconfigure to generate a new one.
 */
const tokens = new Map<string, string>();

export function keepNewGatewayToken(gatewayId: string, token: string): void {
  tokens.set(gatewayId, token);
}

export function newGatewayToken(gatewayId: string): string | null {
  return tokens.get(gatewayId) ?? null;
}

export function forgetNewGatewayToken(gatewayId: string): void {
  tokens.delete(gatewayId);
}
