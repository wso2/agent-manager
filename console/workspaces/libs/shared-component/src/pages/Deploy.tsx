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

import { Box, Button, Drawer, useTheme } from "@mui/material";
import { GroupNavLinks, SubTopNavBar } from "../components/EnvSubNavBar/SubTopNavBar";
import { generatePath, Route, Routes, useMatch, useParams, useSearchParams } from "react-router-dom";
import { absoluteRouteMap, relativeRouteMap, Environment } from "@agent-management-platform/types";
import { Rocket, HomeOutlined, PlayArrowOutlined, CodeOutlined, AutoGraph } from "@mui/icons-material";
import { Overview } from "./subPagesDeploy/Overview";
import { TryOut } from "./subPagesDeploy/TryOut";
import { useGetAgent, useListAgentDeployments, useListEnvironments } from "@agent-management-platform/api-client";
import { FadeIn } from "@agent-management-platform/views";
import { DeploymentConfig } from "../components/DeploymentConfig";
import { useCallback, useMemo } from "react";
import { Traces } from "./subPagesDeploy/Traces";

export function Deploy() {
  const theme = useTheme();
  const [searchParams, setSearchParams] = useSearchParams();
  const showDeploymentDrawer = searchParams.get('deploy') === 'true';
  const { envId, agentId, projectId, orgId } = useParams();
  const { data: agent } = useGetAgent({ 
    orgName: orgId ?? 'default', 
    projName: projectId ?? 'default', 
    agentName: agentId ?? '' 
  });
  // eslint-disable-next-line max-len
  const isOverview = useMatch(absoluteRouteMap.children.org.children.projects.children.agents.children.environment.children.deploy.wildPath);
  // eslint-disable-next-line max-len
  const isTryoout = useMatch(absoluteRouteMap.children.org.children.projects.children.agents.children.environment.children.tryOut.wildPath);
  // eslint-disable-next-line max-len
  const isTraces = useMatch(absoluteRouteMap.children.org.children.projects.children.agents.children.environment.children.observability.children.traces.wildPath);

  const navLinks: GroupNavLinks[] = [
    {
      id: "overview",
      navLinks: [
        {
          label: "Overview",
          id: "overview",
          icon: <HomeOutlined fontSize='inherit' />,
          isActive: !!isOverview,
          // eslint-disable-next-line max-len
          path: generatePath(absoluteRouteMap.children.org.children.projects.children.agents.children.environment.children.deploy.path, { orgId, projectId, agentId, envId: envId }),
        },
        {
          label: "Try Out",
          id: "try-out",
          icon: <PlayArrowOutlined fontSize='inherit' />,
          isActive: !!isTryoout,
          // eslint-disable-next-line max-len
          path: generatePath(absoluteRouteMap.children.org.children.projects.children.agents.children.environment.children.tryOut.path, { orgId, projectId, agentId, envId: envId }),
        },
      ]
    },
    {
      id: "observe",
      icon: <AutoGraph fontSize='inherit' />,
      navLinks: [
        {
          label: "Traces",
          id: "traces",
          icon: <CodeOutlined fontSize='inherit' />,
          isActive: !!isTraces,
          // eslint-disable-next-line max-len
          path: generatePath(absoluteRouteMap.children.org.children.projects.children.agents.children.environment.children.observability.children.traces.path, { orgId, projectId, agentId, envId: envId }),
        }
      ]
    }

  ];

  const { data: environments } = useListEnvironments({
    orgName: orgId || '',
  });
  const { data: deployments } = useListAgentDeployments({
    orgName: orgId || '',
    projName: projectId || '',
    agentName: agentId || '',
  }, {
    enabled: agent?.provisioning.type !== 'external',
  });

  const currentEnvironment = environments?.find(
    (environment: Environment) => environment.name === envId);
  const nextEnvironment = useMemo(() => {
    if (!currentEnvironment || !deployments) {
      return undefined;
    }
    return deployments[currentEnvironment.name]?.promotionTargetEnvironment;
  }, [currentEnvironment, deployments]);

  // Get the current deployment's imageId
  const currentDeployment = deployments && envId ? deployments[envId] : undefined;
  const imageId = currentDeployment?.imageId || 'busybox';

  const handlePromoteClick = useCallback(() => {
    const next = new URLSearchParams(searchParams);
    next.set('deploy', 'true');
    setSearchParams(next);
  }, [searchParams, setSearchParams]);

  const handleCloseDrawer = useCallback(() => {
    const next = new URLSearchParams(searchParams);
    next.delete('deploy');
    setSearchParams(next);
  }, [searchParams, setSearchParams]);

  return (
    <FadeIn>
      <Box display="flex" flexDirection="column" gap={1} pt={1}>
        <SubTopNavBar
          navLinks={navLinks}
          actionButtons={nextEnvironment ?
            <FadeIn>
              <Button
                startIcon={<Rocket />}
                variant="contained"
                color="primary"
                size="small"
                onClick={handlePromoteClick}
              >
                Promote to {nextEnvironment?.displayName}
              </Button>
            </FadeIn> : undefined}
        />
        <Box>
          <Routes>
            <Route index element={<Overview />} />
            <Route path={relativeRouteMap.children.org.children.projects.children.agents.children.environment.children.tryOut.path + "/*"} element={<TryOut />} />
            <Route path={relativeRouteMap.children.org.children.projects.children.agents.children.environment.children.observability.path + "/*"} element={
              <Routes>
                <Route path={relativeRouteMap.children.org.children.projects.children.agents.children.environment.children.observability.children.traces.path + "/*"} element={<Traces />} />
              </Routes>
            } />
          </Routes>
        </Box>
      </Box>
      <Drawer
        anchor="right"
        open={showDeploymentDrawer}
        onClose={handleCloseDrawer}
        sx={{
          zIndex: 1300,
        }}
      >
        <Box
          width={theme.spacing(100)}
          p={2}
          height="100%"
          display="flex"
          flexDirection="column"
          gap={2}
          bgcolor={theme.palette.background.paper}
        >
          {showDeploymentDrawer && nextEnvironment && (
            <DeploymentConfig
              onClose={handleCloseDrawer}
              imageId={imageId}
              to={nextEnvironment.name}
              orgName={orgId || ''}
              projName={projectId || ''}
              agentName={agentId || ''}
            />
          )}
        </Box>
      </Drawer>
    </FadeIn>
  );
}

