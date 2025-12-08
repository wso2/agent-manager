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
  BackgoundLoader,
  NoDataFound,
  PageLayout,
} from "@agent-management-platform/views";
import { useListProjects } from "@agent-management-platform/api-client";
import { generatePath, Link, useParams } from "react-router-dom";
import {
  absoluteRouteMap,
  ProjectResponse,
} from "@agent-management-platform/types";
import {
  Avatar,
  Box,
  ButtonBase,
  Card,
  CardContent,
  TextField,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import {
  Package,
  User as PersonOutline,
  Search as SearchRounded,
  Clock as TimerOutlined,
} from "@wso2/oxygen-ui-icons-react";
import { useMemo, useState } from "react";

dayjs.extend(relativeTime);

function ProjectCard(props: { project: ProjectResponse }) {
  const { project } = props;
  const theme = useTheme();
  const { orgId } = useParams();
  return (
    <ButtonBase
      component={Link}
      to={generatePath(absoluteRouteMap.children.org.children.projects.path, {
        orgId: orgId,
        projectId: project.name,
      })}
    >
      <Card
        sx={{
          minWidth: 320,
          transition: theme.transitions.create(["all"], {
            duration: theme.transitions.duration.short,
          }),
          "&.MuiCard-root": {
            backgroundColor: "background.paper",
          },
          "&:hover": {
            borderColor: "primary.main",
            backgroundColor: "background.default",
            transform: "translateY(-2px)",
          },
        }}
      >
        <CardContent>
          <Box display="flex" alignItems="center" gap={1.5}>
            <Avatar
              sx={{
                height: 64,
                width: 64,
                "&.MuiAvatar-root": {
                  transition: theme.transitions.create(["all"], {
                    duration: theme.transitions.duration.short,
                  }),
                  bgcolor: "secondary.main",
                },
              }}
            >
              <Package fontSize="inherit" size={24} />
            </Avatar>
            <Box display="flex" flexDirection="column" alignItems="flex-start">
              <Typography variant="h5">{project.displayName}</Typography>
              <Typography variant="body2" color="text.secondary">
                {project.description ? project.description : "No description"}
              </Typography>
            </Box>
          </Box>
          <Typography
            variant="body2"
            color="textPrimary"
            sx={{
              mt: 2,
              display: "flex",
              alignItems: "center",
              justifyContent: "flex-start",
            }}
          >
            <TimerOutlined size={16} />
            &nbsp;
            {dayjs(project.createdAt).fromNow()}
          </Typography>
        </CardContent>
      </Card>
    </ButtonBase>
  );
}

export function ProjectList() {
  const { orgId } = useParams();
  const { data: projects, isRefetching } = useListProjects({
    orgName: orgId ?? "default",
  });
  const [search, setSearch] = useState("");

  const filteredProjects = useMemo(
    () =>
      projects?.projects?.filter((project) =>
        project.displayName.toLowerCase().includes(search.toLowerCase())
      ) || [],
    [projects, search]
  );

  return (
    <PageLayout title="Projects" description="List of projects">
      {isRefetching && <BackgoundLoader />}
      <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
        <TextField
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          slotProps={{ input: { endAdornment: <SearchRounded size={16} /> } }}
          fullWidth
          variant="outlined"
          placeholder="Search Projects"
          disabled={!projects?.projects?.length}
        />
        <Box
          sx={{
            display: "inline-flex",
            flexWrap: "wrap",
            gap: 2,
            width: "100%",
            justifyContent: "start",
            alignItems: "start",
            overflow: "visible",
            minHeight: "calc(100vh - 250px)",
          }}
        >
          {filteredProjects?.map((project) => (
            <ProjectCard key={project.createdAt} project={project} />
          ))}
          {filteredProjects?.length === 0 && (
            <Box display="flex" width="100%" justifyContent="center" alignItems="center" pt={10} height="100%">
            <NoDataFound
              message="No projects found"
              subtitle="Create a new project to get started"
              icon={<PersonOutline fontSize="inherit" />}
            />
            </Box>
          )}
        </Box>
      </Box>
    </PageLayout>
  );
}
