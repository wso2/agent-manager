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

import type {
  AgentCardPathParams,
  AgentCardResponse,
  SetAgentCardSourceRequest,
} from "@agent-management-platform/types";
import {
  agentGatewayPath,
  agentRequestOptions,
  httpDELETE,
  httpGET,
  httpPOST,
  httpPUT,
} from "../../shared/utils";

function agentCardPath(params: AgentCardPathParams, suffix = ""): string {
  return agentGatewayPath(params, `/agent-card${suffix}`);
}

// The http helpers throw on non-2xx, so a 404 surfaces as an error with status 404.
export async function getAgentCard(
  params: AgentCardPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentCardResponse> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(agentCardPath(params), agentRequestOptions(params, token));
  return res.json();
}

// 202/204 carry no body, so these resolve to void.
export async function refreshAgentCard(
  params: AgentCardPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const token = getToken ? await getToken() : undefined;
  await httpPOST(agentCardPath(params, "/refresh"), {}, agentRequestOptions(params, token));
}

export async function setAgentCardSource(
  params: AgentCardPathParams,
  body: SetAgentCardSourceRequest,
  getToken?: () => Promise<string>,
): Promise<void> {
  const token = getToken ? await getToken() : undefined;
  await httpPUT(agentCardPath(params, "/source"), body, agentRequestOptions(params, token));
}

export async function deleteAgentCardSource(
  params: AgentCardPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const token = getToken ? await getToken() : undefined;
  await httpDELETE(agentCardPath(params, "/source"), agentRequestOptions(params, token));
}
