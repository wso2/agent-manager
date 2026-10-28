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

import { useMemo } from "react";
import { Box } from "@wso2/oxygen-ui";
import { generatePath, useParams } from "react-router-dom";
import { PageLayout, ExternalAgentIcon, InternalAgentIcon } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { useListAgents } from "@agent-management-platform/api-client";
import { NewAgentTypeCard } from "./NewAgentTypeCard";
import { NewAgentSampleCard } from "./NewAgentSampleCard";

interface NewAgentOptionsProps {
    onSelect: (option: 'new' | 'existing') => void;
    onSelectSample: (sampleId: string) => void;
}

export const NewAgentOptions = ({ onSelect, onSelectSample }: NewAgentOptionsProps) => {
    const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>();

    const { data: agents } = useListAgents({
        orgName: orgId ?? "default",
        projName: projectId ?? "default",
    });

    const handleSelect = (type: string) => {
        onSelect(type as 'new' | 'existing');
    };

    const hasAgents = Boolean(agents?.agents?.length && agents?.agents?.length > 0);

    const backHref = useMemo(() => {
        if (!hasAgents) {
            return undefined;
        }
        return generatePath(absoluteRouteMap.children.org.children.projects.path, {
            orgId: orgId ?? "",
            projectId: projectId ?? "default",
        });
    }, [hasAgents, orgId, projectId]);

    return (
        <PageLayout
            title="Add a New Agent"
            description="Choose how you want to get started. You can deploy an agent on the platform or register an agent that already runs elsewhere."
            disableIcon
            backHref={backHref}
            backLabel="Back to Projects Home"
        >
            <Box display="flex" flexDirection="column" gap={3} width={1}>
                <Box display="flex" flexDirection={{ xs: "column", sm: "row" }} gap={3}>
                    <NewAgentTypeCard
                        type="existing"
                        title="Externally-Hosted Agent"
                        subheader="Connect an existing agent running outside the platform and enable observability and governance."
                        icon={<ExternalAgentIcon width={150} height={160} />}
                        ctaLabel="Register an agent"
                        onClick={handleSelect}
                    />
                    <NewAgentTypeCard
                        type="new"
                        title="Platform-Hosted Agent"
                        subheader="Deploy and manage agents with full lifecycle support, including built-in CI/CD, scaling, observability, and governance."
                        icon={<InternalAgentIcon width={150} height={160} />}
                        ctaLabel="Deploy a new agent"
                        onClick={handleSelect}
                    />
                </Box>
                <NewAgentSampleCard onSelectSample={onSelectSample} />
            </Box>
        </PageLayout>
    );
};
