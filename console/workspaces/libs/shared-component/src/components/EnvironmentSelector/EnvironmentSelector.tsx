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

import { FormControl, MenuItem, Select, Typography } from "@wso2/oxygen-ui";
import { useMemo } from "react";
import { useLocation, useNavigate, useParams } from "react-router-dom";

import { usePipelineEnvironments } from "../../utils/usePipelineEnvironments";

/**
 * Self-contained environment selector for env-scoped pages.
 * Reads envId from the URL, lists only the environments that belong to the
 * current project's deployment pipeline promotion chain, and navigates to the
 * same page with the new envId when the selection changes.
 * Renders nothing when there is only one qualifying environment or no envId.
 */
export function EnvironmentSelector() {
    const { orgId, projectId, envId } = useParams<{
        orgId: string; projectId: string; envId: string;
    }>();
    const { pathname } = useLocation();
    const navigate = useNavigate();

    const pipelineEnvironments = usePipelineEnvironments(orgId, projectId);

    const selectedEnvironment = useMemo(
        () => pipelineEnvironments.find((env) => env.name === envId),
        [pipelineEnvironments, envId],
    );

    if (!envId || pipelineEnvironments.length <= 1) {
        return null;
    }

    return (
        <FormControl size="small" sx={{ minWidth: 160 }}>
            <Select
                value={envId}
                onChange={(e) => {
                    const newEnvId = e.target.value as string;
                    navigate(
                        pathname.replace(`/environment/${envId}`, `/environment/${newEnvId}`),
                    );
                }}
                renderValue={(value) => (
                    <Typography>
                        {selectedEnvironment?.displayName ?? value}
                        {" "}
                        Environment
                    </Typography>
                )}
            >
                {pipelineEnvironments.map((env) => (
                    <MenuItem key={env.name} value={env.name}>
                        {env.displayName ?? env.name}
                    </MenuItem>
                ))}
            </Select>
        </FormControl>
    );
}
