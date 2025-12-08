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
    alpha,
    Box,
    ButtonBase,
    Typography,
    useTheme
} from '@wso2/oxygen-ui';
import {
    Home as HomeIcon,
    MessageCircle as ChatBubbleIcon,
    Target as TargetIcon,
} from '@wso2/oxygen-ui-icons-react';
import { Link } from 'react-router-dom';

// Navigation link interface
export interface NavLink {
    id: string;
    label: string;
    icon: React.ReactNode;
    isActive: boolean;
    path: string;
}
export interface GroupNavLinks {
    id: string;
    label?: string;
    icon?: React.ReactNode;
    navLinks: NavLink[];
}

// Component props interface
export interface SubTopNavBarProps {
    // Left side navigation links
    navLinks?: GroupNavLinks[];
    // Right side action buttons (React nodes)
    actionButtons?: React.ReactNode;
    // Contextual information
    framework?: string;
    version?: string;
    lastUpdated?: string;
}



// Mock data for navigation links based on image description
const defaultNavLinks: GroupNavLinks[] = [
    {
        id: 'overview',
        label: 'Overview',
        icon: <HomeIcon />,
        navLinks: [
            {
                id: 'overview',
                label: 'Overview',
                icon: <HomeIcon />,
                isActive: true,
                path: '/overview'
            },
            {
                id: 'Try Out',
                label: 'Try Out',
                icon: <ChatBubbleIcon />,
                isActive: false,
                path: '/Try Out'
            },
            {
                id: 'evaluate',
                label: 'Evaluate',
                icon: <TargetIcon />,
                isActive: false,
                path: '/evaluate'
            }
        ]
    }
];

export const SubTopNavBar: React.FC<SubTopNavBarProps> = ({
    navLinks = defaultNavLinks,
    actionButtons,
}) => {
    const theme = useTheme();
    return (
        <Box
            sx={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                borderRadius: theme.shape.borderRadius,
                p: 0.25,
                mt: 1,
            }}
        >
            <Box display="flex" gap={1}>
                {
                    navLinks.map((group: GroupNavLinks) => (
                        <Box key={group.id}
                            bgcolor={theme.palette.background.paper}
                            border={`1px solid ${theme.palette.divider}`}
                            display="flex" alignItems="center"
                            overflow="hidden"
                            borderRadius={theme.shape.borderRadius}
                        >
                            {group.navLinks.map((link: NavLink) => (
                                <ButtonBase
                                    key={link.id}
                                    component={Link}
                                    to={link.path}
                                    sx={{
                                        textDecoration: 'none',
                                        p: 0.5,
                                        // borderRadius: 1,
                                        background: link.isActive ?
                                            alpha(theme.palette.primary.main, 0.2) :
                                            theme.palette.background.paper,
                                        color: link.isActive ?
                                            theme.palette.primary.main :
                                            theme.palette.text.secondary,
                                        '&:hover': {
                                            opacity: 0.8,
                                        },
                                    }}
                                >
                                    <Box display="flex" alignItems="center" px={1} gap={1}>
                                        <Box display="flex">
                                            {link.icon}
                                        </Box>
                                        <Typography variant="body2">
                                            {link.label}
                                        </Typography>
                                    </Box>
                                </ButtonBase>
                            ))}
                        </Box>
                    ))
                }
            </Box>
            {/* Right side - Action Buttons */}
            {actionButtons && (
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    {actionButtons}
                </Box>
            )}

        </Box>
    );
};
