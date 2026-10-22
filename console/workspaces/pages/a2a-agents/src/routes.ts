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

import { absoluteRouteMap } from '@agent-management-platform/types';
import { generatePath } from 'react-router-dom';

export const a2aAgentsRoute = absoluteRouteMap.children.org.children.a2aAgents;
export const a2aAgentViewRoute = a2aAgentsRoute.children.view;

export interface A2AAgentRouteParams {
  orgId: string;
  projectName: string;
  agentName: string;
}

export const a2aAgentsListPath = (orgId: string): string =>
  generatePath(a2aAgentsRoute.path, { orgId });

export const a2aAgentViewPath = (params: A2AAgentRouteParams): string =>
  generatePath(a2aAgentViewRoute.path, params);
