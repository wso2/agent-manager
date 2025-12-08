

import { GetTraceListPathParams, GetTracePathParams, TraceDetailsResponse, TraceListResponse } from "@agent-management-platform/types";
import { httpGET, OBS_SERVICE_BASE } from "../utils";

export async function getTrace(
    params: GetTracePathParams, 
    getToken?: () => Promise<string>
): Promise<TraceDetailsResponse>{
    const { orgName = "default", projName = "default", agentName, envId, traceId } = params;
    const token = getToken ? await getToken() : undefined;
    // to do: remove logs once api ready
    console.log('getTrace', orgName, projName, envId);
    const searchParams = { traceId , serviceName: agentName };
    const res = await httpGET(
        `${OBS_SERVICE_BASE}/trace`,
        { searchParams, token , options: { useObsPlaneHostApi: true } },
    );

    if (!res.ok) throw await res.json();
    return res.json();
}

export async function getTraceList(
    params: GetTraceListPathParams, 
    getToken?: () => Promise<string>
): Promise<TraceListResponse>{
    const { orgName = "default", projName = "default", agentName, envId, startTime, endTime } = params;
    const token = getToken ? await getToken() : undefined;
    // to do: remove logs once api ready
    console.log('getTraceList', orgName, projName, envId);
    const searchParams = { startTime, endTime, serviceName: agentName };
    const res = await httpGET(
        `${OBS_SERVICE_BASE}/traces`,
        { searchParams, token , options: { useObsPlaneHostApi: true } },
    );
    if (!res.ok) throw await res.json();
    return res.json();
}
