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

import type { GuardrailDefinition } from "@agent-management-platform/api-client";
import {
  normalizeRootSchema,
  type PolicyDefinition,
} from "../../utils/policyParameterEditor";

/**
 * Builds the configuration schema from the catalog item's inline `parameters`.
 * The service omits empty `parameters`, so an installed policy may arrive with
 * none: that is an empty schema (no settings), not a missing one. Not-installed
 * ones never carry a schema and are not selectable, so `null` means no schema is
 * known.
 */
export function resolvePolicyDetail(
  policy: GuardrailDefinition,
): PolicyDefinition | null {
  if (policy.parameters === undefined && policy.status !== "installed") {
    return null;
  }
  return {
    name: policy.name,
    version: policy.version,
    description: policy.description ?? "",
    parameters: normalizeRootSchema(policy.parameters),
    systemParameters: normalizeRootSchema(policy.systemParameters),
  };
}
