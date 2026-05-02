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

import {
  CreateAgentAPIKeyPathParams,
  CreateAgentAPIKeyRequest,
  CreateAgentAPIKeyResponse,
  ListAgentAPIKeysPathParams,
  AgentAPIKeyListResponse,
  RevokeAgentAPIKeyPathParams,
} from "@agent-management-platform/types";
import { encodeRequired, httpDELETE, httpGET, httpPOST, SERVICE_BASE } from "../utils";

function agentAPIKeysURL(params: ListAgentAPIKeysPathParams): string {
  const org = encodeRequired(params.orgName, "orgName");
  const proj = encodeRequired(params.projName, "projName");
  const agent = encodeRequired(params.agentName, "agentName");
  return `${SERVICE_BASE}/orgs/${org}/projects/${proj}/agents/${agent}/api-keys`;
}

export async function createAgentAPIKey(
  params: CreateAgentAPIKeyPathParams,
  body: CreateAgentAPIKeyRequest,
  getToken?: () => Promise<string>,
): Promise<CreateAgentAPIKeyResponse> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(agentAPIKeysURL(params), body, { token });
  return res.json();
}

export async function createAgentTestAPIKey(
  params: CreateAgentAPIKeyPathParams,
  getToken?: () => Promise<string>,
): Promise<CreateAgentAPIKeyResponse> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(`${agentAPIKeysURL(params)}/test-key`, {}, { token });
  return res.json();
}

export async function listAgentAPIKeys(
  params: ListAgentAPIKeysPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentAPIKeyListResponse> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(agentAPIKeysURL(params), { token });
  return res.json();
}

export async function revokeAgentAPIKey(
  params: RevokeAgentAPIKeyPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const keyName = encodeRequired(params.keyName, "keyName");
  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(`${agentAPIKeysURL(params)}/${keyName}`, { token });
  if (!res.ok) throw await res.json();
}
