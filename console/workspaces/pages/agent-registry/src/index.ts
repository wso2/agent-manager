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

import type { PageMetadata } from "@agent-management-platform/types";
import { Library as RegistryIcon } from "@wso2/oxygen-ui-icons-react";
import { AgentRegistry } from "./AgentRegistry/AgentRegistry";
import { AgentRegistryOverview } from "./AgentRegistryOverview/AgentRegistryOverview";
import { AgentRegistryRoutes } from "./AgentRegistryRoutes";

export const metaData: PageMetadata = {
  title: "Agent registry",
  description: "Every agent in the organization, across all projects",
  icon: RegistryIcon,
  path: "/agent-registry",
  component: AgentRegistryRoutes,
  levels: {
    // The list and each agent's overview, routed inside the package.
    organization: AgentRegistryRoutes,
  },
};

export { AgentRegistry, AgentRegistryOverview, AgentRegistryRoutes };

export default AgentRegistryRoutes;
