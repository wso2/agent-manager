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
import { absoluteRouteMap, relativeRouteMap } from "@agent-management-platform/types";
import { Construction, HomeOutlined, PlayArrowOutlined } from "@mui/icons-material";
import { Overview } from "./subPagesBuild/Overview/Overview";
import { Run } from "./subPagesBuild/Run";
import { useCallback } from "react";
import { BuildPanel } from "../components/BuildPanel";

export function Build() {
  const { agentId, projectId, orgId } = useParams();
  const theme = useTheme();
  const [searchParams, setSearchParams] = useSearchParams();
  const isBuildPanelOpen = searchParams.get('buildPanel') === 'open';

  // eslint-disable-next-line max-len
  const isOverview = useMatch(absoluteRouteMap.children.org.children.projects.children.agents.path);
  // eslint-disable-next-line max-len
  const isRun = useMatch(absoluteRouteMap.children.org.children.projects.children.agents.path);

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
          path: generatePath(absoluteRouteMap.children.org.children.projects.children.agents.path, { orgId, projectId, agentId }),
        },
        {
          label: "Build Details",
          id: "build-details",
          icon: <PlayArrowOutlined fontSize='inherit' />,
          isActive: !!isRun,
          // eslint-disable-next-line max-len
          path: generatePath(absoluteRouteMap.children.org.children.projects.children.agents.path, { orgId, projectId, agentId }),
        },
      ]
    }
  ];

  const handleBuild = useCallback(() => {
    const next = new URLSearchParams(searchParams);
    next.set('buildPanel', 'open');
    setSearchParams(next);
  }, [searchParams, setSearchParams]);

  const closeBuildPanel = useCallback(() => {
    const next = new URLSearchParams(searchParams);
    next.delete('buildPanel');
    setSearchParams(next);
  }, [searchParams, setSearchParams]);

  return (
    <>
      <Box display="flex" flexDirection="column" gap={1} pt={1}>
        <SubTopNavBar navLinks={navLinks} actionButtons={<Button onClick={handleBuild} variant="text" size="small" color="inherit" startIcon={<Construction />}>Build</Button>} />
        <Box>
          <Routes>
            <Route index element={<Overview />} />
            <Route path={relativeRouteMap.children.org.children.projects.children.agents.path + "/*"} element={<Run />} />
          </Routes>
        </Box>
      </Box>

      <Drawer
        anchor="right"
        open={isBuildPanelOpen}
        onClose={closeBuildPanel}
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
          <BuildPanel
            onClose={closeBuildPanel}
            orgName={orgId || ''}
            projName={projectId || ''}
            agentName={agentId || ''}
          />
        </Box>
      </Drawer>
    </>
  );
}
