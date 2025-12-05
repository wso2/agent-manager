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

import {
  Box,
  List,
  ListItem,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  useTheme,
  Typography,
  alpha,
  Tooltip,
} from '@mui/material';
import { NavigationItem, NavigationSection } from './Sidebar';
import { Link as RouterLink } from "react-router-dom"


export interface MobileDrawerProps {
  /** Navigation sections with optional titles */
  navigationSections: Array<NavigationSection | NavigationItem>;
  /** Callback when navigation item is clicked */
  onNavigationClick?: () => void;
}

export function MobileDrawer({
  navigationSections,
  onNavigationClick,
}: MobileDrawerProps) {
  const theme = useTheme();

  return (
    <Box
      sx={{
        paddingTop: theme.spacing(10),
        background: `linear-gradient(45deg, ${alpha(theme.palette.secondary.main, 1)} 0%, ${alpha(theme.palette.primary.main, 1)} 100%)`,
        height: '100vh',
        overflowY: 'auto',
      }}
    >
      {navigationSections.map((navItem, sectionIndex) => (
        navItem.type === 'section' ? (
        <Box key={sectionIndex} sx={{ mb: sectionIndex < navigationSections.length - 1 ? 3 : 0 }}>
          <List
            subheader={
              navItem.title && 
              <Typography variant="caption" pl={theme.spacing(2)} sx={{ color: theme.palette.secondary.light }}>
                {navItem.title}
              </Typography>
            }
          >
            {navItem.items.map((item, itemIndex) => (
              <ListItem key={itemIndex} disablePadding>
                <ListItemButton
                  onClick={() => {
                    if ('onClick' in item && item.onClick) {
                      item.onClick();
                    }
                    onNavigationClick?.();
                  }}
                  component={item.href ? RouterLink : 'div'}
                  to={item.href ?? ''}
                  selected={item.isActive}
                  sx={{
                    justifyContent: 'start',
                    alignItems: 'center',
                    height: theme.spacing(5.5),
                    p: theme.spacing(1),
                    pl: theme.spacing(1.75),
                    mb: theme.spacing(1),
                    transition: theme.transitions.create('all', { duration: theme.transitions.duration.short }),
                    '&.Mui-selected': {
                      backgroundColor: alpha(theme.palette.secondary.light, 0.4),
                      '&:hover': {
                        backgroundColor: alpha(theme.palette.secondary.light, 0.3),
                        opacity: 1,
                      },
                    },
                    '&:hover': {
                      backgroundColor: alpha(theme.palette.secondary.light, 0.2),
                      opacity: 1,
                    },
                  }}
                >
                  {item.icon && (
                    <ListItemIcon
                      sx={{
                        minWidth: theme.spacing(3),
                        color: item.isActive ?
                          theme.palette.primary.contrastText :
                          theme.palette.secondary.light,
                      }}
                    >
                      {item.icon}
                    </ListItemIcon>
                  )}
                  <ListItemText
                    primary={
                      <Typography
                        variant="body2"
                        noWrap
                        sx={{
                          color: item.isActive
                            ? theme.palette.primary.contrastText :
                            theme.palette.secondary.light,
                        }}
                      >
                        {item.label}
                      </Typography>
                    }
                  />
                </ListItemButton>
              </ListItem>
            ))}
          </List>
        </Box>
        ) : (
          <Tooltip title={navItem.label} placement='right' key={navItem.label}>
            <ListItemButton
              onClick={navItem.onClick}
              component={navItem.href ? RouterLink : 'div'}
              to={navItem.href ?? ''}
            >
            </ListItemButton>
          </Tooltip>
        )
      ))}
    </Box>
  );
}
