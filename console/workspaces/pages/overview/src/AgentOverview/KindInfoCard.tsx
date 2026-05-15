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
import { Box, Button, Card, CardContent, Skeleton, Typography } from "@wso2/oxygen-ui";
import { ExternalLink } from "@wso2/oxygen-ui-icons-react";
import React from "react";
import { generatePath, Link } from "react-router-dom";

interface KindInfoCardProps {
    orgId: string;
    kindName: string;
    kindVersion: string;
}

export const KindInfoCard: React.FC<KindInfoCardProps> = ({ orgId, kindName }) => {
    const { data: kind, isLoading } = useGetAgentKind({ orgName: orgId, kindName });

    const kindHref = generatePath(
        absoluteRouteMap.children.org.children.catalog.children.kindDetails.path,
        { orgId, kindId: kindName },
    );

    if (isLoading) {
        return <Skeleton variant="rounded" height={80} />;
    }

    return (
        <Card variant="outlined" sx={{ maxWidth: 400, minWidth: 275 }}>
            <CardContent sx={{ display: "flex", flexDirection: "column", gap: 0.75, "&:last-child": { pb: 1.5 }, pt: 1.5, px: 1.5, pb: 1.5 }}>
                <Box display="flex" flexDirection="row" gap={1} alignItems="center">
                    Agent Kind:
                    <Button
                        component={Link}
                        to={kindHref}
                        variant="text"
                        color="inherit"
                        size="small"
                        sx={{ p: 0, minWidth: 0, fontWeight: 600 }}
                        endIcon={<ExternalLink size={12} />}
                    >
                        <Typography variant="body2" fontWeight={600} noWrap>
                            {kind?.displayName ?? kindName}
                        </Typography>
                    </Button>
                </Box>

                {kind?.description && (
                    <Typography variant="caption" color="text.secondary">
                        {kind.description}
                    </Typography>
                )}

                <Box display="flex" flexDirection="row" gap={1} flexWrap="wrap">
                    {kind?.latestVersion && (
                        <Typography variant="caption" color="text.secondary">
                            Latest: v{kind.latestVersion}
                        </Typography>
                    )}
                    {kind?.versions?.length != null && (
                        <Typography variant="caption" color="text.secondary">
                            · {kind.versions.length} version{kind.versions.length !== 1 ? "s" : ""}
                        </Typography>
                    )}
                </Box>
            </CardContent>
        </Card>
    );
};

export default KindInfoCard;
