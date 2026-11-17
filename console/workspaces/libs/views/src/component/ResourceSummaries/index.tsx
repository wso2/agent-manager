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

import { createContext, useContext } from "react";
import type { PluginResourceSummaryContribution } from "@agent-management-platform/types";

/** An active resource-summary contribution, as the shell's plugin host resolved it. */
export interface ResourceSummaryOption extends PluginResourceSummaryContribution {
  /** Unique across plugins: "<plugin>:<contribution>". */
  key: string;
  pluginId: string;
}

/**
 * The resource cards modules contribute to the organization overview (in
 * acp-core): active ones only, sorted by `order`. Each card's count comes from
 * its own `useSummary` hook, which the overview runs.
 */
const ResourceSummariesContext = createContext<ResourceSummaryOption[]>([]);

export const ResourceSummariesProvider = ResourceSummariesContext.Provider;

/** Active resource summaries, sorted; empty outside a plugin host (tests, stories). */
export function useResourceSummaries(): ResourceSummaryOption[] {
  return useContext(ResourceSummariesContext);
}
