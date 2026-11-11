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

import { Box, Chip, Typography, type ChipProps } from "@wso2/oxygen-ui";
import { generatePath } from "react-router-dom";
import {
    CollapsibleSection,
    DeploymentStatus,
    OverviewSectionCard,
} from "@agent-management-platform/shared-component";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { useAgentEndpointResources } from "./useAgentEndpointResources";

interface EnvAgentInterfaceCardProps {
    orgId: string;
    projectId: string;
    agentId: string;
    envId: string;
    external?: boolean;
    deploymentStatus?: DeploymentStatus;
}

const METHOD_LABEL: Record<string, string> = {
    DELETE: "DEL",
};

const METHOD_COLOR: Record<string, ChipProps["color"]> = {
    GET: "success",
    POST: "warning",
    PUT: "info",
    PATCH: "info",
    DELETE: "error",
};

/**
 * The HTTP resources an agent exposes, parsed from each deployed endpoint's
 * published OpenAPI schema. Rendered next to the "Agent ID" card so the two
 * per-environment identity/interface summaries sit in one row. Links out to
 * the full API Spec viewer on the Try It page.
 */
export const EnvAgentInterfaceCard: React.FC<EnvAgentInterfaceCardProps> = ({
    orgId, projectId, agentId, envId, external, deploymentStatus,
}) => {
    const { resources, invokeUrl, isLoading, isError } = useAgentEndpointResources({
        orgId, projectId, agentId, envId, external,
    });

    if (external) {
        return null;
    }

    const tryItHref = generatePath(
        absoluteRouteMap.children.org.children.projects.children.agents
            .children.environment.children.tryOut.path,
        { orgId, projectId, agentId, envId },
    );

    // Mirrors EnvCapabilitiesSection's own gating: only worth showing once the
    // environment is actually deployed and there's something to point at. A
    // failed fetch is shown regardless, so the failure isn't silently treated
    // as "nothing deployed yet".
    const show = isError || (deploymentStatus === DeploymentStatus.ACTIVE
        && !isLoading && (resources.length > 0 || !!invokeUrl));

    return (
        <CollapsibleSection show={show}>
            <OverviewSectionCard
                title="Agent Interface"
                actionHref={tryItHref}
                actionLabel="Try it"
                // `height: "100%"` alone doesn't equalize this against the
                // "Agent ID" card next to it — both sit inside a
                // CollapsibleSection/Collapse, which sizes itself to its own
                // content height rather than stretching to the Grid row's
                // tallest sibling. A floor roughly matching Agent ID's usual
                // height (avatar row + card chrome) keeps the row looking
                // level when this card's own content (e.g. a single
                // "Unable to find API schema" line) is much shorter.
                sx={{ height: "100%", minHeight: 116 }}
            >
                {isError ? (
                    <Typography variant="body2" color="error">
                        Unable to load the agent interface. Try again later.
                    </Typography>
                ) : resources.length === 0 ? (
                    <Typography
                        variant="caption"
                        color="text.disabled"
                        sx={{ display: "block", fontStyle: "italic" }}
                    >
                        Unable to find API schema
                    </Typography>
                ) : (
                    <Box display="flex" flexWrap="wrap" gap={1}>
                        {resources.map((resource) => (
                            <Box
                                key={`${resource.method} ${resource.path}`}
                                display="flex"
                                alignItems="center"
                                gap={0.75}
                                sx={{
                                    border: "1px solid",
                                    borderColor: "divider",
                                    borderRadius: "999px",
                                    // The Chip already carries its own pill
                                    // padding on the left, so a smaller pl here
                                    // (vs. pr, which backs onto plain unpadded
                                    // text) keeps the inset even on both ends.
                                    pl: 0.5,
                                    pr: 1.25,
                                    py: 0.5,
                                }}
                            >
                                <Chip
                                    label={METHOD_LABEL[resource.method] ?? resource.method}
                                    size="small"
                                    variant="outlined"
                                    color={METHOD_COLOR[resource.method] ?? "default"}
                                    sx={{ fontSize: "0.6875rem", fontWeight: 600 }}
                                />
                                <Typography variant="body2" sx={{ fontFamily: "monospace" }} noWrap>
                                    {resource.path}
                                </Typography>
                            </Box>
                        ))}
                    </Box>
                )}
            </OverviewSectionCard>
        </CollapsibleSection>
    );
};
