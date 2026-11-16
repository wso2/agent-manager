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

import React from "react";
import {
    Box,
    Card,
    Chip,
    CircularProgress,
    IconButton,
    Skeleton,
    Tooltip,
    Typography,
} from "@wso2/oxygen-ui";
import { CheckCircle, ExternalLink, GitHub, XCircle } from "@wso2/oxygen-ui-icons-react";
import {
    BUILD_STATUS_COLOR_MAP,
    type Build,
    type BuildResponse,
    type BuildStatus,
    type RepositoryConfig,
    absoluteRouteMap,
} from "@agent-management-platform/types";
import { formatRelativeTime, parseGitHubUrl } from "@agent-management-platform/shared-component";
import { generatePath, Link } from "react-router-dom";
import { UppercaseCaptionLabel } from "./SectionHeader";

interface AgentInfoCardProps {
    orgId: string;
    projectId: string;
    agentId: string;
    repository?: RepositoryConfig;
    latestBuild?: BuildResponse;
    isBuildsLoading?: boolean;
    build?: Build;
}

/** Builds a link to the exact branch/path in the repository, when possible. */
function buildRepoTreeUrl(url: string, branch: string, appPath: string | null): string {
    if (!branch) return url;
    if (appPath) {
        const normalized = appPath.startsWith("/") ? appPath.substring(1) : appPath;
        return `${url}/tree/${branch}/${normalized}`;
    }
    return `${url}/tree/${branch}`;
}

function statusTextColor(status?: BuildStatus): string {
    const color = status ? BUILD_STATUS_COLOR_MAP[status] : undefined;
    return color && color !== "default" ? `${color}.main` : "text.secondary";
}

function BuildStatusIcon({ status }: { status?: BuildStatus }) {
    if (!status) return null;
    if (status === "Running" || status === "Pending") {
        return <CircularProgress size={12} color="inherit" />;
    }
    if (status === "Failed") return <XCircle size={14} />;
    return <CheckCircle size={14} />;
}

interface SourceInfoProps {
    repoLabel: string | null;
    appPath: string | null;
    repoTreeUrl: string | null;
    branch?: string;
    buildpackLabel: string | null;
}

function SourceInfo({
    repoLabel,
    appPath,
    repoTreeUrl,
    branch,
    buildpackLabel,
}: SourceInfoProps) {
    return (
        <>
            <UppercaseCaptionLabel sx={{ flexShrink: 0 }}>Source</UppercaseCaptionLabel>

            <GitHub size={14} style={{ flexShrink: 0 }} />

            <Box display="flex" alignItems="center" gap={0} sx={{ flexShrink: 0 }}>
                <Typography variant="body2" noWrap>
                    {repoLabel ?? "—"}
                </Typography>

                {appPath && (
                    <Typography variant="body2" color="text.disabled" noWrap>
                        {appPath}
                    </Typography>
                )}
            </Box>

            {repoTreeUrl && (
                <IconButton
                    size="small"
                    component="a"
                    href={repoTreeUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    onClick={(e) => e.stopPropagation()}
                    sx={{ p: 0.25, flexShrink: 0 }}
                    aria-label="Open source repository"
                >
                    <ExternalLink size={13} />
                </IconButton>
            )}

            {branch && (
                <Chip label={branch} size="small" variant="outlined" sx={{ flexShrink: 0 }} />
            )}
            {buildpackLabel && (
                <Chip
                    label={buildpackLabel}
                    size="small"
                    variant="outlined"
                    sx={{ flexShrink: 0, display: { xs: "none", lg: "inline-flex" } }}
                />
            )}
        </>
    );
}

interface BuildStatusInfoProps {
    isBuildsLoading?: boolean;
    latestBuild?: BuildResponse;
}

function BuildStatusInfo({ isBuildsLoading, latestBuild }: BuildStatusInfoProps) {
    if (isBuildsLoading) {
        return <Skeleton variant="text" width={100} sx={{ flexShrink: 0 }} />;
    }

    if (!latestBuild) {
        return (
            <Typography variant="body2" color="text.secondary" sx={{ flexShrink: 0 }}>
                No builds yet
            </Typography>
        );
    }

    return (
        <Tooltip
            title={`Triggered ${formatRelativeTime(latestBuild.startedAt)}`}
        >
            <Box
                display="flex"
                alignItems="center"
                gap={0.5}
                sx={{ flexShrink: 0, color: statusTextColor(latestBuild.status) }}
            >
                <BuildStatusIcon status={latestBuild.status} />
                <Typography variant="body2" fontWeight={600} color="inherit">
                    {latestBuild.status}
                </Typography>
            </Box>
        </Tooltip>
    );
}

export const AgentInfoCard: React.FC<AgentInfoCardProps> = ({
    orgId,
    projectId,
    agentId,
    repository,
    latestBuild,
    isBuildsLoading,
    build,
}) => {
    const buildpackLabel = (() => {
        if (!build) return null;
        if (build.type === "buildpack") {
            const { language, languageVersion } = build.buildpack;
            return languageVersion ? `${language} ${languageVersion}` : language;
        }
        return "Docker";
    })();

    const buildsPath = generatePath(
        absoluteRouteMap.children.org.children.projects.children.agents.children.build.path,
        { orgId, projectId, agentId },
    );

    const parsedRepo = repository?.url ? parseGitHubUrl(repository.url) : null;
    const repoLabel = parsedRepo ? `${parsedRepo.owner}/${parsedRepo.repo}` : null;
    const appPath = repository?.appPath && repository.appPath !== "/" ? repository.appPath : null;
    const repoTreeUrl = repository?.url
        ? buildRepoTreeUrl(repository.url, repository.branch, appPath)
        : null;

    return (
        <Card variant="outlined">
            <Box display="flex" alignItems="center" gap={1.5} minWidth={0} sx={{ px: 2, py: 1.25 }}>
                <Box display="flex" overflow="hidden" alignItems="center" gap={1.5} minWidth={0} flexGrow={1}>
                    <SourceInfo
                        repoLabel={repoLabel}
                        appPath={appPath}
                        repoTreeUrl={repoTreeUrl}
                        branch={repository?.branch}
                        buildpackLabel={buildpackLabel}
                    />
                </Box>

                <Typography variant="body2" color="text.secondary" sx={{ flexShrink: 0 }}>
                    Latest Build:
                </Typography>

                <Box
                    component={Link}
                    to={buildsPath}
                    sx={{
                        display: "flex",
                        alignItems: "center",
                        flexShrink: 0,
                        textDecoration: "none",
                        color: "inherit",
                        px: 1,
                        py: 0.5,
                        borderRadius: 1,
                        "&:hover": { bgcolor: "action.hover" },
                    }}
                >
                    <BuildStatusInfo isBuildsLoading={isBuildsLoading} latestBuild={latestBuild} />
                </Box>
            </Box>
        </Card>
    );
};
