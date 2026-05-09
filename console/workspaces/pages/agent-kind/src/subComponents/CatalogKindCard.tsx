import React from "react";
import { Chip, Form, Stack, Tooltip, Typography } from "@wso2/oxygen-ui";
import { Link } from "react-router-dom";
import type { CatalogItem } from "../catalog.mock";
import { getLatestVersion } from "../catalog.mock";

interface CatalogKindCardProps {
    item: CatalogItem;
    viewPath: string;
}

const MAX_TAGS = 2;
const MAX_DESC_LENGTH = 180;

export const CatalogKindCard: React.FC<CatalogKindCardProps> = ({ item, viewPath }) => {

    const latestVersion = getLatestVersion(item);
    const latestDescription = latestVersion?.description ?? "";
    const latestReleaseLabel = latestVersion
        ? `Latest Release: ${latestVersion.versionKey} (${new Date(latestVersion.releaseDate).toLocaleDateString("en-US", {
              year: "numeric",
              month: "short",
              day: "numeric",
          })})`
        : null;
    const truncatedDesc =
        latestDescription.length > MAX_DESC_LENGTH
            ? `${latestDescription.slice(0, MAX_DESC_LENGTH)}...`
            : latestDescription;

    const visibleTags = item.tags.slice(0, MAX_TAGS);
    const remainingTagCount = item.tags.length - MAX_TAGS;

    const descriptionEl = (
        <Typography
            variant="caption"
            color="text.secondary"
            sx={{ display: "block", mb: 1 }}
        >
            {truncatedDesc}
        </Typography>
    );

    return (
        <Link to={viewPath} style={{ textDecoration: "none" }}>
            <Form.CardButton
                sx={{
                    width: "100%",
                    textAlign: "left",
                    textDecoration: "none",
                    height: 200,
                    display: "flex",
                    flexDirection: "column",
                    justifyContent: "flex-start",
                }}
            >
                <Form.CardHeader
                    title={
                        <Form.Stack direction="column" spacing={1}>
                            <Tooltip title={item.title} placement="top">
                                <Stack direction="row" spacing={1} alignItems="center">
                                    <Typography
                                        variant="h6"
                                        textOverflow="ellipsis"
                                        overflow="hidden"
                                        whiteSpace="nowrap"
                                    >
                                        {item.title}
                                    </Typography>
                                </Stack>
                            </Tooltip>
                            {visibleTags.length > 0 && (
                                <Form.Stack direction="row" spacing={1} alignItems="center">
                                    {visibleTags.map((tag) => (
                                        <Chip key={tag} size="small" label={tag} variant="outlined" />
                                    ))}
                                    {remainingTagCount > 0 && (
                                        <Tooltip title={item.tags.join(", ")} placement="top">
                                            <Typography variant="caption" noWrap color="text.secondary">
                                                {`+${remainingTagCount} more`}
                                            </Typography>
                                        </Tooltip>
                                    )}
                                </Form.Stack>
                            )}
                        </Form.Stack>
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
                    {latestDescription.length > MAX_DESC_LENGTH ? (
                        <Tooltip title={latestDescription} placement="top">
                            {descriptionEl}
                        </Tooltip>
                    ) : (
                        descriptionEl
                    )}
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
