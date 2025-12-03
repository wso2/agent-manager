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

import { Box, Tab, Tabs } from "@mui/material";
import {
    Home,
    Visibility,
} from "@mui/icons-material";
import { useLocation, useNavigate, useParams, generatePath } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";

export const AgentOverviewNav = () => {
    const location = useLocation();
    const navigate = useNavigate();
    const { orgId, projectId, agentId } = useParams();

    // Determine current tab based on path
    const getCurrentTab = () => {
        const path = location.pathname;
        if (path.includes('/observe')) return 'observe';
        if (path.includes('/evaluate')) return 'evaluate';
        if (path.includes('/govern')) return 'govern';
        return 'overview';
    };

    const handleTabChange = (_event: React.SyntheticEvent, newValue: string) => {
        const basePath = generatePath(
            absoluteRouteMap.children.org.children.projects.children.agents.path,
            { orgId, projectId, agentId }
        );

        if (newValue === 'overview') {
            navigate(basePath);
        } else {
            navigate(`${basePath}/${newValue}`);
        }
    };

    return (
        <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
            <Tabs
                value={getCurrentTab()}
                onChange={handleTabChange}
                sx={{ minHeight: 48 }}
            >
                <Tab
                    icon={<Home fontSize="small" />}
                    iconPosition="start"
                    label="Overview"
                    value="overview"
                    sx={{ minHeight: 48 }}
                />
                <Tab
                    icon={<Visibility fontSize="small" />}
                    iconPosition="start"
                    label="Observe"
                    value="observe"
                    sx={{ minHeight: 48 }}
                />
                {/* <Tab
                    icon={<Assessment fontSize="small" />}
                    iconPosition="start"
                    label="Evaluate"
                    value="evaluate"
                    sx={{ minHeight: 48 }}
                />
                <Tab
                    icon={<Shield fontSize="small" />}
                    iconPosition="start"
                    label="Govern"
                    value="govern"
                    sx={{ minHeight: 48 }}
                /> */}
            </Tabs>
        </Box>
    );
};
