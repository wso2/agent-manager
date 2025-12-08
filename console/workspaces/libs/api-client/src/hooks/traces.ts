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
  });
}
