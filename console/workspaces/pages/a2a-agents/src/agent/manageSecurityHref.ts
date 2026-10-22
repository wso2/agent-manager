/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { generatePath } from 'react-router-dom';
import { absoluteRouteMap, type AgentCardSource } from '@agent-management-platform/types';
import { buildConfigureTabHref } from '@agent-management-platform/views';

/** Where an agent's security is edited: the A2A configure tab (external) or deployment. */
export function manageSecurityHref(
  kind: AgentCardSource,
  orgId: string,
  projectName: string,
  agentName: string,
  environment: string,
): string {
  if (kind === 'external') {
    return buildConfigureTabHref(orgId, projectName, agentName, 'a2a', environment);
  }
  return generatePath(
    absoluteRouteMap.children.org.children.projects.children.agents.children.deployment.path,
    { orgId, projectId: projectName, agentId: agentName },
  );
}
