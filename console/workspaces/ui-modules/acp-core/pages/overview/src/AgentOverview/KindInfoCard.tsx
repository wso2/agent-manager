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

import { useGetAgentKind } from "@agent-management-platform/api-client";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { formatRelativeTime } from "@agent-management-platform/shared-component";
import { MarkdownView } from "@agent-management-platform/views";
import {
    Box,
    Card,
    CardContent,
    Divider,
    IconButton,
    Skeleton,
    Tooltip,
    Typography,
} from "@wso2/oxygen-ui";
import { ExternalLink, Tag } from "@wso2/oxygen-ui-icons-react";
import React from "react";
import { generatePath, Link } from "react-router-dom";
import { UppercaseCaptionLabel } from "./SectionHeader";

interface KindInfoCardProps {
    orgId: string;
    kindName: string;
    /** The kind version deployed in the selected environment. Absent when nothing
     *  is deployed there, or when the deployed image matches no published version. */
    kindVersion?: string;
}

export const KindInfoCard: React.FC<KindInfoCardProps> = ({
    orgId, kindName, kindVersion,
}) => {
    const { data: kind, isLoading } = useGetAgentKind({ orgName: orgId, kindName });

    const kindHref = generatePath(
        absoluteRouteMap.children.org.children.catalog.children.kindDetails.path,
        { orgId, kindId: kindName },
    );

    // The version shown is the one actually deployed — never the kind's newest,
    // which diverges from it the moment the kind publishes again. An environment
    // with nothing deployed has nothing to show, so the chip is omitted rather
    // than filled in with a version we can't attribute to it. versionData carries
    // that version's own metadata (its publish date), and is absent when the
    // version has since been deleted from the kind.
    const versionData = kindVersion
        ? kind?.versions?.find((v) => v.version === kindVersion)
        : undefined;

    return (
        <Card variant="outlined">
            <CardContent sx={{ py: 1.5, "&:last-child": { pb: 1.5 } }}>
                <Box
                    display="flex"
                    alignItems="center"
                    justifyContent="space-between"
                    flexWrap="wrap"
                    gap={1.5}
                    pb={1}
                    minWidth={0}
                >
                    {isLoading ? (
                        <Skeleton variant="text" width={160} />
                    ) : (
                        <>
                            <Box display="flex" alignItems="center" gap={0.5} minWidth={0}>
                                <Typography variant="h6" noWrap>
                                    Kind: {kind?.displayName ?? kindName}
                                </Typography>
                                <IconButton
                                    size="small"
                                    component={Link}
                                    to={kindHref}
                                    sx={{ p: 0.25, flexShrink: 0 }}
                                    aria-label="View Agent Kind details"
                                >
                                    <ExternalLink size={12} />
                                </IconButton>
                            </Box>

                            {kindVersion && (
                                <Box display="flex" alignItems="center" gap={1.5} minWidth={0}>
                                    <Box display="flex" alignItems="center" gap={0.5} sx={{ flexShrink: 0 }}>
                                        <Tag size={13} />
                                        <Typography variant="body2" color="text.secondary" noWrap>
                                            {kindVersion}
                                        </Typography>
                                    </Box>
                                    {versionData && (
                                        <Typography variant="body2" color="text.secondary" noWrap>
                                            {formatRelativeTime(versionData.createdAt)}
                                        </Typography>
                                    )}
                                </Box>
                            )}
                        </>
                    )}
                </Box>
                <Divider sx={{ mb: 1.5 }} />
                <Box minWidth={0}>
                    <UppercaseCaptionLabel sx={{ display: "block", mb: 0.75 }}>
                        Description
                    </UppercaseCaptionLabel>
                    {isLoading ? (
                        <Skeleton variant="text" width={200} />
                    ) : kind?.description ? (
                        <Tooltip title={kind.description} placement="bottom-start">
                            <Box
                                sx={{
                                    overflow: "hidden",
                                    display: "-webkit-box",
                                    WebkitLineClamp: 2,
                                    WebkitBoxOrient: "vertical",
                                }}>
                                <MarkdownView content={kind.description} />
                            </Box>
                        </Tooltip>
                    ) : (
                        <Typography variant="body2" color="text.secondary">
                            No description provided.
                        </Typography>
                    )}
                </Box>
            </CardContent>
        </Card>
    );
};

export default KindInfoCard;
