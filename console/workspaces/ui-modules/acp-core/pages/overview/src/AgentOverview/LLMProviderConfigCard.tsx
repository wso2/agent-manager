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
    useGetAgentModelConfig,
    useLLMPoliciesCatalog,
    useListCatalogLLMProviders,
    useListLLMProviderTemplates,
} from "@agent-management-platform/api-client";
import type { AgentModelConfigListItem } from "@agent-management-platform/types";
import { buildLLMProviderViewHref } from "./configureTabLink";
import { ConfigListCard } from "./ConfigListCard";
import { getAvatarInitials } from "@agent-management-platform/views";
import { getProviderAvatarColor } from "./providerAvatar";
import { useConfigEnvMapping } from "./useConfigEnvMapping";
import type { ConfigResolution } from "./useEnvFilteredConfigs";

interface LLMProviderConfigCardProps {
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

/**
 * One "LLM Providers" preview row. The list endpoint only returns name/id, so
 * this card does its own per-config fetch for the guardrails selected on this
 * config's environment mapping (the same list editable on Configure Agent →
 * LLM Providers → this config → Guardrails), plus the org's policy catalog to
 * resolve display names — mirroring ViewLLMProvider.Component's own lookup.
 * Configs not mapped to `envId` at all report themselves as inapplicable and
 * render nothing — never falls back to showing another environment's data.
 */
export const LLMProviderConfigCard: React.FC<LLMProviderConfigCardProps> = ({
    orgId, projectId, agentId, envId, config, visible, onResolved,
}) => {
    const {
        data: fullConfig, isLoading: isLoadingConfig, isError: isConfigError,
    } = useGetAgentModelConfig({
        orgName: orgId,
        projName: projectId,
        agentName: agentId,
        configId: config.uuid,
    });
    const { data: catalog, isLoading: isLoadingCatalog } = useLLMPoliciesCatalog(orgId);

    // Same catalog + template lookup ViewLLMProvider.Component.tsx uses to
    // render this config's provider logo: providerName is the catalog entry's
    // handle, whose `template` id resolves to the template's metadata.logoUrl.
    const { data: providerCatalog } = useListCatalogLLMProviders(
        { orgName: orgId },
        { limit: 50 },
    );
    const { data: templatesData } = useListLLMProviderTemplates({ orgName: orgId });

    const displayNameByPolicy = useMemo(() => {
        const map = new Map<string, string>();
        for (const policy of catalog?.data ?? []) {
            if (policy.displayName) map.set(policy.name, policy.displayName);
        }
        return map;
    }, [catalog]);

    const envMapping = useConfigEnvMapping(
        fullConfig?.envMappings, isLoadingConfig, isConfigError, envId, config.uuid, onResolved,
    );

    const guardrailNames = useMemo(() => {
        // Org-level guardrails (configured on the provider itself) apply in
        // addition to whatever this agent's config overrides on top of them.
        const policies = [
            ...(envMapping?.configuration?.providerPolicies ?? []),
            ...(envMapping?.configuration?.policies ?? []),
        ];
        const seen = new Set<string>();
        const names: string[] = [];
        for (const policy of policies) {
            const label = displayNameByPolicy.get(policy.name) ?? policy.name;
            if (seen.has(label)) continue;
            seen.add(label);
            names.push(label);
        }
        return names;
    }, [envMapping, displayNameByPolicy]);

    const isLoading = isLoadingConfig || isLoadingCatalog;
    const subtitle = guardrailNames.length > 0
        ? `Guardrails: ${guardrailNames.join(", ")}`
        : "No guardrails configured";

    const providerName = envMapping?.configuration?.providerName;

    const template = useMemo(() => {
        const catalogEntry = providerCatalog?.entries.find((e) => e.handle === providerName);
        return templatesData?.templates.find((t) => t.id === catalogEntry?.template);
    }, [providerCatalog, templatesData, providerName]);

    if (!visible) {
        return null;
    }

    return (
        <ConfigListCard
            avatarLabel={getAvatarInitials(config.name, { maxChars: 1, fallback: "?" })}
            avatarColor={getProviderAvatarColor(providerName ?? config.name)}
            avatarSrc={template?.metadata?.logoUrl}
            title={config.name}
            providerLabel={template?.name}
            subtitle={subtitle}
            isLoadingSubtitle={isLoading}
            href={buildLLMProviderViewHref(orgId, projectId, agentId, config.uuid)}
        />
    );
};
