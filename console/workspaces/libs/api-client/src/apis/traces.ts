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



import { GetTraceListPathParams, GetTracePathParams, TraceDetailsResponse, TraceListResponse } from "@agent-management-platform/types";
import { httpGET, OBS_SERVICE_BASE } from "../utils";

export async function getTrace(
    params: GetTracePathParams, 
    getToken?: () => Promise<string>
): Promise<TraceDetailsResponse>{
    const { agentName, traceId } = params;
    
    if (!agentName) {
        throw new Error("agentName (serviceName) is required");
    }
    if (!traceId) {
        throw new Error("traceId is required");
    }
    
    const token = getToken ? await getToken() : undefined;
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
    const { agentName, startTime, endTime } = params;
    
    if (!agentName) {
        throw new Error("agentName (serviceName) is required");
    }
    
    const token = getToken ? await getToken() : undefined;
    
    const searchParams = { startTime, endTime, serviceName: agentName };
    const res = await httpGET(
        `${OBS_SERVICE_BASE}/traces`,
        { searchParams, token , options: { useObsPlaneHostApi: true } },
    );
    if (!res.ok) throw await res.json();
    return res.json();
}
