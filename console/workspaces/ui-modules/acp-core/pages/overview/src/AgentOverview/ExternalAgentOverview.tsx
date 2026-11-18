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

import { Box, Skeleton } from "@wso2/oxygen-ui";
import { useParams } from "react-router-dom";
import {
  DeploymentStatus,
  EnvironmentCard,
  usePipelineEnvironmentsState,
  UppercaseCaptionLabel,
} from "@agent-management-platform/shared-component";
import { NoDataFound } from "@agent-management-platform/views";
import { EnvironmentSectionsContent } from "./EnvironmentSectionsContent";
import { EnvironmentSingleHeader } from "./EnvironmentSingleHeader";
import { EnvironmentTabsBar } from "./EnvironmentTabsBar";
import { useSelectedEnvironmentParam } from "./useSelectedEnvironmentParam";

export const ExternalAgentOverview = () => {
  const { agentId, orgId, projectId } = useParams();

  // Show only the environments in the current project's deployment pipeline,
  // ordered by the promotion chain. isLoading covers environments + project + pipelines.
  const { environments: sortedEnvironmentList, isLoading: isEnvironmentsLoading } =
    usePipelineEnvironmentsState(orgId, projectId);
  const { selectedEnvironment, selectEnvironment } =
    useSelectedEnvironmentParam(sortedEnvironmentList);

  // Once loaded, a single environment doesn't need a "which environment" tab
  // strip or section label — only shown when there's more than one, or while
  // still loading (before we know how many there are).
  const showEnvironmentsHeader = isEnvironmentsLoading || sortedEnvironmentList.length !== 1;
  const hasMultipleEnvironments = sortedEnvironmentList.length > 1;

  return (
    <>
      <Box display="flex" flexDirection="column" gap={2}>
        {showEnvironmentsHeader && <UppercaseCaptionLabel>Environments</UppercaseCaptionLabel>}
        {isEnvironmentsLoading ? (
          <Box display="flex" flexDirection="column" gap={2}>
            <Skeleton variant="rounded" height={100} />
            <Skeleton variant="rounded" height={100} />
          </Box>
        ) : sortedEnvironmentList.length === 0 ? (
          <NoDataFound
            message="No environments found"
            subtitle="Environments will appear here once they are created"
          />
        ) : (
          selectedEnvironment &&
          orgId &&
          projectId &&
          agentId && (
            <EnvironmentCard
              key={selectedEnvironment.name}
              orgId={orgId}
              projectId={projectId}
              agentId={agentId}
              environment={selectedEnvironment}
              tabsHeader={
                hasMultipleEnvironments ? (
                  <EnvironmentTabsBar
                    environments={sortedEnvironmentList}
                    selectedName={selectedEnvironment.name}
                    onSelect={selectEnvironment}
                    dotColor={() => "success.main"}
                  />
                ) : (
                  <EnvironmentSingleHeader
                    environment={selectedEnvironment}
                    status={DeploymentStatus.ACTIVE}
                    dotColor="success.main"
                  />
                )
              }
              bottomContent={
                <EnvironmentSectionsContent
                  orgId={orgId}
                  projectId={projectId}
                  agentId={agentId}
                  envId={selectedEnvironment.name}
                  external
                />
              }
            />
          )
        )}
      </Box>
    </>
  );
};
