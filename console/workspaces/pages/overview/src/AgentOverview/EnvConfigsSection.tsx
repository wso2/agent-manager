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

import {
    useListAgentMCPConfigs,
    useListAgentModelConfigs,
} from "@agent-management-platform/api-client";
import type {
    AgentModelConfigListResponse,
    ListAgentModelConfigsPathParams,
    ListAgentModelConfigsQuery,
} from "@agent-management-platform/types";
import { buildConfigureTabHref } from "@agent-management-platform/views";
import { buildAddLLMProviderHref } from "./configureTabLink";
import { EnvConfigGroup } from "./EnvConfigGroup";
import { LLMProviderConfigCard } from "./LLMProviderConfigCard";
import { MCPProxyConfigCard } from "./MCPProxyConfigCard";

interface EnvConfigsSectionProps {
    orgId: string;
    projectId: string;
    agentId: string;
    envId: string;
}

const PREVIEW_LIMIT = 3;
// Configs are agent-wide but only some are deployed to any given environment
// (see useEnvFilteredConfigs), so the applicable ones for this environment
// could be anywhere in the full list — an arbitrary first-page cap risks
// missing them entirely if they happen to sort past it. FIRST_PAGE_LIMIT is
// just the initial page size; useAllConfigs below escalates to fetch the
// true total once the first page reveals it, so every config is considered
// and PREVIEW_LIMIT only ever caps what's *displayed*.
const FIRST_PAGE_LIMIT = 10;

type ListConfigsHook = (
    params: ListAgentModelConfigsPathParams,
    query?: ListAgentModelConfigsQuery,
) => { data?: AgentModelConfigListResponse; isError: boolean };

/**
 * Fetches every config (not just the first page) by escalating to a second
 * request sized to the true total once the first page reveals it — at most
 * 2 requests, and only 1 when everything already fit on the first page. The
 * second call is disabled (via the same empty-orgName trick used elsewhere
 * to skip a query without touching the shared hook) whenever it isn't
 * needed.
 */
function useAllConfigs(
    useListConfigs: ListConfigsHook,
    orgId: string,
    projectId: string,
    agentId: string,
) {
    const firstPage = useListConfigs(
        { orgName: orgId, projName: projectId, agentName: agentId },
        { limit: FIRST_PAGE_LIMIT, offset: 0 },
    );
    const total = firstPage.data?.pagination.count ?? 0;
    const needsMore = total > FIRST_PAGE_LIMIT;
    const fullList = useListConfigs(
        { orgName: needsMore ? orgId : "", projName: projectId, agentName: agentId },
        { limit: total, offset: 0 },
    );
    return needsMore ? fullList : firstPage;
}

/**
 * Compact preview of the agent's Model Configs and MCP Proxies, rendered
 * right below the Invoke URL in EnvCapabilitiesSection — just enough to show
 * what's configured for this environment specifically, with a "View all"
 * link to the full list on the Configure Agent page.
 *
 * Each group collapses itself independently (see EnvConfigGroup) while its
 * own list is loading or empty, so LLM Providers can appear before MCP
 * Proxies (or vice versa) instead of both waiting on whichever list is
 * slower.
 */
export const EnvConfigsSection: React.FC<EnvConfigsSectionProps> = ({
    orgId, projectId, agentId, envId,
}) => {
    const { data: modelData, isError: isModelListError } = useAllConfigs(
        useListAgentModelConfigs, orgId, projectId, agentId,
    );
    const { data: mcpData, isError: isMcpListError } = useAllConfigs(
        useListAgentMCPConfigs, orgId, projectId, agentId,
    );

    return (
        <>
            <EnvConfigGroup
                orgId={orgId}
                projectId={projectId}
                agentId={agentId}
                envId={envId}
                configs={modelData?.configs ?? []}
                listError={isModelListError}
                title="LLM Providers"
                viewAllHref={buildConfigureTabHref(orgId, projectId, agentId, "llm")}
                addHref={buildAddLLMProviderHref(orgId, projectId, agentId)}
                addLabel="Configure LLM"
                previewLimit={PREVIEW_LIMIT}
                CardComponent={LLMProviderConfigCard}
            />
            <EnvConfigGroup
                orgId={orgId}
                projectId={projectId}
                agentId={agentId}
                envId={envId}
                configs={mcpData?.configs ?? []}
                listError={isMcpListError}
                title="MCP Servers"
                viewAllHref={buildConfigureTabHref(orgId, projectId, agentId, "tools")}
                addHref={buildConfigureTabHref(orgId, projectId, agentId, "tools")}
                addLabel="Configure MCP"
                previewLimit={PREVIEW_LIMIT}
                CardComponent={MCPProxyConfigCard}
            />
        </>
    );
};
