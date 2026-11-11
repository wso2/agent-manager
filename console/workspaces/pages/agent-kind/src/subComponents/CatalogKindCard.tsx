import React from "react";
import { ListingCard, Stack, Tooltip, Typography } from "@wso2/oxygen-ui";
import { formatRelativeTime } from "@agent-management-platform/shared-component";
import { Link } from "react-router-dom";
import { BookOpenText } from "@wso2/oxygen-ui-icons-react";
import type { AgentKindResponse } from "@agent-management-platform/types";

/** Matches ListingCard's own minimum so the loading skeletons line up with it. */
export const CARD_HEIGHT = 160;

interface CatalogKindCardProps {
    item: AgentKindResponse;
    viewPath: string;
    /** Router state for the card link. */
    viewState?: unknown;
}

export const CatalogKindCard: React.FC<CatalogKindCardProps> = ({ item, viewPath, viewState }) => {

    // The latest version's image, so people can tell which build the kind ships.
    const latest =
        item.versions?.find((v) => v.version === item.latestVersion) ?? item.versions?.[0];
    const decorators = (
        <Stack spacing={0.5}>
            <Typography variant="body2" fontWeight={600}>
                {item.latestVersion ? `Latest: ${item.latestVersion}` : "No versions"}
            </Typography>
            {latest?.imageId && (
                <Tooltip title={latest.imageId}>
                    <Typography
                        variant="caption"
                        color="text.secondary"
                        noWrap
                        sx={{ fontFamily: "monospace" }}
                    >
                        Image: {latest.imageId}
                    </Typography>
                </Tooltip>
            )}
        </Stack>
    );

    return (
        // ListingCard's own `href` renders a plain anchor, which would reload the
        // app; the router Link keeps navigation client-side.
        <Link to={viewPath} state={viewState} style={{ textDecoration: "none" }}>
            <ListingCard
                title={item.displayName}
                description={decorators}
                icon={<BookOpenText size={20} />}
                updatedLabel="Created"
                updatedTime={formatRelativeTime(item.createdAt)}
                updatedDateTime={item.createdAt}
            />
        </Link>
    );
};

export default CatalogKindCard;
