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
  A2AProxyPathParams,
  A2AProxyResponse,
  SetA2AProxyRequest,
} from "@agent-management-platform/types";
import { httpDELETE, httpGET, httpPOST, httpPUT } from "../utils";
import { agentEnvironmentResourcePath } from "./agent-card";

// A direct environment answers 404, which the http helpers throw with status 404.
export async function getA2AProxy(
  params: A2AProxyPathParams,
  getToken?: () => Promise<string>,
): Promise<A2AProxyResponse> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(agentEnvironmentResourcePath(params, "a2a-proxy"), { token });
  return res.json();
}

export async function setA2AProxy(
  params: A2AProxyPathParams,
  body: SetA2AProxyRequest,
  getToken?: () => Promise<string>,
): Promise<void> {
  const token = getToken ? await getToken() : undefined;
  await httpPUT(agentEnvironmentResourcePath(params, "a2a-proxy"), body, { token });
}

export async function deleteA2AProxy(
  params: A2AProxyPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const token = getToken ? await getToken() : undefined;
  await httpDELETE(agentEnvironmentResourcePath(params, "a2a-proxy"), { token });
}

export async function refreshA2AProxy(
  params: A2AProxyPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const token = getToken ? await getToken() : undefined;
  await httpPOST(`${agentEnvironmentResourcePath(params, "a2a-proxy")}/refresh`, {}, { token });
}
