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

import { useMemo } from "react";
import { useGetAgentEndpoints } from "@agent-management-platform/api-client";
import {
    extractOpenApiResources,
    parseOpenApiSpecContent,
    type OpenApiResource,
} from "@agent-management-platform/shared-component";

interface UseAgentEndpointResourcesParams {
    orgId: string;
    projectId: string;
    agentId: string;
    envId: string;
    external?: boolean;
}

/**
 * Resolves an agent environment's deployed endpoints into what both the
 * "API Endpoint" and "Agent Interface" cards need: the invoke URL, the
 * deduped list of HTTP resources parsed from each endpoint's OpenAPI schema,
 * and the fetch's own error state so a failed request isn't silently treated
 * as "no data yet". Shared so the two cards can be split across different
 * parts of the page layout without duplicating the fetch/parse logic. Not
 * applicable to external agents (they aren't deployed through this platform,
 * so there's nothing to fetch), so `external` withholds `orgName` to keep
 * useGetAgentEndpoints disabled instead of firing a request that would just
 * be discarded.
 */
export function useAgentEndpointResources({
    orgId, projectId, agentId, envId, external,
}: UseAgentEndpointResourcesParams) {
    const { data: endpoints, isLoading, isError } = useGetAgentEndpoints(
        { orgName: external ? "" : orgId, projName: projectId, agentName: agentId },
        { environment: envId },
    );

    // Single pass over the endpoint map: flattens every endpoint's OpenAPI
    // schema into a deduped method+path list, and separately picks the
    // externally-reachable endpoint as "the" invoke URL, falling back to
    // whichever entry is present when none is marked external.
    const { resources, invokeUrl } = useMemo(() => {
        const endpointList = Object.values(endpoints ?? {});
        const byKey = new Map<string, OpenApiResource>();
        endpointList.forEach((endpoint) => {
            const spec = parseOpenApiSpecContent(endpoint.schema?.content);
            extractOpenApiResources(spec).forEach((resource) => {
                byKey.set(`${resource.method} ${resource.path}`, resource);
            });
        });
        const externalEndpoint = endpointList.find(
            (endpoint) => endpoint.visibility?.toLowerCase() === "external",
        );
        return {
            resources: Array.from(byKey.values()),
            invokeUrl: (externalEndpoint ?? endpointList[0])?.url,
        };
    }, [endpoints]);

    return { resources, invokeUrl, isLoading, isError };
}
