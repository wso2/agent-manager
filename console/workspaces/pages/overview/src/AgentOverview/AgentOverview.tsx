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

import { useGetAgent, useGetAgentKind, useUserDisplayName } from "@agent-management-platform/api-client";
import { InternalAgentOverview } from "./InternalAgentOverview";
import { useParams } from "react-router-dom";
import { ExternalAgentOverview } from "./ExternalAgentOverview";
import { useState } from "react";
import { Box, Button, Divider, Skeleton, Stack } from "@wso2/oxygen-ui";
import { Edit, Tag } from "@wso2/oxygen-ui-icons-react";
import { EditAgentDrawer } from "./EditAgentDrawer";
import {
    PageLayout,
    DescriptionCard,
    CreatedMetadata,
    CreatedByMetadata,
    PageMetaItem,
} from "@agent-management-platform/views";
import { AgentTypeChips, LabelChips } from "@agent-management-platform/shared-component";

function AgentOverviewSkeleton() {
    return (
        <Box display="flex" flexDirection="column" gap={4} width="100%">
            <Skeleton variant="rounded" width="100%" height="40vh" />
        </Box>
    );
}

export function AgentOverview() {
    const { orgId, agentId, projectId } = useParams();
    const [editAgentDrawerOpen, setEditAgentDrawerOpen] = useState(false);
    const { data: agent, isLoading: isAgentLoading } = useGetAgent({
        orgName: orgId,
        projName: projectId,
        agentName: agentId,
    });
    // The agent record only carries the creator's id (plus a best-effort
    // username); the profile lookup turns that into their real name.
    const resolvedName = useUserDisplayName({
        orgName: orgId ?? "",
        userId: agent?.createdBy?.id ?? "",
    });
    const createdByName = resolvedName ?? agent?.createdBy?.display ?? agent?.createdBy?.id;
    const { data: kind } = useGetAgentKind({ orgName: orgId ?? "", kindName: agent?.kindName ?? "" });

    return (
        <>
            <PageLayout docs={["agentLifecycle", "internalAndExternalAgent", "observability"]}
                variant="card"
                title={agent?.displayName ?? "Agent"}
                isLoading={isAgentLoading}
                titleTail={
                    <Stack direction="row" spacing={1} alignItems="center">
                        <AgentTypeChips
                            provisioning={agent?.provisioning}
                            agentType={agent?.agentType}
                            kindName={agent?.kindName}
                            kindDisplayName={kind?.displayName}
                        />
                    </Stack>
                }
                meta={
                    <>
                        {agent?.labels && Object.keys(agent.labels).length > 0 && (
                            <PageMetaItem icon={<Tag size={12} />} label="Custom tags">
                                <LabelChips labels={agent.labels} />
                            </PageMetaItem>
                        )}
                        <Stack
                            direction="row"
                            spacing={1}
                            alignItems="center"
                            divider={<Divider orientation="vertical" flexItem />}
                        >
                            <CreatedMetadata createdAt={agent?.createdAt} />
                            <CreatedByMetadata createdBy={createdByName} />
                        </Stack>
                    </>
                }
                actions={
                    <Button
                        variant="outlined"
                        size="small"
                        startIcon={<Edit size={16} />}
                        onClick={() => setEditAgentDrawerOpen(true)}
                        disabled={!agent}
                    >
                        Edit Agent
                    </Button>
                }
            >
                {isAgentLoading ? (
                    <AgentOverviewSkeleton />
                ) : (
                    <Box display="flex" flexDirection="column" gap={2}>
                        {agent?.description && (
                            <DescriptionCard content={agent.description} />
                        )}
                        {agent?.provisioning?.type === "internal" && <InternalAgentOverview />}
                        {agent?.provisioning?.type === "external" && <ExternalAgentOverview />}
                    </Box>
                )}
            </PageLayout>

            {agent && (
                <EditAgentDrawer
                    open={editAgentDrawerOpen}
                    onClose={() => setEditAgentDrawerOpen(false)}
                    agent={agent}
                    orgId={orgId || "default"}
                    projectId={projectId || "default"}
                />
            )}
        </>
    );
}
