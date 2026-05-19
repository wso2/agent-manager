import React from "react";
import { Form, Stack, Tooltip, Typography } from "@wso2/oxygen-ui";
import { Link } from "react-router-dom";
import type { AgentKindResponse } from "@agent-management-platform/types";

interface CatalogKindCardProps {
    item: AgentKindResponse;
    viewPath: string;
}

export const CatalogKindCard: React.FC<CatalogKindCardProps> = ({ item, viewPath }) => {

    const description = item.description ?? "";
    const latestReleaseLabel = item.latestVersion
        ? `Latest: ${item.latestVersion}`
        : null;

    return (
        <Link to={viewPath} style={{ textDecoration: "none" }}>
            <Form.CardButton
                sx={{
                    width: "100%",
                    textAlign: "left",
                    textDecoration: "none",
                    height: 160,
                    display: "flex",
                    flexDirection: "column",
                    justifyContent: "flex-start",
                }}
            >
                <Form.CardHeader
                    title={
                        <Tooltip title={item.displayName} placement="top">
                            <Typography
                                variant="h6"
                                textOverflow="ellipsis"
                                overflow="hidden"
                                whiteSpace="nowrap"
                            >
                                {item.displayName}
                            </Typography>
                        </Tooltip>
                    }
                />
                <Form.CardContent
                    sx={{
                        width: "100%",
                        display: "flex",
                        flexDirection: "column",
                        flexGrow: 1,
                        minHeight: 0,
                    }}
                >
                    <Stack flexGrow={1} minHeight={0}>
                        <Tooltip title={description} placement="top" disableHoverListener={!description}>
                            <Typography
                                variant="caption"
                                color="text.secondary"
                                sx={{
                                    display: "-webkit-box",
                                    WebkitBoxOrient: "vertical",
                                    WebkitLineClamp: 2,
                                    overflow: "hidden",
                                    mb: 1,
                                }}
                            >
                                {description || "No description provided."}
                            </Typography>
                        </Tooltip>
                    </Stack>
                    {latestReleaseLabel && (
                        <Typography variant="caption" color="text.secondary">
                            {latestReleaseLabel}
                        </Typography>
                    )}
                </Form.CardContent>
            </Form.CardButton>
        </Link>
    );
};

export default CatalogKindCard;
