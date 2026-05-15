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

import { Box, Form, Typography } from "@wso2/oxygen-ui";
import { FileCode, Package } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useParams } from "react-router-dom";
import { PageLayout } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";

interface NewAgentSourceOptionsProps {
    onSelect: (option: "source" | "catalog") => void;
}

export const NewAgentSourceOptions = ({ onSelect }: NewAgentSourceOptionsProps) => {
    const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>();

    const handleSelect = (type: string) => {
        onSelect(type as "source" | "catalog");
    };

    const backHref = generatePath(absoluteRouteMap.children.org.children.projects.path, {
        orgId: orgId ?? "",
        projectId: projectId ?? "default",
    });

    const sourceOptions = [
        {
            type: "source",
            title: "Source Code",
            subheader: "Provide agent source code from project repository",
            icon: <FileCode size={56} />,
        },
        {
            type: "catalog",
            title: "Agent Catalog",
            subheader: "Pick an Agent Kind from Agent Catalog",
            icon: <Package size={56} />,
        },
    ] as const;

    return (
        <PageLayout
            title="Create a Platform-Hosted Agent"
            description="Pick a source type for the agent"
            disableIcon
            backHref={backHref}
            backLabel="Back to Agents"
        >
            <Box display="flex" flexDirection="row" gap={3} flexWrap="wrap">
                {sourceOptions.map((option) => (
                    <Form.CardButton
                        key={option.type}
                        onClick={() => handleSelect(option.type)}
                        sx={{
                            minHeight: 152,
                            px: 3,
                            py: 2,
                            display: "flex",
                            flexDirection: "row",
                            alignItems: "center",
                            justifyContent: "flex-start",
                            gap: 3,
                            textAlign: "left",
                        }}
                    >
                        <Box
                            sx={{
                                height: 112,
                                display: "flex",
                                alignItems: "center",
                                justifyContent: "center",
                                flexShrink: 0,
                            }}
                        >
                            {option.icon}
                        </Box>
                        <Box display="flex" flexDirection="column" gap={1} alignItems="flex-start">
                            <Typography variant="h3" textAlign="left">
                                {option.title}
                            </Typography>
                            <Typography variant="body1" textAlign="left" sx={{ maxWidth: 260 }}>
                                {option.subheader}
                            </Typography>
                        </Box>
                    </Form.CardButton>
                ))}
            </Box>
        </PageLayout>
    );
};
