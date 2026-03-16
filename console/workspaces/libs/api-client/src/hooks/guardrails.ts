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

import { useQuery } from "@tanstack/react-query";
import { useAuthHooks } from "@agent-management-platform/auth";
import { globalConfig } from "@agent-management-platform/types";

export interface GuardrailDefinition {
  name: string;
  version: string;
  displayName: string;
  description: string;
  provider: string;
  categories: string[];
  isLatest: boolean;
}

export interface GuardrailsCatalogResponse {
  count: number;
  data: GuardrailDefinition[];
}

export function useGuardrailsCatalog() {
  const url = globalConfig.guardrailsCatalogUrl;
  const { getToken } = useAuthHooks();

  return useQuery<GuardrailsCatalogResponse>({
    queryKey: ["guardrails-catalog", url],
    enabled: Boolean(url),
    queryFn: async () => {
      if (!url) {
        throw new Error("Guardrails catalog URL is not configured.");
      }

      const token = await getToken();
      const res = await fetch(url, {
        headers: token
          ? { Authorization: `Bearer ${token}` }
          : undefined,
      });
      if (!res.ok) {
        const text = await res.text().catch(() => "");
        throw new Error(
          text || `Failed to fetch guardrails catalog: ${res.status}`,
        );
      }
      return (await res.json()) as GuardrailsCatalogResponse;
    },
  });
}

/**
 * Fetches a single guardrail policy definition (YAML) by name and version.
 *
 * The definition endpoint returns YAML content which should be
 * parsed by the consumer (e.g. with `parsePolicyYaml`).
 *
 * URL pattern:
 * `{guardrailsDefinitionBaseUrl}/{name}/versions/{version}/definition`
 */
export function useGuardrailPolicyDefinition(
  name: string | undefined,
  version: string | undefined,
) {
  const baseUrl = globalConfig.guardrailsDefinitionBaseUrl;
  const { getToken } = useAuthHooks();
  const enabled = Boolean(baseUrl && name && version);

  return useQuery<string>({
    queryKey: [
      "guardrail-policy-definition", baseUrl, name, version,
    ],
    enabled,
    queryFn: async () => {
      if (!baseUrl || !name || !version) {
        throw new Error(
          "Guardrails definition base URL, policy name,"
          + " and version are required.",
        );
      }

      const token = await getToken();
      const url =
        `${baseUrl}/${encodeURIComponent(name)}`
        + `/versions/${encodeURIComponent(version)}`
        + `/definition`;
      const res = await fetch(url, {
        headers: token
          ? { Authorization: `Bearer ${token}` }
          : undefined,
      });
      if (!res.ok) {
        const errText = await res.text().catch(() => "");
        throw new Error(
          errText
          || `Failed to fetch policy definition: ${res.status}`,
        );
      }
      return res.text();
    },
  });
}

