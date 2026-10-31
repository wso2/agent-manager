import React from "react";
import { Avatar, Box, Divider, Form, Tooltip, Typography } from "@wso2/oxygen-ui";
import { formatRelativeTime } from "@agent-management-platform/shared-component";
import { Link } from "react-router-dom";
import { BookOpenText, Tag, Clock as TimerOutlined } from "@wso2/oxygen-ui-icons-react";
import type { AgentKindResponse } from "@agent-management-platform/types";

export const CARD_HEIGHT = 116;

interface CatalogKindCardProps {
    item: AgentKindResponse;
    viewPath: string;
}

export const CatalogKindCard: React.FC<CatalogKindCardProps> = ({ item, viewPath }) => {

    const createdAtText = item.createdAt
        ? formatRelativeTime(item.createdAt)
        : "—";

    return (
        <Link to={viewPath} style={{ textDecoration: "none" }}>
            <Form.CardButton
                sx={{
                    position: "relative",
                    width: "100%",
                    height: CARD_HEIGHT,
                    textAlign: "left",
                    display: "flex",
                    flexDirection: "column",
                    p: 2,
                    pt: 3,
                    boxSizing: "border-box",
                    overflow: "hidden",
                }}
            >
                {/* Top: avatar + name + latest version */}
                <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1.5, width: "100%", minWidth: 0 }}>
                    <Avatar
                        sx={{
                            bgcolor: "secondary.main",
                            color: "primary.light",
                            height: 44,
                            width: 44,
                            flexShrink: 0,
                        }}
                    >
                        <BookOpenText size={26} />
                    </Avatar>
                    <Box sx={{ flex: 1, minWidth: 0 }}>
                        <Tooltip title={item.displayName} placement="top-start">
                            <Typography
                                variant="h5"
                                sx={{
                                    lineHeight: 1.3,
                                    mb: 0.4,
                                    overflow: "hidden",
                                    textOverflow: "ellipsis",
                                    whiteSpace: "nowrap",
                                    minWidth: 0,
                                }}
                            >
                                {item.displayName}
                            </Typography>
                        </Tooltip>
                        <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, overflow: "hidden" }}>
                            <Tag size={13} style={{ opacity: 0.5, flexShrink: 0 }} />
                            {item.latestVersion ? (
                                <Typography
                                    variant="caption"
                                    color="text.secondary"
                                    noWrap
                                    sx={{ minWidth: 0 }}
                                >
                                    Latest: {item.latestVersion}
                                </Typography>
                            ) : (
                                <Typography variant="caption" color="text.disabled" sx={{ fontStyle: "italic" }}>
                                    No versions published
                                </Typography>
                            )}
                        </Box>
                    </Box>
                </Box>

                <Box sx={{ flex: 1 }} />

                <Divider sx={{ mb: 1 }} />

                {/* Bottom: time */}
                <Box sx={{ width: "100%", minWidth: 0 }}>
                    <Typography
                        variant="caption"
                        color="text.disabled"
                        sx={{ display: "flex", alignItems: "center", gap: 0.4 }}
                    >
                        <TimerOutlined size={12} />
                        {createdAtText}
                    </Typography>
                </Box>
            </Form.CardButton>
        </Link>
    );
};

export default CatalogKindCard;
