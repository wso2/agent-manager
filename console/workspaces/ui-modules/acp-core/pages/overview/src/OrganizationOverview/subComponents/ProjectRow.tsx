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

import { Link as RouterLink, generatePath } from "react-router-dom";
import { Box, Stack, Typography } from "@wso2/oxygen-ui";
import { Clock, Package } from "@wso2/oxygen-ui-icons-react";
import { absoluteRouteMap, type ProjectResponse } from "@agent-management-platform/types";
import { formatRelativeTime } from "@agent-management-platform/shared-component";
import { IconTile } from "@agent-management-platform/views";

const orgRoutes = absoluteRouteMap.children.org;

export function ProjectRow({ project, orgId }: { project: ProjectResponse; orgId: string }) {
  return (
    <Stack
      component={RouterLink}
      to={generatePath(orgRoutes.children.projects.path, { orgId, projectId: project.name })}
      direction="row"
      alignItems="center"
      spacing={2}
      sx={{
        px: { xs: 2, md: 2.5 },
        py: 1.5,
        color: "text.primary",
        textDecoration: "none",
        "&:hover": { bgcolor: "action.hover" },
      }}
    >
      <IconTile selected size={40}>
        <Package size={20} />
      </IconTile>
      <Box sx={{ flex: 1, minWidth: 0 }}>
        <Typography variant="subtitle2" noWrap>
          {project.displayName}
        </Typography>
        {project.description && (
          <Typography variant="caption" color="text.secondary" noWrap sx={{ display: "block" }}>
            {project.description}
          </Typography>
        )}
      </Box>
      <Stack
        direction="row"
        spacing={0.75}
        alignItems="center"
        sx={{ color: "text.secondary", display: { xs: "none", sm: "flex" } }}
      >
        <Clock size={14} />
        <Typography variant="body2" noWrap>
          {formatRelativeTime(project.createdAt)}
        </Typography>
      </Stack>
    </Stack>
  );
}
