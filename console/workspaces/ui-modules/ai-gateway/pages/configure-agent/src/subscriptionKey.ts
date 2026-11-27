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

/** The policy that checks a subscription key on an MCP server's endpoint. */
const SUBSCRIPTION_VALIDATION = "subscription-validation";

/** The header the policy reads the key from when its subscriptionKeyHeader parameter is unset. */
export const DEFAULT_SUBSCRIPTION_KEY_HEADER = "Subscription-Key";

type EndpointWithPolicies = {
  policies?: readonly { name: string; params?: Record<string, unknown> }[];
};

/**
 * The header an MCP server's endpoint reads a subscription key from, when subscription-validation
 * is attached to it; undefined when it isn't, so no key is needed.
 */
export function subscriptionKeyHeaderOf(endpoint?: EndpointWithPolicies): string | undefined {
  const policy = endpoint?.policies?.find((p) => p.name === SUBSCRIPTION_VALIDATION);
  if (!policy) return undefined;
  const header = policy.params?.subscriptionKeyHeader;
  return typeof header === "string" && header.trim() !== ""
    ? header.trim()
    : DEFAULT_SUBSCRIPTION_KEY_HEADER;
}

/** The cURL line that sends a subscription key in the header. */
export const subscriptionKeyCurlLine = (header: string) =>
  `  --header "${header}: <subscription-key>"`;
