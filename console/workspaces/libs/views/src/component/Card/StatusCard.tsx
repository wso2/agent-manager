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

import React from 'react';
import {
    Card,
    CardContent,
    Typography,
    Chip,
    Avatar,
    useTheme,
    alpha,
    Box,
} from '@wso2/oxygen-ui';

export interface StatusCardProps {
    /** The title/label for the status card */
    title: string;
    /** The main value to display (e.g., "v2.1.3", "47/47", "3m 42s") */
    value: string;
    /** The subtitle/description text */
    subtitle: string;
    /** The icon to display in the top-left */
    icon: React.ReactNode;
    /** The variant/color scheme for the icon */
    iconVariant?: 'primary' | 'secondary' | 'success' | 'warning' | 'error' | 'info';
    /** The tag text to display in the top-right corner */
    tag?: string;
    /** The variant/color scheme for the tag */
    tagVariant?: 'default' | 'error' | 'info' | 'success' | 'warning' | 'primary' | 'secondary';
    /** Optional click handler */
    onClick?: () => void;
    /** Additional CSS class name */
    className?: string;
    /** Whether the card is clickable */
    clickable?: boolean;
    /** The minimum width of the card */
    minWidth?: string | number;
}

export function StatusCard({
    title,
    value,
    subtitle,
    icon,
    iconVariant = 'primary',
    tag,
    tagVariant = 'default',
    onClick,
    className,
    clickable = false,
    minWidth = '400px',
}: StatusCardProps) {
    const theme = useTheme();

    const handleClick = () => {
        if (clickable && onClick) {
            onClick();
        }
    };

    // Simple color map instead of a switch
    const colorForVariant: Record<string, string> = {
        primary: theme.palette.primary.main,
        secondary: theme.palette.secondary.main,
        success: theme.palette.success.main,
        warning: theme.palette.warning.main,
        error: theme.palette.error.main,
        info: theme.palette.info.main,
    };
    const primaryColor = colorForVariant[iconVariant] || theme.palette.primary.main;

    return (
        <Card
            className={className}
            onClick={handleClick}
            sx={{
                position: 'relative',
                "&.MuiCard-root": {
                    backgroundColor: 'background.paper',
                },
                transition: theme.transitions.create(['box-shadow', 'transform'], {
                    duration: theme.transitions.duration.short,
                }),
                cursor: clickable ? 'pointer' : 'default',
                minWidth: minWidth,
                '&:hover': clickable ? {
                    boxShadow: theme.shadows[4],
                } : {},
            }}
        >
            <CardContent>
                {tag && (
                    <Chip
                        label={tag}
                        color={tagVariant}
                        size="small"
                        variant="outlined"
                        sx={{
                            position: 'absolute',
                            top: 8,
                            right: 8,
                        }}
                    />
                )}
                <Box display="flex" alignItems="center" gap={2}>
                    <Avatar
                        sx={{
                            width: 64,
                            height: 64,
                            "&.MuiAvatar-root": {
                                color: primaryColor,
                                backgroundColor: alpha(primaryColor, 0.1),
                            },
                        }}
                    >
                        {icon}
                    </Avatar>
                    <Box flexDirection="column" display="flex" gap={0.5}>
                        <Typography variant="caption">{title}</Typography>
                        <Typography variant="h5">{value}</Typography>
                        <Typography variant="caption">{subtitle}</Typography>
                    </Box>
                </Box>
            </CardContent>
        </Card>
    );
}
