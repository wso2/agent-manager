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
import type { PluginAgentTypeContribution } from "@agent-management-platform/types";

/** An active agent-type contribution, as the shell's plugin host resolved it. */
export interface AgentTypeOption extends PluginAgentTypeContribution {
  /** Unique across plugins: "<plugin>:<contribution>". */
  key: string;
  pluginId: string;
}

export interface AgentTypesState {
  /** Active agent types, sorted by `order`. */
  agentTypes: AgentTypeOption[];
  /**
   * True while the page's content may still change: an agent type, or a panel
   * at the page's mount point, has not settled (activation or its module's
   * install detection still running). Wait for false before acting on it.
   */
  isPending: boolean;
}

/**
 * The agent types modules contribute to the "Add a New Agent" page. Lets that
 * page (in acp-core) and the create forms list them without importing the
 * shell or the modules that own them.
 */
const AgentTypesContext = createContext<AgentTypesState>({ agentTypes: [], isPending: false });

export const AgentTypesProvider = AgentTypesContext.Provider;

/** Active agent types and whether more may still activate; empty outside a plugin host. */
export function useAgentTypesState(): AgentTypesState {
  return useContext(AgentTypesContext);
}

/** Active agent types, sorted; empty outside a plugin host (tests, stories). */
export function useAgentTypes(): AgentTypeOption[] {
  return useContext(AgentTypesContext).agentTypes;
}
