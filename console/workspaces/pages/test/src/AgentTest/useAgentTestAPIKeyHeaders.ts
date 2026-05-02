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
import { useCallback, useRef } from "react";

const DEFAULT_TEST_API_KEY_HEADER = "x-api-key";
const DEFAULT_TEST_API_KEY_TTL_MS = 10 * 60 * 1000;
const EXPIRY_REFRESH_BUFFER_MS = 30 * 1000;

interface CachedTestAPIKey {
  apiKey: string;
  headerName: string;
  expiresAtMs: number;
}

export function useAgentTestAPIKeyHeaders(params: AgentPathParams) {
  const { mutateAsync: createTestAPIKey } = useCreateAgentTestAPIKey();
  const cachedKeyRef = useRef<CachedTestAPIKey | null>(null);

  return useCallback(async (): Promise<Record<string, string>> => {
    if (!params.orgName || !params.projName || !params.agentName) {
      return {};
    }

    const cachedKey = cachedKeyRef.current;
    if (
      cachedKey &&
      cachedKey.expiresAtMs - EXPIRY_REFRESH_BUFFER_MS > Date.now()
    ) {
      return { [cachedKey.headerName]: cachedKey.apiKey };
    }

    const response = await createTestAPIKey(params);
    if (!response.apiKey) {
      throw new Error("Failed to create test API key");
    }

    const headerName = response.headerName || DEFAULT_TEST_API_KEY_HEADER;
    const parsedExpiry = response.expiresAt ? Date.parse(response.expiresAt) : NaN;
    const expiresAtMs = Number.isNaN(parsedExpiry)
      ? Date.now() + DEFAULT_TEST_API_KEY_TTL_MS
      : parsedExpiry;

    cachedKeyRef.current = {
      apiKey: response.apiKey,
      headerName,
      expiresAtMs,
    };

    return { [headerName]: response.apiKey };
  }, [createTestAPIKey, params]);
}
