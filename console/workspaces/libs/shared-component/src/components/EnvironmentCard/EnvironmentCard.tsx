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

import { useGetAgent } from "@agent-management-platform/api-client";
import type { Environment } from "@agent-management-platform/types";
import {
  Box,
  Card,
  CardContent,
  Skeleton,
  Typography,
} from "@wso2/oxygen-ui";

export interface EnvironmentCardProps {
  environment?: Environment;
  orgId: string;
  projectId: string;
  agentId: string;
  actions?: React.ReactNode;
  /**
   * Rendered below the header/tabs row. Individual sections (Capabilities,
   * Configs, Agent Identity) each decide for themselves whether they have
   * anything to show.
   */
  bottomContent?: React.ReactNode;
  /** Replaces the environment name heading, e.g. a tab strip switching between sibling envs. */
  tabsHeader?: React.ReactNode;
  /**
   * Suppresses the environment name heading entirely (no tabsHeader, no
   * fallback title) — e.g. when there's only one environment and naming it
   * adds no information.
   */
  hideEnvTitle?: boolean;
}

export const EnvironmentCard = (props: EnvironmentCardProps) => {
  const {
    environment,
    orgId,
    projectId,
    agentId,
    actions,
    bottomContent,
    tabsHeader,
    hideEnvTitle,
  } = props;
  const { isLoading: isAgentLoading } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });

  const envTitle = environment?.displayName ?? environment?.name ?? "Environment";

  if (isAgentLoading) {
    return <Skeleton variant="rounded" height={100} />;
  }

  return (
    <Card variant="outlined">
      <CardContent>
        <Box display="flex" flexDirection="row" gap={1} justifyContent="space-between" alignItems="center">
          <Box display="flex" flexDirection="row" gap={1} alignItems="center">
            {!hideEnvTitle && (tabsHeader ?? <Typography variant="h6">{envTitle}</Typography>)}
          </Box>
          <Box display="flex" flexDirection="row" gap={1} alignItems="center">
            {actions}
          </Box>
        </Box>
        {bottomContent}
      </CardContent>
    </Card>
  );
};
