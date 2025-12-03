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

import React, { ReactNode } from 'react';
import {
  AppBar,
  Typography,
  IconButton,
  Box,
  Avatar,
  useTheme,
  Collapse,
  ButtonBase,
  Divider,
} from '@mui/material';
import {
  Notes,
  MenuOpen,
  KeyboardArrowDown,
} from '@mui/icons-material';
import { User } from './UserMenu';
import { TopSelecter, TopSelecterProps } from './TopSelecter';
import { Link } from 'react-router-dom';

export interface NavBarToolbarProps {
  /** App logo component or text */
  logo?: ReactNode;
  /** Whether to show the app title */
  showAppTitle?: boolean;
  /** Whether the sidebar is collapsed (icons only) */
  sidebarOpen?: boolean;
  /** Whether this is mobile view */
  isMobile?: boolean;
  /** Elements to display on the left side of the toolbar */
  leftElements?: ReactNode;
  /** Elements to display on the right side of the toolbar */
  rightElements?: ReactNode;
  /** User information for the user menu */
  user?: User;
  /** Callback when mobile drawer toggle is clicked */
  onMobileDrawerToggle?: () => void;
  /** Callback when sidebar toggle is clicked */
  onSidebarToggle?: () => void;
  /** Callback when user menu is opened */
  onUserMenuOpen?: (event: React.MouseEvent<HTMLElement>) => void;
  /** Top selectors Props */
  topSelectorsProps?: TopSelecterProps[];
  /** Home path */
  homePath?: string;
}

export function NavBarToolbar({
  logo,
  showAppTitle = true,
  sidebarOpen = false,
  isMobile = false,
  leftElements,
  rightElements,
  user,
  onSidebarToggle,
  onUserMenuOpen,
  topSelectorsProps,
  homePath,
}: NavBarToolbarProps) {
  const theme = useTheme();
  return (
    <AppBar
      position="fixed"
      sx={{
        zIndex: theme.zIndex.drawer + 1,
        transition: theme.transitions.create(['width', 'margin'], {
          easing: theme.transitions.easing.sharp,
          duration: theme.transitions.duration.enteringScreen,
        }),
      }}
    >
      <Box sx={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        width: '100%',
        paddingRight: theme.spacing(1),
        height: theme.spacing(8),
      }}>
        {/* sidebar toggle button */}

        <Box
          // width={theme.spacing(30)}
          paddingRight={theme.spacing(1)}
          sx={{
            display: 'flex',
            alignItems: 'center',
            height: '100%',
          }}>
          {isMobile && (
            <IconButton
              color="inherit"
              aria-label="toggle sidebar"
              onClick={onSidebarToggle}
              sx={{ mr: theme.spacing(2) }}
            >
              {sidebarOpen ? <MenuOpen color="secondary" /> : <Notes color="secondary" />}
            </IconButton>
          )}

          <ButtonBase
            sx={{
              display: 'flex',
              alignItems: 'center', gap: theme.spacing(2),
              padding: theme.spacing(1),
              marginY: theme.spacing(1),
              borderRadius: theme.spacing(1),
              "&:hover": {
                backgroundColor: theme.palette.action.hover,
              },
            }}
            component={Link}
            to={homePath ?? '/'}
          >
            {logo && (
              <Box sx={{ display: 'flex', alignItems: 'center' }}>
                {logo}
              </Box>
            )}
            {/* App Title (only show when sidebar is collapsed, hide on mobile) */}

            <Collapse
              in={showAppTitle}
              mountOnEnter
              unmountOnExit
              orientation="horizontal"
            >
              <Box display="flex" flexDirection="column" alignItems="flex-start">
                <Typography variant="caption" color="text.secondary">
                  WSO2
                </Typography>
                <Typography
                  variant="h5"
                  noWrap
                  color="primary"
                >
                  AI Agent Manager
                </Typography>
              </Box>
            </Collapse>
          </ButtonBase>
          <Divider orientation="vertical" flexItem />
        </Box>
        <Box display="flex" alignItems="center" gap={theme.spacing(1)}>
          {
            topSelectorsProps?.map((tsProps) => (
              <TopSelecter key={tsProps.label} {...tsProps} />
            ))
          }
        </Box>

        {/* Left Elements */}
        {leftElements && (
          <Box sx={{ ml: theme.spacing(2), display: 'flex', alignItems: 'center' }}>
            {leftElements}
          </Box>
        )}

        {/* Spacer */}
        <Box sx={{ flexGrow: 1 }} />

        {/* Right Elements */}
        {rightElements && (
          <Box sx={{ mr: theme.spacing(2), display: 'flex', alignItems: 'center' }}>
            {rightElements}
          </Box>
        )}

        {/* User Menu */}
        {user && (
          <ButtonBase
            onClick={onUserMenuOpen}
            sx={{
              "&:hover": {
                backgroundColor: theme.palette.action.hover,
              },
              padding: theme.spacing(1),
              borderRadius: theme.spacing(1),
            }}
          >
            <Box display="flex" alignItems="center" gap={theme.spacing(1)}>
              {user.avatar ? (
                <Avatar
                  src={user.avatar}
                  alt={user.name}
                  sx={{
                    padding: 1,
                    background: `linear-gradient(45deg, ${theme.palette.primary.main} 30%, ${theme.palette.secondary.main} 90%)`,
                    color: theme.palette.primary.contrastText,
                    fontWeight: 'bold'
                  }}
                />
              ) : (
                <Avatar sx={{
                  padding: 1,
                  background: `linear-gradient(45deg, ${theme.palette.primary.main} 30%, ${theme.palette.secondary.main} 90%)`,
                  color: theme.palette.primary.contrastText,
                  fontWeight: 'bold'
                }}>
                  {user.name.split(' ').map(name => name.charAt(0).toUpperCase()).join('')}
                </Avatar>
              )}
              <Box display="flex" flexDirection="column" textAlign="left">
                <Typography variant="body2" fontWeight={600} color={theme.palette.text.primary}>
                  {user.name}
                </Typography>
                <Typography variant="caption">
                  {user.email}
                </Typography>
              </Box>
              <KeyboardArrowDown color="secondary" fontSize='small' />
            </Box>

          </ButtonBase>
        )}
      </Box>
    </AppBar>
  );
}
