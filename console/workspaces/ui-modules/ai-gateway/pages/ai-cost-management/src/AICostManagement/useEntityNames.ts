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
  useListLLMProviders,
  useListOrgAgents,
  useListProjects,
} from "@agent-management-platform/api-client";
import type { CostNames } from "./costFormat";

/**
 * Display names of the organization's LLM providers, projects and agents, keyed by the
 * handles budgets and cost data use (agents as project/agent). A handle with no entry is
 * shown as is.
 */
export function useEntityNames(orgName: string): CostNames {
  const providers = useListLLMProviders({ orgName });
  const projects = useListProjects({ orgName }, { limit: 100 });
  const agents = useListOrgAgents({ orgName });
  return useMemo<CostNames>(
    () => ({
      llmProvider: Object.fromEntries(
        (providers.data?.providers ?? []).map((p) => [p.id, p.name || p.id]),
      ),
      project: Object.fromEntries(
        (projects.data?.projects ?? []).map((p) => [
          p.name,
          p.displayName || p.name,
        ]),
      ),
      agent: Object.fromEntries(
        (agents.data?.agents ?? []).map((a) => [
          `${a.projectName}/${a.name}`,
          a.displayName || a.name,
        ]),
      ),
    }),
    [providers.data, projects.data, agents.data],
  );
}
