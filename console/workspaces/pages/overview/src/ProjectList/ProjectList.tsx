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

import { NoDataFound, PageLayout } from "@agent-management-platform/views";
import { useListProjects } from "@agent-management-platform/api-client";
import { generatePath, Link, useParams } from "react-router-dom";
import { absoluteRouteMap, ProjectResponse } from "@agent-management-platform/types";
import { alpha, Avatar, Box, ButtonBase, Card, TextField, Typography, useTheme } from "@mui/material";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import { PersonOutline, SearchRounded, TimerOutlined } from "@mui/icons-material";
import { useMemo, useState } from "react";

dayjs.extend(relativeTime);

function ProjectCard(props: { project: ProjectResponse }) {
    const { project } = props;
    const theme = useTheme();
    const { orgId } = useParams();
    return (
        <ButtonBase
            component={Link}
            to={generatePath(absoluteRouteMap.children.org.children.projects.path,
                { orgId: orgId, projectId: project.name })}
        >
            <Card
            >
                <Box
                    sx={{
                        display: 'flex',
                        width: theme.spacing(40),
                        height: theme.spacing(20),
                        flexDirection: 'column',
                        gap: 2,
                        p: theme.spacing(2),
                        justifyContent: 'flex-start',
                        alignItems: 'flex-start',
                        "&:hover": {
                            backgroundColor: theme.palette.action.hover,
                        },
                    }}
                >
                    <Box
                        sx={{
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'space-between',

                        }}
                    >
                        <Avatar sx={{ background: `linear-gradient(45deg, ${alpha(theme.palette.primary.main, 0.5)} 30%, ${alpha(theme.palette.secondary.main, 0.5)} 90%)` }} >
                            <PersonOutline fontSize="inherit" />
                        </Avatar>
                        <Box
                            sx={{
                                p: 2,
                                display: 'flex',
                                flexDirection: 'column',
                                alignItems: 'flex-start',
                            }}>
                            <Typography variant="h6">{project.displayName}</Typography>
                            <Typography variant="body2" color="text.secondary">{project.description ?
                                project.description : 'No description'}
                            </Typography>
                        </Box>

                    </Box>
                    <Typography
                        variant="body2"
                        color="text.secondary"
                        sx={{
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'flex-start',
                        }}
                    >
                        <TimerOutlined fontSize="inherit" />
                        &nbsp;
                        {dayjs(project.createdAt).fromNow()}
                    </Typography>
                </Box>
            </Card>
        </ButtonBase>
    );
}

export function ProjectList() {
    const { orgId } = useParams();
    const { data: projects } = useListProjects({
        orgName: orgId ?? 'default',
    });
    const theme = useTheme();
    const [search, setSearch] = useState('');

    const filteredProjects = useMemo(() =>
        projects?.projects?.filter((project) =>
            project.displayName.toLowerCase().includes(search.toLowerCase())) || [],
        [projects, search]);

    return (
        <PageLayout
            title="Projects"
            description="List of projects"
        >
            <TextField
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                slotProps={{ input: { endAdornment: <SearchRounded fontSize='small' /> } }}
                fullWidth
                size='small'
                sx={{
                    m: theme.spacing(1, 0),
                }}
                variant='standard'
                placeholder='Search agents'
                disabled={!projects?.projects?.length}
            />
            <Box sx={{ display: 'inline-flex', flexWrap: 'wrap', gap: 2, py: theme.spacing(2) }}>
                {filteredProjects?.map((project) => (
                    <ProjectCard key={project.createdAt} project={project} />
                ))}
            </Box>
            <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', pt: theme.spacing(10), height: '100%' }}>
                {filteredProjects?.length === 0 && (
                    <NoDataFound message="No projects found" subtitle="Create a new project to get started" icon={<PersonOutline fontSize="inherit" />} />
                )}
            </Box>
        </PageLayout>
    );
}
