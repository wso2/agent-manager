import React from "react";
import { Chip, ListingCard, Stack } from "@wso2/oxygen-ui";
import { formatRelativeTime } from "@agent-management-platform/shared-component";
import { Link } from "react-router-dom";
import { BookOpenText } from "@wso2/oxygen-ui-icons-react";
import type { AgentKindResponse } from "@agent-management-platform/types";

/** Matches ListingCard's own minimum so the loading skeletons line up with it. */
export const CARD_HEIGHT = 160;

interface CatalogKindCardProps {
    item: AgentKindResponse;
    viewPath: string;
}

export const CatalogKindCard: React.FC<CatalogKindCardProps> = ({ item, viewPath }) => {
    const labels = Object.entries(item.labels ?? {});

    // The card's metadata reads as chips rather than prose: the version is the
    // thing people scan a catalog for.
    const decorators = (
        <Stack direction="row" spacing={0.5} useFlexGap flexWrap="wrap" alignItems="center">
            <Chip
                size="small"
                variant="outlined"
                label={item.latestVersion ? `v${item.latestVersion}` : "No versions"}
            />
            {labels.slice(0, 2).map(([key, value]) => (
                <Chip key={key} size="small" variant="outlined" label={`${key}: ${value}`} />
            ))}
            {labels.length > 2 && (
                <Chip size="small" variant="outlined" label={`+${labels.length - 2}`} />
            )}
        </Stack>
    );

    return (
        // ListingCard's own `href` renders a plain anchor, which would reload the
        // app; the router Link keeps navigation client-side.
        <Link to={viewPath} style={{ textDecoration: "none" }}>
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
