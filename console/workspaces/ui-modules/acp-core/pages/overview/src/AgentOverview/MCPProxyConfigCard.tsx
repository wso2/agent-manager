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
import {
    useGetAgentMCPConfig,
    useGetMCPProxy,
    useListEnvironments,
} from "@agent-management-platform/api-client";
import type {
    AgentModelConfigListItem,
    ProviderConfig,
} from "@agent-management-platform/types";
import { buildMCPProxyViewHref } from "./configureTabLink";
import { ConfigListCard } from "./ConfigListCard";
import { getAvatarInitials } from "@agent-management-platform/views";
import { getProviderAvatarColor } from "./providerAvatar";
import { useConfigEnvMapping } from "./useConfigEnvMapping";
import type { ConfigResolution } from "./useEnvFilteredConfigs";

interface MCPProxyConfigCardProps {
    orgId: string;
    projectId: string;
    agentId: string;
    envId: string;
    config: AgentModelConfigListItem;
    /** Whether the parent has room to show this config (only true once it's
     * confirmed applicable to envId and ranks within the preview limit). */
    visible: boolean;
    /** Reports whether this config is actually deployed to envId, once known. */
    onResolved: (configId: string, resolution: ConfigResolution) => void;
}

// Mirrors ViewMCPServer.Component.tsx's private getMCPProxyName — the config's
// env mapping only ever populates one of these depending on when it was saved.
function getMCPProxyName(config?: ProviderConfig): string | undefined {
    return config?.proxyName
        ?? config?.proxyId
        ?? config?.mcpProxyName
        ?? config?.mcpProxyId
        ?? config?.providerName;
}

// Tool capability entries are Record<string, unknown> — this mirrors
// getCapabilityId("tool", raw) from @agent-management-platform/mcp-proxies
// (kept local instead of adding that page as a cross-package dependency for
// one string-extraction helper).
function getToolName(raw: Record<string, unknown>): string | undefined {
    const value = raw.name;
    return typeof value === "string" && value.trim() ? value.trim() : undefined;
}

/**
 * One "MCP Proxies" preview row. The agent config only references a proxy by
 * name/id, so this card fetches the org-level MCPProxy (env-bound endpoint,
 * matched the same way ViewMCPServer.Component.tsx does via environmentUuid)
 * to list the tools that endpoint exposes. Configs not mapped to `envId` at
 * all report themselves as inapplicable and render nothing — never falls
 * back to showing another environment's tools.
 */
export const MCPProxyConfigCard: React.FC<MCPProxyConfigCardProps> = ({
    orgId, projectId, agentId, envId, config, visible, onResolved,
}) => {
    const {
        data: fullConfig, isLoading: isLoadingConfig, isError: isConfigError,
    } = useGetAgentMCPConfig({
        orgName: orgId,
        projName: projectId,
        agentName: agentId,
        configId: config.uuid,
    });

    const envMapping = useConfigEnvMapping(
        fullConfig?.envMappings, isLoadingConfig, isConfigError, envId, config.uuid, onResolved,
    );
    const proxyName = getMCPProxyName(envMapping?.configuration);

    const {
        data: environments, isLoading: isLoadingEnvironments, isError: isEnvironmentsError,
    } = useListEnvironments({ orgName: orgId });
    const {
        data: proxy, isLoading: isLoadingProxy, isError: isProxyError,
    } = useGetMCPProxy({
        orgName: orgId,
        proxyId: proxyName ?? "",
    });

    const envUuid = environments?.find((env) => env.name === envId)?.id;
    const endpoint = proxy?.endpoints?.find((ep) =>
        ep.environments?.some((binding) => binding.environmentUuid === envUuid),
    );

    const toolNames = useMemo(() => {
        const seen = new Set<string>();
        const names: string[] = [];
        for (const raw of endpoint?.capabilities?.tools ?? []) {
            const name = getToolName(raw);
            if (!name || seen.has(name)) continue;
            seen.add(name);
            names.push(name);
        }
        return names;
    }, [endpoint]);

    // "No tools exposed" is only a meaningful answer once config, environments,
    // and proxy have all actually settled — otherwise an in-flight environments
    // lookup (envUuid still undefined) would make the endpoint match fail and
    // read as "no tools" before we've actually found the right one.
    const isLoading = isLoadingConfig || isLoadingProxy || isLoadingEnvironments;
    const hasLoadError = isConfigError || isProxyError || isEnvironmentsError;
    const subtitle = hasLoadError
        ? "Unable to load tools"
        : toolNames.length > 0
            ? `Tools: ${toolNames.join(", ")}`
            : "No tools exposed";

    if (!visible) {
        return null;
    }

    return (
        <ConfigListCard
            avatarLabel={getAvatarInitials(config.name, { maxChars: 1, fallback: "?" })}
            avatarColor={getProviderAvatarColor(proxyName ?? config.name)}
            title={config.name}
            providerLabel={proxy?.name}
            subtitle={subtitle}
            isLoadingSubtitle={isLoading}
            href={buildMCPProxyViewHref(orgId, projectId, agentId, config.uuid)}
        />
    );
};
