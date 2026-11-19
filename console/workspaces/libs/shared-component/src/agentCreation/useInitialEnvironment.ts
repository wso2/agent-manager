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

import { useMemo } from "react";
import {
  useGetDeploymentPipeline,
  useIsSingleEnvironment,
  useListEnvironments,
} from "@agent-management-platform/api-client";
import type { OrgProjPathParams } from "@agent-management-platform/types";
import { findLowestEnvironmentName, hasMultipleEnvironments } from "./buildAgentPayload";

export interface InitialEnvironment {
  /** The environment the create-time config (LLM providers, MCP servers) applies to. */
  initialEnvironmentName: string | undefined;
  /** True when the agent will have more environments than the initial one. */
  multipleEnvironments: boolean;
  /** True until the initial environment is known; hold off submitting until then. */
  isLoading: boolean;
}

/**
 * The environment a new agent's create-time config applies to.
 *
 * With the agent runtime, that is the lowest environment of the project's
 * deployment pipeline. Without it there is no pipeline, so it is the org's
 * first environment, by its real name from the environments API (the selection
 * maps in the LLM and MCP sections are keyed by that name).
 */
export function useInitialEnvironment(params: OrgProjPathParams): InitialEnvironment {
  const singleEnv = useIsSingleEnvironment();
  const { data: pipeline, isLoading: isPipelineLoading } = useGetDeploymentPipeline(params);
  const { data: environments, isLoading: isEnvironmentsLoading } = useListEnvironments({
    orgName: params.orgName,
  });

  return useMemo<InitialEnvironment>(() => {
    // Still detecting whether the agent runtime is present: the source of the
    // initial environment is unknown, so this is loading, not ready.
    if (singleEnv === undefined) {
      return { initialEnvironmentName: undefined, multipleEnvironments: false, isLoading: true };
    }
    if (singleEnv) {
      return {
        initialEnvironmentName: environments?.[0]?.name,
        multipleEnvironments: (environments?.length ?? 0) > 1,
        isLoading: isEnvironmentsLoading,
      };
    }
    return {
      initialEnvironmentName: findLowestEnvironmentName(pipeline?.promotionPaths),
      multipleEnvironments: hasMultipleEnvironments(pipeline?.promotionPaths),
      isLoading: isPipelineLoading,
    };
  }, [singleEnv, environments, isEnvironmentsLoading, pipeline, isPipelineLoading]);
}
