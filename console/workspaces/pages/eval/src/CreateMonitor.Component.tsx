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

import React, { useState } from "react";
import { PageLayout } from "@agent-management-platform/views";
import { generatePath, useParams } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { CreateMonitorForm, SelectPresetMonitors } from "./subComponents/CreateMonitorForm";
import { Button, Stack } from "@wso2/oxygen-ui";
import { ArrowLeft, ArrowRight } from "@wso2/oxygen-ui-icons-react";

export const CreateMonitorComponent: React.FC = () => {
    const { agentId, envId, orgId, projectId } = useParams<{
        agentId: string, envId: string, orgId: string, projectId: string
    }>();

    const [page, setPage] = useState<1 | 2>(1);

    return (
        <PageLayout
            title="Create Monitor"
            description="Define a new eval monitor by selecting targets, thresholds, and alerting rules."
            disableIcon
            backLabel="Back to Monitors"
            backHref={
                generatePath(
                    absoluteRouteMap.children.org.children.projects.children.agents
                        .children.environment.children.evaluation.children.monitor.path,
                    { orgId, envId, projectId, agentId })
            }
        >
            <Stack spacing={3}>
                {
                    page === 1 &&
                    <CreateMonitorForm />
                }

                {
                    page === 2 && (
                        <SelectPresetMonitors />
                    )
                }
                <Stack direction="row" gap={2} >
                    {
                        page === 1 && (
                            <>
                                <Button variant="contained" color="primary" endIcon={<ArrowRight size={20} />} onClick={() => setPage(2)}>
                                    Next
                                </Button>
                            </>
                        )
                    }
                    {
                        page === 2 && (
                            <>
                                <Button variant="outlined" startIcon={<ArrowLeft size={20} />} color="primary" onClick={() => setPage(1)}>
                                    Previous
                                </Button>
                                <Button variant="contained" color="primary">
                                    Create Monitor
                                </Button>
                            </>
                        )
                    }
                </Stack>
            </Stack>
        </PageLayout>
    )
};

export default CreateMonitorComponent;
