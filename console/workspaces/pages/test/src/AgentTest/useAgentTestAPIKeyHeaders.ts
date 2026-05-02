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

import { useCreateAgentTestAPIKey } from "@agent-management-platform/api-client";
import type { AgentPathParams } from "@agent-management-platform/types";
import { useCallback } from "react";

const DEFAULT_TEST_API_KEY_HEADER = "x-api-key";
const GATEWAY_API_KEY_SYNC_DELAY_MS = 1500;
const EXPIRY_REFRESH_BUFFER_MS = 30 * 1000;

interface CachedTestAPIKey {
  apiKey: string;
  headerName: string;
  expiresAtMs: number;
}

const cachedKeys = new Map<string, CachedTestAPIKey>();
const pendingKeys = new Map<string, Promise<CachedTestAPIKey>>();

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function cacheKey(params: AgentPathParams): string {
  return `${params.orgName ?? ""}/${params.projName ?? ""}/${params.agentName ?? ""}`;
}

export function invalidateAgentTestAPIKeyHeaders(params: AgentPathParams) {
  cachedKeys.delete(cacheKey(params));
  pendingKeys.delete(cacheKey(params));
}

export function useAgentTestAPIKeyHeaders(
  params: AgentPathParams,
  enabled: boolean,
) {
  const { mutateAsync: createTestAPIKey } = useCreateAgentTestAPIKey();

  return useCallback(async (): Promise<Record<string, string>> => {
    if (!enabled) {
      return {};
    }
    if (!params.orgName || !params.projName || !params.agentName) {
      return {};
    }

    const key = cacheKey(params);
    const cachedKey = cachedKeys.get(key);
    if (
      cachedKey &&
      cachedKey.expiresAtMs - EXPIRY_REFRESH_BUFFER_MS > Date.now()
    ) {
      return { [cachedKey.headerName]: cachedKey.apiKey };
    }

    let pendingKey = pendingKeys.get(key);
    if (!pendingKey) {
      pendingKey = (async () => {
        const response = await createTestAPIKey(params);
        if (!response.apiKey) {
          throw new Error("Failed to create test API key");
        }

        const headerName = response.headerName || DEFAULT_TEST_API_KEY_HEADER;
        const parsedExpiry = response.expiresAt
          ? Date.parse(response.expiresAt)
          : NaN;
        const expiresAtMs = Number.isNaN(parsedExpiry)
          ? Date.now()
          : parsedExpiry;
        const nextKey = {
          apiKey: response.apiKey,
          headerName,
          expiresAtMs,
        };

        cachedKeys.set(key, nextKey);
        await sleep(GATEWAY_API_KEY_SYNC_DELAY_MS);
        return nextKey;
      })().finally(() => {
        pendingKeys.delete(key);
      });
      pendingKeys.set(key, pendingKey);
    }

    const nextKey = await pendingKey;
    return { [nextKey.headerName]: nextKey.apiKey };
  }, [createTestAPIKey, enabled, params]);
}
