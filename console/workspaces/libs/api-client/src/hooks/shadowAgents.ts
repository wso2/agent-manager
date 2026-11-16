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

import { useQueryClient } from "@tanstack/react-query";
import {
  clearShadowScans,
  createShadowConnection,
  createShadowScan,
  deleteShadowConnection,
  deleteShadowFinding,
  deleteShadowFindings,
  getShadowConnection,
  getShadowFinding,
  getShadowScan,
  getShadowSummary,
  listShadowConnections,
  listShadowFindings,
  listShadowProviders,
  listShadowScans,
  testShadowConnection,
  updateShadowConnection,
  updateShadowFinding,
} from "../apis";
import type {
  ShadowAgentSummary,
  ShadowConnectionListResponse,
  ShadowConnectionRequest,
  ShadowConnectionResponse,
  ShadowConnectionTest,
  ShadowFindingDetail,
  ShadowFindingListQuery,
  ShadowFindingListResponse,
  ShadowFindingUpdateRequest,
  ShadowProviderListResponse,
  ShadowFindingClearResponse,
  ShadowScanClearResponse,
  ShadowScanListResponse,
  ShadowScanRequest,
  ShadowScanResponse,
  ShadowScanStatus,
} from "@agent-management-platform/types";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiMutation, useApiQuery } from "./react-query-notifications";

/** How often an active scan is polled. */
export const SHADOW_SCAN_POLL_MS = 3000;

/** Whether a scan is still queued or running. */
export function isShadowScanActive(status: ShadowScanStatus | undefined): boolean {
  return status === "queued" || status === "running";
}

/** Hook to read the platform catalog: forms, permissions and evidence definitions. */
export function useShadowProviders(orgName: string | undefined) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ShadowProviderListResponse>({
    queryKey: ["shadow-agents", "providers", orgName],
    queryFn: () => listShadowProviders(orgName, getToken),
    enabled: !!orgName,
    staleTime: Infinity,
  });
}

/** Hook to read finding counts and the latest scan; polls while a scan runs. */
export function useShadowSummary(orgName: string | undefined) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ShadowAgentSummary>({
    queryKey: ["shadow-agents", "summary", orgName],
    queryFn: () => getShadowSummary(orgName, getToken),
    enabled: !!orgName,
    refetchInterval: (q) =>
      isShadowScanActive(q.state.data?.latestScan?.status) ? SHADOW_SCAN_POLL_MS : false,
  });
}

export function useShadowConnections(orgName: string | undefined) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ShadowConnectionListResponse>({
    queryKey: ["shadow-agents", "connections", orgName],
    queryFn: () => listShadowConnections(orgName, getToken),
    enabled: !!orgName,
  });
}

export function useShadowConnection(orgName: string | undefined, connectionId: string | undefined) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ShadowConnectionResponse>({
    queryKey: ["shadow-agents", "connections", orgName, connectionId],
    queryFn: () => getShadowConnection(orgName, connectionId ?? "", getToken),
    enabled: !!orgName && !!connectionId,
  });
}

/** Hook to list scans; polls while the newest one is active. */
export function useShadowScans(orgName: string | undefined) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ShadowScanListResponse>({
    queryKey: ["shadow-agents", "scans", orgName],
    queryFn: () => listShadowScans(orgName, getToken),
    enabled: !!orgName,
    refetchInterval: (q) =>
      isShadowScanActive(q.state.data?.scans?.[0]?.status) ? SHADOW_SCAN_POLL_MS : false,
  });
}

export function useShadowScan(orgName: string | undefined, scanId: string | undefined) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ShadowScanResponse>({
    queryKey: ["shadow-agents", "scans", orgName, scanId],
    queryFn: () => getShadowScan(orgName, scanId ?? "", getToken),
    enabled: !!orgName && !!scanId,
    refetchInterval: (q) =>
      isShadowScanActive(q.state.data?.status) ? SHADOW_SCAN_POLL_MS : false,
  });
}

export function useShadowFindings(orgName: string | undefined, query: ShadowFindingListQuery) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ShadowFindingListResponse>({
    queryKey: ["shadow-agents", "findings", orgName, query],
    queryFn: () => listShadowFindings(orgName, query, getToken),
    enabled: !!orgName,
  });
}

export function useShadowFinding(orgName: string | undefined, findingId: string | undefined) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ShadowFindingDetail>({
    queryKey: ["shadow-agents", "findings", orgName, "one", findingId],
    queryFn: () => getShadowFinding(orgName, findingId ?? "", getToken),
    enabled: !!orgName && !!findingId,
  });
}

