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

import React, { useState, ReactNode } from 'react';
import {
    Drawer,
    useTheme,
    useMediaQuery,
    Box,
} from '@mui/material';
import {
    Sidebar,
    MobileDrawer,
    UserMenu,
    NavBarToolbar,
    User,
    NavigationSection as NavigationSectionType,
    UserMenuItem as UserMenuItemType,
    NavigationItem
} from './subcomponents';
import { TopSelecterProps } from './subcomponents/TopSelecter';


export interface MainLayoutProps {
    /** App logo component or text */
    logo?: ReactNode;
    /** User information for the user menu */
    user?: User;
    /** Navigation items for mobile drawer */
    navigationItems?: Array<NavigationSectionType | NavigationItem>;
    /** User menu items */
    userMenuItems?: Array<UserMenuItemType>;
    /** Elements to display on the left side of the toolbar */
    leftElements?: ReactNode;
    /** Elements to display on the right side of the toolbar */
    rightElements?: ReactNode;
    /** Whether to show the app title */
    showAppTitle?: boolean;
    /** Whether the sidebar is collapsed (icons only) */
    sidebarCollapsed?: boolean;
    /** Callback when sidebar collapse state changes */
    onSidebarToggle?: (collapsed: boolean) => void;
    /** Children to display inside the main content area */
    children?: ReactNode;
    /** Top selectors Props */
    topSelectorsProps?: TopSelecterProps[];
    /** Home path */
    homePath?: string;
}

export function MainLayout({
    logo,
    user,
    navigationItems = [],
    userMenuItems = [],
    leftElements,
    rightElements,
    showAppTitle = true,
    sidebarCollapsed = true,
    onSidebarToggle,
    children,
    topSelectorsProps,
    homePath,
}: MainLayoutProps) {
    const theme = useTheme();
    const isMobile = useMediaQuery(theme.breakpoints.down('md'));
    const [userMenuAnchor, setUserMenuAnchor] = useState<null | HTMLElement>(null);
    const [sidebarOpen, setSidebarOpen] = useState(!sidebarCollapsed);


    const handleSidebarToggle = () => {
        const newState = !sidebarOpen;
        setSidebarOpen(newState);
        onSidebarToggle?.(!newState);
    };

    const handleUserMenuOpen = (event: React.MouseEvent<HTMLElement>) => {
        setUserMenuAnchor(event.currentTarget);
    };

    const handleUserMenuClose = () => {
        setUserMenuAnchor(null);
    };

    const drawerWidth = sidebarOpen ? theme.spacing(30) : theme.spacing(8); // 240px : 64px

    return (
        <Box sx={{
            display: 'flex',
            flexDirection: 'column',
            height: '100vh',
            overflow: 'hidden',
            width: '100vw',
            backgroundColor: theme.palette.background.default
        }}>

            {/* Main Toolbar */}
            <NavBarToolbar
                homePath={homePath}
                logo={logo}
                showAppTitle={showAppTitle}
                sidebarOpen={sidebarOpen}
                isMobile={isMobile}
                leftElements={leftElements}
                rightElements={rightElements}
                user={user}
                onSidebarToggle={handleSidebarToggle}
                onUserMenuOpen={handleUserMenuOpen}
                topSelectorsProps={topSelectorsProps}
            />

            {/* Desktop Sidebar */}
            {!isMobile && (
                <Drawer
                    variant="permanent"
                    sx={{
                        width: drawerWidth,
                        flexShrink: 0,
                        '& .MuiDrawer-paper': {
                            width: drawerWidth,
                            boxSizing: 'border-box',
                            transition: theme.transitions.create('width', {
                                easing: theme.transitions.easing.easeIn,
                                duration: theme.transitions.duration.enteringScreen,
                            }),
                        },
                    }}
                >
                    <Sidebar
                        onSidebarToggle={handleSidebarToggle}
                        sidebarOpen={sidebarOpen}
                        navigationSections={navigationItems}
                        isMobile={isMobile}
                        onNavigationClick={() => handleSidebarToggle()}
                    />
                </Drawer>
            )}


            {/* Mobile Drawer */}
            <Drawer
                variant="temporary"
                open={sidebarOpen}
                onClose={handleSidebarToggle}
                ModalProps={{
                    keepMounted: true, // Better open performance on mobile.
                }}
                sx={{
                    display: { xs: 'block', md: 'none' },
                    '& .MuiDrawer-paper': {
                        width: theme.spacing(31.25), // 250px
                        borderRight: `none`,
                    },
                }}
            >
                <MobileDrawer
                    navigationSections={navigationItems}
                    onNavigationClick={() => handleSidebarToggle()}
                />
            </Drawer>

            {/* User Menu */}
            {user && (
                <UserMenu
                    user={user}
                    userMenuItems={userMenuItems}
                    anchorEl={userMenuAnchor}
                    open={Boolean(userMenuAnchor)}
                    onClose={handleUserMenuClose}
                />
            )}

            {/* Main Content Area */}
            <Box position="relative" height="100%" sx={{ ml: (sidebarOpen ? theme.spacing(30) : (isMobile ? 0 : theme.spacing(8))), }}>
                <Box sx={{
                    flexGrow: 1,
                    mt: theme.spacing(8),
                    p: theme.spacing(0.5),
                    display: 'flex',
                    height: 'calc(100vh - 64px)',
                    // height: '100%',
                    // flexGrow: 1,
                    overflow: 'auto',
                }}>
                    {children}
                </Box>
            </Box>
        </Box>
    );
}
