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

import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import {
  cancelShadowScan,
  clearShadowScans,
  createShadowConnection,
  createShadowRunnerScan,
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
  onboardShadowFinding,
  setShadowFindingHidden,
  testShadowConnection,
  updateShadowConnection,
  uploadShadowScanResults,
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
  ShadowOnboardingRequest,
  ShadowProviderListResponse,
  ShadowFindingClearResponse,
  ShadowRunnerScanRequest,
  ShadowRunnerScanResponse,
  ShadowScanClearResponse,
  ShadowScanListResponse,
  ShadowScanRequest,
  ShadowScanResponse,
  ShadowScanResults,
  ShadowScanResultsUploadResponse,
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

/**
 * Hook to list scans; polls while one is active. A newer scan waiting for
 * results can sit above the running one, so every scan is checked.
 */
export function useShadowScans(orgName: string | undefined) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ShadowScanListResponse>({
    queryKey: ["shadow-agents", "scans", orgName],
    queryFn: () => listShadowScans(orgName, getToken),
    enabled: !!orgName,
    refetchInterval: (q) =>
      q.state.data?.scans?.some((s) => isShadowScanActive(s.status)) ? SHADOW_SCAN_POLL_MS : false,
  });
}

/** Hook to read one scan; polls while it is queued or running, for its progress. */
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

/**
 * Refreshes the agents lists and the summary. A running scan's page calls it as
 * each connector finishes, since only the scan itself is polled.
 */
export function useRefreshShadowFindings(orgName: string | undefined) {
  const queryClient = useQueryClient();
  return useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: ["shadow-agents", "findings", orgName] });
    void queryClient.invalidateQueries({ queryKey: ["shadow-agents", "summary", orgName] });
  }, [queryClient, orgName]);
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

/**
 * Generate command: creates a pending source code scan for a GitHub or Azure
 * DevOps connection. The response holds a scan token, so callers keep it in
 * component state only (it is never put in the query cache).
 */
export function useCreateShadowRunnerScan(orgName: string | undefined) {
  const { getToken } = useAuthHooks();
  const invalidate = useInvalidateShadow();
  return useApiMutation<
    ShadowRunnerScanResponse,
    unknown,
    { connectionId: string } & ShadowRunnerScanRequest
  >({
    // The panel or dialog shows the command and the error itself.
    showSuccess: false,
    showError: false,
    // The response holds a scan token: drop it from the mutation cache at once.
    gcTime: 0,
    mutationFn: ({ connectionId, ...body }) =>
      createShadowRunnerScan(orgName, connectionId, body, getToken),
    // The new pending scan shows in Scans.
    onSuccess: () => invalidate(),
  });
}

/**
 * Uploads a results file's results to the scan waiting for them. The dialog
 * shows errors itself; the success notice names the agents found.
 */
export function useUploadShadowScanResults(orgName: string | undefined) {
  const { getToken } = useAuthHooks();
  const invalidate = useInvalidateShadow();
  return useApiMutation<
    ShadowScanResultsUploadResponse,
    unknown,
    { scanId: string; results: ShadowScanResults }
  >({
    showError: false,
    successMessage: (r) =>
      `Results uploaded: ${r.findingsCount} ${r.findingsCount === 1 ? "agent" : "agents"} found`,
    // The variables hold the whole results file: drop them from the cache at once.
    gcTime: 0,
    mutationFn: ({ scanId, results }) =>
      uploadShadowScanResults(orgName, scanId, results, getToken),
    // The scan, its agents and the summary change; a 409 means the scan
    // changed meanwhile: refresh it either way.
    onSettled: () => invalidate(),
  });
}

/**
 * Cancels a scan that is waiting for results, queued or running. The agents
 * already found stay; unfinished connections are not scanned.
 */
export function useCancelShadowScan(orgName: string | undefined) {
  const { getToken } = useAuthHooks();
  const invalidate = useInvalidateShadow();
  return useApiMutation<void, unknown, string>({
    successMessage: () => "Scan cancelled",
    errorMessage: (e) =>
      (e as { status?: number })?.status === 409
        ? "The scan has already finished."
        : "The scan could not be cancelled. Try again.",
    mutationFn: (scanId) => cancelShadowScan(orgName, scanId, getToken),
    // Refreshes the summary, the scans and the scan detail. A 409 means the
    // scan finished meanwhile: refresh it either way.
    onSettled: () => invalidate(),
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

/**
 * Links the agent to the agent registered or created for it. `silent` leaves
 * the success and error notices to the caller.
 */
export function useOnboardShadowFinding(
  orgName: string | undefined,
  findingId: string | undefined,
  { silent = false }: { silent?: boolean } = {},
) {
  const { getToken } = useAuthHooks();
  const invalidate = useInvalidateShadow();
  return useApiMutation<ShadowFindingDetail, unknown, ShadowOnboardingRequest>({
    showSuccess: !silent,
    showError: !silent,
    successMessage: () => "Agent onboarded",
    mutationFn: (body) => onboardShadowFinding(orgName, findingId ?? "", body, getToken),
    onSuccess: () => invalidate(),
  });
}

/**
 * Removes an onboarded agent from the Unregistered Agents list (true), or
 * puts it back (false). Later scans keep the choice.
 */
export function useSetShadowFindingHidden(
  orgName: string | undefined,
  findingId: string | undefined,
) {
  const { getToken } = useAuthHooks();
  const invalidate = useInvalidateShadow();
  return useApiMutation<ShadowFindingDetail, unknown, boolean>({
    successMessage: (_data, hidden) =>
      hidden ? "Removed from Unregistered Agents" : "Shown in Unregistered Agents again",
    mutationFn: (hidden) => setShadowFindingHidden(orgName, findingId ?? "", { hidden }, getToken),
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
