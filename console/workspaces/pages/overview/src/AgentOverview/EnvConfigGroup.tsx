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

import { Box, Typography } from "@wso2/oxygen-ui";
import { CollapsibleSection, OverviewSectionCard } from "@agent-management-platform/shared-component";
import type { AgentModelConfigListItem } from "@agent-management-platform/types";
import { useResourceAccess } from "@agent-management-platform/feature-management";
import { AddConfigCard } from "./AddConfigCard";
import { useEnvFilteredConfigs, type ConfigResolution } from "./useEnvFilteredConfigs";

export interface ConfigCardProps {
    orgId: string;
    projectId: string;
    agentId: string;
    envId: string;
    config: AgentModelConfigListItem;
    visible: boolean;
    onResolved: (configId: string, resolution: ConfigResolution) => void;
}

interface EnvConfigGroupProps {
    orgId: string;
    projectId: string;
    agentId: string;
    envId: string;
    configs: AgentModelConfigListItem[];
    /**
     * Whether the config *list* fetch itself failed (distinct from an
     * individual config's own detail fetch failing, tracked via
     * useEnvFilteredConfigs' hasError).
     */
    listError?: boolean;
    title: string;
    viewAllHref: string;
    /** Destination for the trailing "add" tile — the "add config" flow, not the listing. */
    addHref: string;
    /** Tooltip label for the trailing "add" tile, e.g. "Configure LLM" / "Configure MCP". */
    addLabel: string;
    previewLimit: number;
    CardComponent: React.ComponentType<ConfigCardProps>;
}

/**
 * One "LLM Providers" / "MCP Proxies" preview group: probes `configs` for
 * applicability to `envId` via useEnvFilteredConfigs, then renders up to
 * `previewLimit` of them as cards with a "View all" link.
 *
 * Stays collapsed (zero height) while probing and, once settled, only the
 * resolved-applicable configs stay mounted inside — cards that resolved as
 * inapplicable, or ranked past the preview limit, unmount instead of
 * lingering forever, so their per-config queries stop refetching in the
 * background. A group with nothing applicable to this environment simply
 * never expands. A genuine failure (the list itself, or any candidate's own
 * detail fetch) surfaces as an explicit error instead of looking the same as
 * "nothing applies here".
 */
export const EnvConfigGroup: React.FC<EnvConfigGroupProps> = ({
    orgId, projectId, agentId, envId, configs, listError,
    title, viewAllHref, addHref, addLabel, previewLimit, CardComponent,
}) => {
    const {
        visible, reportResolved, isSettled, extraCount, hasError,
    } = useEnvFilteredConfigs(configs, previewLimit, envId);
    const { canWrite } = useResourceAccess("agent");

    if (!listError && configs.length === 0) {
        return null;
    }

    const hasAnyError = listError || hasError;
    const activeConfigs = isSettled ? visible : configs;
    const show = hasAnyError || (isSettled && visible.length > 0);

    return (
        <CollapsibleSection show={show}>
            <OverviewSectionCard title={title} actionHref={viewAllHref} sx={{ mb: 1.5 }}>
                {hasAnyError ? (
                    <Typography variant="body2" color="error">
                        Unable to load {title.toLowerCase()}. Try again later.
                    </Typography>
                ) : (
                    <>
                        <Box display="flex" flexWrap="wrap" gap={1} sx={{ mb: extraCount > 0 ? 0.5 : 0 }}>
                            {activeConfigs.map((config) => (
                                <CardComponent
                                    key={config.uuid}
                                    orgId={orgId}
                                    projectId={projectId}
                                    agentId={agentId}
                                    envId={envId}
                                    config={config}
                                    visible={visible.some((c) => c.uuid === config.uuid)}
                                    onResolved={reportResolved}
                                />
                            ))}
                            {canWrite && <AddConfigCard label={addLabel} href={addHref} />}
                        </Box>
                        {extraCount > 0 && (
                            <Typography variant="caption" color="text.disabled" sx={{ display: "block" }}>
                                +{extraCount} more
                            </Typography>
                        )}
                    </>
                )}
            </OverviewSectionCard>
        </CollapsibleSection>
    );
};
