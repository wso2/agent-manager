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
  AgentAPIKeyListResponse,
  CreateAgentAPIKeyPathParams,
  CreateAgentAPIKeyRequest,
  CreateAgentAPIKeyResponse,
  IssueTestAgentAPIKeyPathParams,
  IssueTestAgentAPIKeyResponse,
  ListAgentAPIKeysPathParams,
  RevokeAgentAPIKeyPathParams,
  RotateAgentAPIKeyPathParams,
  RotateAgentAPIKeyRequest,
  RotateAgentAPIKeyResponse,
} from "@agent-management-platform/types";
import { encodeRequired, httpDELETE, httpGET, httpPOST, httpPUT, SERVICE_BASE } from "../utils";

export async function createAgentAPIKey(
  params: CreateAgentAPIKeyPathParams,
  body: CreateAgentAPIKeyRequest,
  getToken?: () => Promise<string>,
): Promise<CreateAgentAPIKeyResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const proj = encodeRequired(params.projName, "projName");
  const agent = encodeRequired(params.agentName, "agentName");
  const env = encodeRequired(params.envId, "envId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${org}/projects/${proj}/agents/${agent}/environments/${env}/api-keys`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function listAgentAPIKeys(
  params: ListAgentAPIKeysPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentAPIKeyListResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const proj = encodeRequired(params.projName, "projName");
  const agent = encodeRequired(params.agentName, "agentName");
  const env = encodeRequired(params.envId, "envId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${org}/projects/${proj}/agents/${agent}/environments/${env}/api-keys`,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function rotateAgentAPIKey(
  params: RotateAgentAPIKeyPathParams,
  body: RotateAgentAPIKeyRequest,
  getToken?: () => Promise<string>,
): Promise<RotateAgentAPIKeyResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const proj = encodeRequired(params.projName, "projName");
  const agent = encodeRequired(params.agentName, "agentName");
  const env = encodeRequired(params.envId, "envId");
  const keyName = encodeRequired(params.keyName, "keyName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPUT(
    `${SERVICE_BASE}/orgs/${org}/projects/${proj}/agents/${agent}/environments/${env}/api-keys/${keyName}`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function revokeAgentAPIKey(
  params: RevokeAgentAPIKeyPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const org = encodeRequired(params.orgName, "orgName");
  const proj = encodeRequired(params.projName, "projName");
  const agent = encodeRequired(params.agentName, "agentName");
  const env = encodeRequired(params.envId, "envId");
  const keyName = encodeRequired(params.keyName, "keyName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpDELETE(
    `${SERVICE_BASE}/orgs/${org}/projects/${proj}/agents/${agent}/environments/${env}/api-keys/${keyName}`,
    { token },
  );
  if (!res.ok) throw await res.json();
}

export async function issueTestAgentAPIKey(
  params: IssueTestAgentAPIKeyPathParams,
  getToken?: () => Promise<string>,
): Promise<IssueTestAgentAPIKeyResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const proj = encodeRequired(params.projName, "projName");
  const agent = encodeRequired(params.agentName, "agentName");
  const env = encodeRequired(params.envId, "envId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${org}/projects/${proj}/agents/${agent}/environments/${env}/api-keys/test`,
    {},
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}
