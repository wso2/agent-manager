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

import { Avatar, Box, Card, CardContent, Skeleton, Typography, useTheme } from "@wso2/oxygen-ui";
import { ChevronRight } from "@wso2/oxygen-ui-icons-react";
import { Link } from "react-router-dom";

interface ConfigListCardProps {
    avatarLabel: string;
    avatarColor: string;
    /** Provider logo (e.g. LLM provider template's metadata.logoUrl), shown
     * instead of the letter avatar when present — mirrors the logo Chip on
     * the LLM Providers listing page (LLMProviderTable.tsx). */
    avatarSrc?: string;
    title: string;
    /** Underlying provider/proxy name, shown muted next to the title. */
    providerLabel?: string;
    subtitle?: string;
    isLoadingSubtitle?: boolean;
    /** When set, the whole card becomes a link to this config's view page and
     * a trailing chevron is shown to signal it's clickable. */
    href?: string;
}

/**
 * Presentational row card shared by the Model Configs and MCP Proxies
 * previews below Invoke URL — provider logo (or a colored letter fallback),
 * config name, and an optional secondary line (e.g. guardrails summary for
 * LLM configs). Uses the same Card variant="outlined" + CardContent pairing
 * as EmptyConfigCard.tsx elsewhere in Configure Agent, instead of a
 * hand-styled Box.
 */
export const ConfigListCard: React.FC<ConfigListCardProps> = ({
    avatarLabel, avatarColor, avatarSrc, title, providerLabel, subtitle, isLoadingSubtitle, href,
}) => {
    const theme = useTheme();
    const avatarBgcolor = avatarSrc ? theme.palette.grey[100] : avatarColor;
    const avatarTextColor = theme.palette.getContrastText(avatarBgcolor);

    // A bounded, flexible tile so a group's cards wrap into a tidy grid rather
    // than one full-width row each; minWidth: 0 lets the noWrap text below
    // actually ellipsis-truncate instead of forcing the tile wider. When href
    // is set the whole card is a router Link — hence the reset of link colors
    // and the hover affordance.
    const linkProps = href
        ? {
            component: Link,
            to: href,
            sx: {
                flex: "1 1 240px",
                minWidth: 0,
                maxWidth: 340,
                textDecoration: "none",
                color: "inherit",
                display: "block",
                transition: "border-color 120ms, background-color 120ms",
                "&:hover": {
                    borderColor: "primary.main",
                    backgroundColor: "action.hover",
                },
            },
        }
        : { sx: { flex: "1 1 240px", minWidth: 0, maxWidth: 340 } };

    return (
    <Card variant="outlined" {...linkProps}>
        <CardContent sx={{ display: "flex", alignItems: "center", gap: 1.5, "&:last-child": { pb: 2 } }}>
            <Avatar
                src={avatarSrc}
                sx={{
                    bgcolor: avatarBgcolor,
                    color: avatarTextColor,
                    width: 36,
                    height: 36,
                    fontSize: 14,
                    flexShrink: 0,
                }}
            >
                {avatarLabel}
            </Avatar>
            <Box sx={{ flex: 1, minWidth: 0 }}>
                <Box display="flex" alignItems="baseline" gap={0.75} minWidth={0}>
                    <Typography variant="body2" fontWeight={600} noWrap>
                        {title}
                    </Typography>
                    {providerLabel && (
                        <Typography variant="caption" color="text.disabled" noWrap>
                            {providerLabel}
                        </Typography>
                    )}
                </Box>
                {isLoadingSubtitle ? (
                    <Skeleton variant="text" width={140} height={16} />
                ) : subtitle ? (
                    <Typography variant="caption" color="text.secondary" noWrap sx={{ display: "block" }}>
                        {subtitle}
                    </Typography>
                ) : null}
            </Box>
            {href && (
                <ChevronRight
                    size={22}
                    color={theme.palette.text.secondary}
                    style={{ flexShrink: 0 }}
                />
            )}
        </CardContent>
    </Card>
    );
};
