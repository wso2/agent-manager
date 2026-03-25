/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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
  getTimeRange,
  TraceListResponse,
  TraceListTimeRange,
  GetTraceListPathParams,
  ExportTracesPathParams,
  TraceExportResponse,
} from "@agent-management-platform/types";
import { getTrace, getTraceList, exportTraces } from "../apis/traces";
import { getAgent } from "../apis/agents";
import { listEnvironments } from "../apis/deployments";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiMutation, useApiQuery } from "./react-query-notifications";

// Resolves the componentUid (agent UUID) and environmentUid from the AMP APIs.
// Both are available from the existing /agents and /environments endpoints.
async function resolveUids(
  orgName: string,
  projName: string,
  agentName: string,
  envId: string,
  getToken: () => Promise<string>
): Promise<{ componentUid: string; environmentUid: string }> {
  const [agent, environments] = await Promise.all([
    getAgent({ orgName, projName, agentName }, getToken),
    listEnvironments({ orgName }, getToken),
  ]);

  const componentUid = agent.uuid;
  if (!componentUid) {
    throw new Error(`Agent "${agentName}" does not have a UUID. Ensure it has been deployed.`);
  }

  const env = environments.find((e) => e.name === envId);
  const environmentUid = env?.id;
  if (!environmentUid) {
    throw new Error(`Environment "${envId}" not found or does not have a UUID.`);
  }

  return { componentUid, environmentUid };
}

export function useTraceList(
  orgName?: string,
  projName?: string,
  agentName?: string,
  envId?: string,
  timeRange?: TraceListTimeRange | undefined,
  limit?: number | undefined,
  offset?: number | undefined,
  sortOrder?: GetTraceListPathParams['sortOrder'] | undefined,
  customStartTime?: string,
  customEndTime?: string,
) {
  const { getToken } = useAuthHooks();

  const hasCustomRange = !!customStartTime && !!customEndTime;

  return useApiQuery({
    queryKey: ["trace-list", orgName, projName, agentName, envId, timeRange, limit, offset, sortOrder, customStartTime, customEndTime],
    queryFn: async () => {
      if (!orgName || !projName || !agentName || !envId) {
        throw new Error("Missing required parameters");
      }

      let startTime: string;
      let endTime: string;
      if (hasCustomRange) {
        startTime = customStartTime;
        endTime = customEndTime;
      } else {
        if (!timeRange) {
          throw new Error("Missing required parameters");
        }
        ({ startTime, endTime } = getTimeRange(timeRange));
      }

      const { componentUid, environmentUid } = await resolveUids(orgName, projName, agentName, envId, getToken);

      const res = await getTraceList(
        { componentUid, environmentUid, startTime, endTime, limit, offset, sortOrder },
        getToken
      );
      if (res.totalCount === 0) {
        return { traces: [], totalCount: 0 } as TraceListResponse;
      }
      return res;
    },
    refetchInterval: hasCustomRange ? false : 30000,
    enabled: !!orgName && !!projName && !!agentName && !!envId && (hasCustomRange || !!timeRange),
  });
}

export function useTrace(
  orgName: string,
  projName: string,
  agentName: string,
  envId: string,
  traceId: string
) {
  const { getToken } = useAuthHooks();
  return useApiQuery({
    queryKey: ["trace", orgName, projName, agentName, envId, traceId],
    queryFn: async () => {
      const { componentUid, environmentUid } = await resolveUids(orgName, projName, agentName, envId, getToken);
      return getTrace({ traceId, componentUid, environmentUid }, getToken);
    },
    enabled: !!orgName && !!projName && !!agentName && !!envId && !!traceId,
  });
}

export function useExportTraces() {
  const { getToken } = useAuthHooks();

  return useApiMutation({
    action: { verb: 'create', target: 'trace export' },
    mutationFn: async (params: ExportTracesPathParams): Promise<TraceExportResponse> => {
      const { orgName, projName, agentName, environment, startTime, endTime, limit, offset, sortOrder } = params;

      if (!orgName || !projName || !agentName || !environment) {
        throw new Error("Missing required parameters for export");
      }

      const { componentUid, environmentUid } = await resolveUids(orgName, projName, agentName, environment, getToken);
      return exportTraces({ componentUid, environmentUid, startTime, endTime, limit, offset, sortOrder }, getToken);
    },
  });
}