/** Invalidates everything a Shadow Agent Discovery change can affect. */
function useInvalidateShadow() {
  const queryClient = useQueryClient();
  return () => queryClient.invalidateQueries({ queryKey: ["shadow-agents"] });
}

export function useCreateShadowConnection(orgName: string | undefined) {
  const { getToken } = useAuthHooks();
  const invalidate = useInvalidateShadow();
  return useApiMutation<ShadowConnectionResponse, unknown, ShadowConnectionRequest>({
    action: { verb: "create", target: "platform" },
    mutationFn: (body) => createShadowConnection(orgName, body, getToken),
    onSuccess: () => invalidate(),
  });
}

export function useUpdateShadowConnection(
  orgName: string | undefined,
  connectionId: string | undefined,
) {
  const { getToken } = useAuthHooks();
  const invalidate = useInvalidateShadow();
  return useApiMutation<ShadowConnectionResponse, unknown, ShadowConnectionRequest>({
    action: { verb: "update", target: "platform" },
    mutationFn: (body) => updateShadowConnection(orgName, connectionId ?? "", body, getToken),
    onSuccess: () => invalidate(),
  });
}

export function useDeleteShadowConnection(orgName: string | undefined) {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  const invalidate = useInvalidateShadow();
  return useApiMutation<void, unknown, string>({
    action: { verb: "delete", target: "platform" },
    mutationFn: (connectionId) => deleteShadowConnection(orgName, connectionId, getToken),
    onSuccess: (_data, connectionId) => {
      // Drop the row at once: the refetch below can take a moment, and a row
      // still on screen could be deleted again (404).
      queryClient.setQueryData<ShadowConnectionListResponse>(
        ["shadow-agents", "connections", orgName],
        (old) =>
          old && { ...old, connections: old.connections.filter((c) => c.id !== connectionId) },
      );
      return invalidate();
    },
  });
}

export function useTestShadowConnection(orgName: string | undefined) {
  const { getToken } = useAuthHooks();
  const invalidate = useInvalidateShadow();
  return useApiMutation<ShadowConnectionTest, unknown, string>({
    showSuccess: false,
    mutationFn: (connectionId) => testShadowConnection(orgName, connectionId, getToken),
    onSuccess: () => invalidate(),
  });
}

export function useStartShadowScan(orgName: string | undefined) {
  const { getToken } = useAuthHooks();
  const invalidate = useInvalidateShadow();
  return useApiMutation<ShadowScanResponse, unknown, ShadowScanRequest>({
    action: { verb: "start", target: "scan" },
    mutationFn: (body) => createShadowScan(orgName, body, getToken),
    onSuccess: () => invalidate(),
  });
}

/** Deletes the finished scans. Findings are not changed. */
export function useClearShadowScans(orgName: string | undefined) {
  const { getToken } = useAuthHooks();
  const invalidate = useInvalidateShadow();
  return useApiMutation<ShadowScanClearResponse, unknown, void>({
    successMessage: (r) => (r.deleted === 1 ? "1 scan cleared" : `${r.deleted} scans cleared`),
    mutationFn: () => clearShadowScans(orgName, getToken),
    onSuccess: () => invalidate(),
  });
}

export function useUpdateShadowFinding(orgName: string | undefined, findingId: string | undefined) {
  const { getToken } = useAuthHooks();
  const invalidate = useInvalidateShadow();
  return useApiMutation<ShadowFindingDetail, unknown, ShadowFindingUpdateRequest>({
    action: { verb: "update", target: "agent" },
    mutationFn: (body) => updateShadowFinding(orgName, findingId ?? "", body, getToken),
    onSuccess: () => invalidate(),
  });
}

/** Deletes an agent, on every connection that found it. */
export function useDeleteShadowFinding(orgName: string | undefined) {
  const { getToken } = useAuthHooks();
  const invalidate = useInvalidateShadow();
  return useApiMutation<void, unknown, string>({
    action: { verb: "delete", target: "agent" },
    mutationFn: (findingId) => deleteShadowFinding(orgName, findingId, getToken),
    onSuccess: () => invalidate(),
  });
}

/** Deletes every agent of the organization. Platforms and scans are kept. */
export function useDeleteShadowFindings(orgName: string | undefined) {
  const { getToken } = useAuthHooks();
  const invalidate = useInvalidateShadow();
  return useApiMutation<ShadowFindingClearResponse, unknown, void>({
    successMessage: (r) => (r.deleted === 1 ? "1 agent deleted" : `${r.deleted} agents deleted`),
    mutationFn: () => deleteShadowFindings(orgName, getToken),
    onSuccess: () => invalidate(),
  });
}
