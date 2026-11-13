/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
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

import { useAuthHooks } from "@agent-management-platform/auth";
import { globalConfig } from "@agent-management-platform/types";
import { listAvailableMCPPolicies } from "../apis";
import { useApiQuery } from "./react-query-notifications";

export interface MCPPolicyDefinition {
  name: string;
  version: string;
  policyHubVersion?: string;
  displayName: string;
  description: string;
  provider: string;
  categories: string[];
  isLatest: boolean;
}

export interface MCPPoliciesCatalogResponse {
  count: number;
  data: MCPPolicyDefinition[];
}

function getMCPPoliciesCatalogUrl(): string {
  const url = globalConfig.guardrailsCatalogUrl;

  if (!url) return "";

  try {
    const mcpPoliciesUrl = new URL(url);
    mcpPoliciesUrl.searchParams.set("categories", "MCP");
    if (!mcpPoliciesUrl.searchParams.has("limit")) {
      mcpPoliciesUrl.searchParams.set("limit", "50");
    }
    return mcpPoliciesUrl.toString();
  } catch {
    return "";
  }
}

export function useMCPPoliciesCatalog(orgName?: string) {
  const url = getMCPPoliciesCatalogUrl();
  const { getToken } = useAuthHooks();

  return useApiQuery<MCPPoliciesCatalogResponse>({
    queryKey: ["MCP policies catalog", url, orgName],
    enabled: Boolean(url && orgName),
    queryFn: async () => {
      if (!url) {
        throw new Error("MCP policies catalog URL is not configured.");
      }
      if (!orgName) {
        throw new Error("Organization name is required to list MCP policies.");
      }

      const token = await getToken();
      const available = await listAvailableMCPPolicies(
        { orgName },
        async () => token,
      );
      const gatewayVersionByPolicyMajor = new Map<string, string>();
      for (const policy of available.list ?? []) {
        const key = `${policy.name}:${normalizePolicyVersionToMajor(policy.version)}`;
        const existing = gatewayVersionByPolicyMajor.get(key);
        if (!existing || compareMinorVersions(policy.version, existing) > 0) {
          gatewayVersionByPolicyMajor.set(key, policy.version);
        }
      }
      if (gatewayVersionByPolicyMajor.size === 0) {
        return { count: 0, data: [] };
      }

      const res = await fetch(url, {
        headers: token
          ? { Authorization: `Bearer ${token}` }
          : undefined,
      });
      if (!res.ok) {
        const text = await res.text().catch(() => "");
        throw new Error(
          text || `Failed to fetch MCP policies catalog: ${res.status}`,
        );
      }
      const catalog = (await res.json()) as MCPPoliciesCatalogResponse;
      const data = (catalog.data ?? []).flatMap((policy) => {
        const gatewayVersion = gatewayVersionByPolicyMajor.get(
          `${policy.name}:${normalizePolicyVersionToMajor(policy.version)}`,
        );
        if (!gatewayVersion) return [];
        return [
          {
            ...policy,
            version: normalizePolicyVersionToMajor(gatewayVersion),
            policyHubVersion: normalizePolicyVersionToMinor(gatewayVersion),
          },
        ];
      });
      return {
        ...catalog,
        count: data.length,
        data,
      };
    },
  });
}

export function useMCPPolicyDefinition(
  name: string | undefined,
  version: string | undefined,
) {
  const baseUrl = globalConfig.guardrailsDefinitionBaseUrl;
  const { getToken } = useAuthHooks();
  const enabled = Boolean(baseUrl && name && version);

  return useApiQuery<string>({
    queryKey: ["MCP policy definition", baseUrl, name, version],
    enabled,
    queryFn: async () => {
      if (!baseUrl || !name || !version) {
        throw new Error(
          "MCP policy definition base URL, policy name, and version are required.",
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
        const text = await res.text().catch(() => "");
        throw new Error(
          text || `Failed to fetch MCP policy definition: ${res.status}`,
        );
      }
      return res.text();
    },
  });
}

export function normalizePolicyVersionToPolicyHubMinor(version: string): string {
  return normalizePolicyVersionToMinor(version);
}

function normalizePolicyVersionToMinor(version: string): string {
  const trimmedVersion = version.trim();
  if (!trimmedVersion) return trimmedVersion;

  let versionWithoutPrefix = trimmedVersion;
  if (versionWithoutPrefix.toLowerCase().startsWith("v")) {
    versionWithoutPrefix = versionWithoutPrefix.slice(1);
  }
  if (!versionWithoutPrefix) return trimmedVersion;

  const parts = versionWithoutPrefix.split(".");
  if (parts.length === 1) {
    return /^\d+$/.test(parts[0]) ? `${parts[0]}.0` : trimmedVersion;
  }
  const major = parts[0].trim();
  const minor = parts[1].trim();
  if (!/^\d+$/.test(major) || !/^\d+$/.test(minor)) {
    return trimmedVersion;
  }
  return `${major}.${minor}`;
}

function compareMinorVersions(a: string, b: string): number {
  const minorOf = (version: string) =>
    Number(normalizePolicyVersionToMinor(version).split(".")[1] ?? -1);
  const diff = minorOf(a) - minorOf(b);
  return Number.isNaN(diff) ? 0 : diff;
}

function normalizePolicyVersionToMajor(version: string): string {
  const trimmedVersion = version.trim();
  if (!trimmedVersion) return trimmedVersion;

  let versionWithoutPrefix = trimmedVersion;
  if (versionWithoutPrefix.toLowerCase().startsWith("v")) {
    versionWithoutPrefix = versionWithoutPrefix.slice(1);
  }
  if (!versionWithoutPrefix) return trimmedVersion;

  let majorVersion = versionWithoutPrefix;
  const dotIndex = majorVersion.indexOf(".");
  if (dotIndex >= 0) majorVersion = majorVersion.slice(0, dotIndex);
  const dashIndex = majorVersion.indexOf("-");
  if (dashIndex >= 0) majorVersion = majorVersion.slice(0, dashIndex);
  majorVersion = majorVersion.trim();
  if (!/^\d+$/.test(majorVersion)) return trimmedVersion;
  return `v${majorVersion}`;
}
