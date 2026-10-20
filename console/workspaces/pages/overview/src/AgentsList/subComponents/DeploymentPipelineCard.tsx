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

import {
  useGetProject,
  useListDeploymentPipelines,
  useListEnvironments,
  useUpdateProject,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  isDeploymentPipelineManagedMode,
  type Environment,
} from "@agent-management-platform/types";
import {
  Box,
  Button,
  Card,
  CardContent,
  Divider,
  Skeleton,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { ArrowLeftRight, Edit, GitBranch } from "@wso2/oxygen-ui-icons-react";
import { useMemo, useState } from "react";
import { generatePath, Link, useParams } from "react-router-dom";
import { SelectPipelineDrawer } from "./SelectPipelineDrawer";
import { buildPromotionChain, PromotionChainChips } from "./promotionChain";

const CARD_SX = {
  minWidth: 300,
  flexGrow: 1,
};

export function DeploymentPipelineCard() {
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>();
  const [selectDrawerOpen, setSelectDrawerOpen] = useState(false);
  const [selectDrawerKey, setSelectDrawerKey] = useState(0);

  const openSelectDrawer = () => {
    setSelectDrawerKey((key) => key + 1);
    setSelectDrawerOpen(true);
  };

  const { data: project, isLoading: isLoadingProject } = useGetProject({
    orgName: orgId,
    projName: projectId,
  });

  const { data: pipelinesData, isLoading: isLoadingPipelines } = useListDeploymentPipelines(
    { orgName: orgId },
  );

  const { data: environments } = useListEnvironments({ orgName: orgId });

  const { mutate: updateProject, isPending: isUpdatingPipeline } = useUpdateProject({
    orgName: orgId,
    projName: projectId,
  });

  const pipeline = useMemo(
    () => pipelinesData?.deploymentPipelines?.find((p) => p.name === project?.deploymentPipeline),
    [pipelinesData, project?.deploymentPipeline]
  );

  const envMap = useMemo(
    () => new Map<string, Environment>(environments?.map((e) => [e.name, e]) ?? []),
    [environments]
  );

  const promotionChain = useMemo(
    () => buildPromotionChain(pipeline?.promotionPaths ?? []),
    [pipeline]
  );

  const handleSelectPipeline = (pipelineName: string) => {
    if (!project || pipelineName === project.deploymentPipeline) {
      setSelectDrawerOpen(false);
      return;
    }
    updateProject(
      {
        name: project.name,
        displayName: project.displayName,
        description: project.description,
        deploymentPipeline: pipelineName,
      },
      { onSuccess: () => setSelectDrawerOpen(false) }
    );
  };

  const isLoading = isLoadingProject || isLoadingPipelines;

  return (
    <>
      <Card variant="outlined" sx={CARD_SX}>
        <CardContent>
          {isLoading ? (
            <Box display="flex" flexDirection="column" gap={1.5}>
              <Box display="flex" alignItems="center" gap={1}>
                <Skeleton variant="circular" width={18} height={18} />
                <Skeleton variant="text" width="60%" height={28} />
              </Box>
              <Skeleton variant="text" width="85%" height={16} />
              <Divider />
              <Skeleton variant="text" width="45%" height={12} />
              <Box display="flex" alignItems="center" gap={1}>
                <Skeleton variant="rounded" width={64} height={24} sx={{ borderRadius: 4 }} />
                <Skeleton variant="text" width={14} height={14} />
                <Skeleton variant="rounded" width={72} height={24} sx={{ borderRadius: 4 }} />
                <Skeleton variant="text" width={14} height={14} />
                <Skeleton variant="rounded" width={80} height={24} sx={{ borderRadius: 4 }} />
              </Box>
            </Box>
          ) : !pipeline ? (
            <Box display="flex" flexDirection="column" gap={1.5}>
              <Box display="flex" alignItems="center" justifyContent="space-between" gap={1}>
                <Typography variant="h6">Deployment Environments</Typography>
                <Button size="small" startIcon={<ArrowLeftRight size={14} />} onClick={openSelectDrawer}>
                  Select Pipeline
                </Button>
              </Box>
              <Divider />
              <Typography variant="body2" color="text.secondary" sx={{ fontStyle: "italic" }}>
                No deployment pipeline configured for this project.
              </Typography>
            </Box>
          ) : (
            <Box display="flex" flexDirection="column" gap={1.5}>
              <Typography variant="h6">Project Environments</Typography>
              <Divider />
              <Box display="flex" flexDirection="column" gap={0.5}>
                <Box display="flex" alignItems="center" justifyContent="space-between" gap={0.75}>
                  <Box display="flex" alignItems="center" gap={0.75}>
                    <GitBranch size={14} style={{ opacity: 0.6, flexShrink: 0 }} />
                    <Typography variant="body2" fontWeight={500}>
                      {pipeline.displayName || pipeline.name}
                    </Typography>
                  </Box>
                  <Tooltip title="Change Deployment Pipeline">
                    <Button
                      size="small"
                      startIcon={<ArrowLeftRight size={14} />}
                      onClick={openSelectDrawer}
                      disabled={isUpdatingPipeline}
                    >
                      Change
                    </Button>
                  </Tooltip>
                </Box>
                {pipeline.description && (
                  <Typography variant="body2" color="text.disabled" sx={{ pl: 2.75 }}>
                    {pipeline.description}
                  </Typography>
                )}
              </Box>

              <Divider />

              <Box display="flex" flexWrap="wrap" alignItems="center" gap={0.75}>
                <PromotionChainChips chain={promotionChain} envMap={envMap} />
                {!isDeploymentPipelineManagedMode() && (
                  <Tooltip title="Edit Promotion Chain">
                    <Button
                      size="small"
                      startIcon={<Edit size={14} />}
                      component={Link}
                      to={generatePath(
                        absoluteRouteMap.children.org.children.deploymentPipelines.children.view
                          .path,
                        { orgId, pipelineName: pipeline.name },
                      )}
                    >
                      Edit
                    </Button>
                  </Tooltip>
                )}
              </Box>
            </Box>
          )}
        </CardContent>
      </Card>
      {project && (
        <SelectPipelineDrawer
          key={selectDrawerKey}
          open={selectDrawerOpen}
          onClose={() => setSelectDrawerOpen(false)}
          pipelines={pipelinesData?.deploymentPipelines ?? []}
          envMap={envMap}
          selectedName={project.deploymentPipeline}
          isUpdating={isUpdatingPipeline}
          onSelect={handleSelectPipeline}
        />
      )}
    </>
  );
}
