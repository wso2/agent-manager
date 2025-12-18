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

import { useQuery } from "@tanstack/react-query";
import { getTimeRange, TraceListTimeRange } from "@agent-management-platform/types";
import { getTrace, getTraceList } from "../apis/traces";
import { useAuthHooks } from "@agent-management-platform/auth";

export function useTraceList(
    orgName: string, 
    projName: string,
     agentName: string,
      envId: string, 
      timeRange: TraceListTimeRange
) {
    const { getToken } = useAuthHooks();

  return useQuery({
    queryKey: ['trace-list', orgName, projName, agentName, envId, timeRange],
    queryFn: async () => {
        const { startTime, endTime } = getTimeRange(timeRange);
        try {
        const res = await getTraceList({
            orgName,
            projName,
            agentName,
            envId,
            startTime,
            endTime,
        }, getToken);
        if (res.totalCount === 0) {
            return [];
        }
        return res;
        } catch (error) {
            console.error(error);
            return [];
        }
    },
    enabled: !!orgName && !!projName && !!agentName && !!envId,
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
  return useQuery({
    queryKey: ['trace', orgName, projName, agentName, envId, traceId],
    queryFn: async () => {
        try {
        const res = await getTrace({
            orgName,
            projName,
            agentName,
            envId,
            traceId,
        }, getToken);
        return res;
        } catch (error) {
            console.error(error);
            return null;
        }
    },
    enabled: !!orgName && !!projName && !!agentName && !!envId && !!traceId,
  });
}
