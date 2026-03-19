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

import type { AgentPathParams } from "./common";

// -----------------------------------------------------------------------------
// Resource config schemas
// -----------------------------------------------------------------------------

export interface ResourceRequests {
  cpu?: string;
  memory?: string;
}

export interface ResourceLimits {
  cpu?: string;
  memory?: string;
}

export interface ResourceConfig {
  requests?: ResourceRequests;
  limits?: ResourceLimits;
}

export interface AutoScalingConfig {
  enabled?: boolean;
  minReplicas?: number;
  maxReplicas?: number;
}

// -----------------------------------------------------------------------------
// Request / Response types
// -----------------------------------------------------------------------------

export interface UpdateAgentResourceConfigsRequest {
  replicas: number;
  resources: ResourceConfig;
  autoScaling: AutoScalingConfig;
}

export interface AgentResourceConfigsResponse {
  replicas?: number;
  resources?: ResourceConfig;
  autoScaling?: AutoScalingConfig;
}

// -----------------------------------------------------------------------------
// Path params and query
// -----------------------------------------------------------------------------

export type GetAgentResourceConfigsPathParams = AgentPathParams;
export type UpdateAgentResourceConfigsPathParams = AgentPathParams;

export interface GetAgentResourceConfigsQuery {
  environment?: string;
}

export interface UpdateAgentResourceConfigsQuery {
  environment?: string;
}
