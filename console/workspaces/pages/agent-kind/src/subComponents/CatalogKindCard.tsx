import React from "react";
import { Form, Stack, Tooltip, Typography } from "@wso2/oxygen-ui";
import { Link } from "react-router-dom";
import type { AgentKindResponse } from "@agent-management-platform/types";

interface CatalogKindCardProps {
    item: AgentKindResponse;
    viewPath: string;
}

const MAX_DESC_LENGTH = 180;

export const CatalogKindCard: React.FC<CatalogKindCardProps> = ({ item, viewPath }) => {

    const description = item.description ?? "";
    const latestReleaseLabel = item.latestVersion
        ? `Latest: ${item.latestVersion}`
        : null;
    const truncatedDesc =
        description.length > MAX_DESC_LENGTH
            ? `${description.slice(0, MAX_DESC_LENGTH)}...`
            : description;

    const descriptionEl = (
        <Typography
            variant="caption"
            color="text.secondary"
            sx={{ display: "block", mb: 1 }}
        >
            {truncatedDesc || "No description provided."}
        </Typography>
    );

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
                        {description.length > MAX_DESC_LENGTH ? (
                            <Tooltip title={description} placement="top">
                                {descriptionEl}
                            </Tooltip>
                        ) : (
                            descriptionEl
                        )}
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
