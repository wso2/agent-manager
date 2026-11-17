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

import { generatePath } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";

const configureRoutes =
    absoluteRouteMap.children.org.children.projects.children.agents.children.configure.children;

/**
 * Deep-links to a single LLM provider config's view page — the same route the
 * Configure Agent page's LLM Providers list navigates to on row click. The id
 * is URL-encoded to mirror Configure.Component.tsx's own navigation.
 */
export function buildLLMProviderViewHref(
    orgId: string,
    projectId: string,
    agentId: string,
    configId: string,
): string {
    return generatePath(configureRoutes.llmProviders.children.view.path, {
        orgId, projectId, agentId, configId: encodeURIComponent(configId),
    });
}

/**
 * Deep-links to a single MCP server config's view page. Note the route param
 * is `proxyId` (not `configId`), matching ViewMCPServer.Component.tsx.
 */
export function buildMCPProxyViewHref(
    orgId: string,
    projectId: string,
    agentId: string,
    proxyId: string,
): string {
    return generatePath(configureRoutes.mcpProxies.children.view.path, {
        orgId, projectId, agentId, proxyId: encodeURIComponent(proxyId),
    });
}

/**
 * Deep-links to the "add LLM provider" flow on the Configure Agent page. (MCP
 * has no equivalent dedicated add route — its "add" entry point is just the
 * tools tab, so use buildConfigureTabHref(..., "tools") for that.)
 */
export function buildAddLLMProviderHref(
    orgId: string,
    projectId: string,
    agentId: string,
): string {
    return generatePath(configureRoutes.llmProviders.children.add.path, {
        orgId, projectId, agentId,
    });
}
