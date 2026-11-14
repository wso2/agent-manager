/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React from "react";
import { Typography } from "@wso2/oxygen-ui";

interface AgentNameWithProjectProps {
  name: string;
  projectName?: string;
}

// Agent names are only unique within a project, so pairing the name with its
// (muted) project name disambiguates two agents that would otherwise look
// identical in a picker option or a members/assignees table row.
export const AgentNameWithProject: React.FC<AgentNameWithProjectProps> = ({
  name,
  projectName,
}) => (
  <>
    <Typography variant="body2">{name}</Typography>
    <Typography variant="caption" color="text.secondary">
      {projectName}
    </Typography>
  </>
);

export default AgentNameWithProject;
