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

import { isIdentityPromotionUiEnabled } from "@agent-management-platform/types";
import { usePipelineEnvironmentsState } from "./usePipelineEnvironments";

interface UseIdentityPromotedEnvironmentParams {
  orgId: string;
  projectId: string;
  envId: string;
}

/**
 * Whether an environment receives an agent's identity from the project's first
 * environment instead of having it created there. The platform promotes the
 * identity (agent application, roles, groups, MCP access) along the project's
 * deployment pipeline by itself, so in those environments roles and groups are
 * managed in the first environment and shown read only.
 *
 * Inert with the identity promotion UI flag off: the pipeline lookup gets no
 * ids, so nothing is requested and `isPromotedEnvironment` stays false. The
 * first environment is the head of the pipeline order, never the org's list
 * order, and a project whose pipeline has no promotion paths has none.
 */
export function useIdentityPromotedEnvironment({
  orgId, projectId, envId,
}: UseIdentityPromotedEnvironmentParams) {
  const enabled = isIdentityPromotionUiEnabled();
  const { environments, isLoading, hasPipelineOrder } = usePipelineEnvironmentsState(
    enabled ? orgId : undefined,
    enabled ? projectId : undefined,
  );
  const home = environments[0];
  // Without the pipeline's order (still loading, no promotion paths, or no
  // pipeline) the list is the org's unordered one, so no first environment is
  // known and nothing counts as promoted.
  const isPromotedEnvironment = enabled
    && !isLoading
    && hasPipelineOrder
    && !!home
    && home.name !== envId
    && environments.some((env) => env.name === envId);

  const current = environments.find((env) => env.name === envId);

  return {
    enabled,
    isLoading: enabled && isLoading,
    isPromotedEnvironment,
    /** Display name of `envId`, falling back to the name itself. */
    envLabel: current?.displayName || envId,
    /** Name of the project's first environment, where identity is managed. */
    homeEnvironment: home?.name,
    /** Its display name, for text shown to the user. */
    homeEnvironmentLabel: home?.displayName || home?.name,
  };
}
