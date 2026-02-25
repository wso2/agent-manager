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
  type EvaluatorListQuery,
  type EvaluatorListResponse,
  type EvaluatorLLMProviderListResponse,
  type EvaluatorResponse,
  type GetEvaluatorPathParams,
  type ListEvaluatorLLMProvidersPathParams,
  type ListEvaluatorsPathParams,
} from "@agent-management-platform/types";
import { httpGET, SERVICE_BASE } from "../utils";

export async function listEvaluators(
  params: ListEvaluatorsPathParams,
  query?: EvaluatorListQuery,
  getToken?: () => Promise<string>
): Promise<EvaluatorListResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = {};
  if (query) {
    if (query.limit !== undefined) {
      searchParams.limit = String(query.limit);
    }
    if (query.offset !== undefined) {
      searchParams.offset = String(query.offset);
    }
    if (query.tags && query.tags.length > 0) {
      searchParams.tags = query.tags.join(",");
    }
    if (query.search) {
      searchParams.search = query.search;
    }
    if (query.provider) {
      searchParams.provider = query.provider;
    }
  }

  const res = await httpGET(`${SERVICE_BASE}/orgs/${org}/evaluators`, {
    searchParams: Object.keys(searchParams).length > 0 ? searchParams : undefined,
    token,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getEvaluator(
  params: GetEvaluatorPathParams,
  getToken?: () => Promise<string>
): Promise<EvaluatorResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const evaluatorId = encodeRequired(params.evaluatorId, "evaluatorId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${org}/evaluators/${evaluatorId}`,
    { token }
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function listEvaluatorLLMProviders(
  params: ListEvaluatorLLMProvidersPathParams,
  getToken?: () => Promise<string>
): Promise<EvaluatorLLMProviderListResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(`${SERVICE_BASE}/orgs/${org}/evaluators/llm-providers`, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

function encodeRequired(value: string | undefined, label: string): string {
  if (!value) {
    throw new Error(`Missing required parameter: ${label}`);
  }
  return encodeURIComponent(value);
}
