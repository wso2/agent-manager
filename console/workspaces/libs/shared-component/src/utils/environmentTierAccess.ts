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

import type {
  FeatureAccessKey,
  FeatureManagement,
} from "@agent-management-platform/feature-management";

/**
 * The environment tier decides where an action may land. `agent_deploy` allows
 * non-production environments; `agent_deploy_production` is needed in addition for
 * environments flagged isProduction. This mirrors the server's rule so the console
 * doesn't offer a button that will 403; the server is still the enforcement point.
 */

/** The parts of feature management the rule reads. */
export type AccessState = Pick<FeatureManagement, "isAccessLoading" | "hasAccess">;

/** Only the part of an Environment that decides the tier. */
export interface EnvironmentTier {
  isProduction?: boolean;
}

export interface AccessDecision {
  allowed: boolean;
  /** The access key whose absence decided it. Undefined when allowed. */
  missingAccess?: FeatureAccessKey;
  /** A sentence for the disabled control's tooltip. Empty when allowed. */
  reason: string;
}

export const ALLOWED: AccessDecision = { allowed: true, reason: "" };

function deny(missingAccess: FeatureAccessKey, what: string): AccessDecision {
  return {
    allowed: false,
    missingAccess,
    reason: `You do not have permission to ${what}.`,
  };
}

/**
 * Whether the user may act on `environment`, given any extra capability the
 * operation needs (e.g. `agent_suspend`). Checks run in the server's order:
 * capability, then the non-production floor, then production.
 *
 * Allows everything while access is loading, and checks only the floor while the
 * environment (and so its tier) is unknown, so controls don't flash disabled.
 */
export function evaluateAgentEnvironmentAccess(
  state: AccessState,
  environment: EnvironmentTier | undefined,
  capability?: FeatureAccessKey,
): AccessDecision {
  if (state.isAccessLoading) return ALLOWED;
  if (capability && !state.hasAccess(capability)) {
    return deny(capability, "perform this action");
  }
  if (!state.hasAccess("agent_deploy")) {
    return deny("agent_deploy", "act on deployment environments");
  }
  if (environment?.isProduction && !state.hasAccess("agent_deploy_production")) {
    return deny("agent_deploy_production", "act on production environments");
  }
  return ALLOWED;
}
