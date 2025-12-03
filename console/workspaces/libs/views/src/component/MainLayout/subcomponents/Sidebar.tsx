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

import { ReactNode, useState } from 'react';
import {
  Box,
  List,
  ListItemButton,
  ListItemText,
  useTheme,
  Typography,
  alpha,
  Divider,
  Collapse,
  Tooltip,
} from '@mui/material';
import { ArrowDropDownOutlined, ArrowDropUpOutlined, ChevronLeftOutlined, ChevronRightOutlined, Menu } from '@mui/icons-material';
import { NavigationItemButton } from './NavigationItemButton';
export interface NavigationItem {
  label: string;
  icon?: ReactNode;
  onClick?: () => void;
  href?: string;
  isActive?: boolean;
  type: 'item';
}
export interface NavigationSection {
  title: string;
  items: Array<NavigationItem>;
  icon?: ReactNode;
  type: 'section';
}


export interface SidebarProps {
  /** Whether the sidebar is collapsed (icons only) */
  sidebarOpen?: boolean;
  /** Callback when sidebar is toggled */
  onSidebarToggle?: () => void;
  /** Navigation sections with optional titles */
  navigationSections: Array<NavigationSection | NavigationItem>;
  /** Whether this is mobile view */
  isMobile?: boolean;
  /** Callback when navigation item is clicked */
  onNavigationClick?: () => void;
}

// Action Button Component (internal to Sidebar, for buttons without href)
interface ActionButtonProps {
  icon?: ReactNode;
  title: string;
  onClick?: () => void;
  isSelected?: boolean;
  sidebarOpen?: boolean;
  subIcon?: ReactNode;
}

function ActionButton({
  icon,
  title,
  onClick,
  isSelected = false,
  sidebarOpen = true,
  subIcon,
}: ActionButtonProps) {
  const theme = useTheme();

  return (
    <Tooltip title={title} placement="right" disableHoverListener={sidebarOpen}>
      <ListItemButton
        onClick={onClick}
        selected={isSelected}
        sx={{
          justifyContent: 'center',
          alignItems: 'center',
          height: theme.spacing(5.5),
          gap: 0.5,
          m: 0,
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
        {icon && (
          <Box sx={{ color: theme.palette.secondary.light }} display="flex" justifyContent="center" alignItems="center">
            {icon}
            {
              subIcon && (
                <Box sx={{ position: 'absolute', right: 0, top: 12, opacity: 0.5 }}>
                  {subIcon}
                </Box>
              )}
          </Box>
        )}
        {sidebarOpen && (
          <ListItemText
            primary={
              <Typography
                variant="body2"
                noWrap
                sx={{
                  color: theme.palette.secondary.light,
                }}
              >
                {title}
              </Typography>
            }
          />
        )}
      </ListItemButton>
    </Tooltip>
  );
}

export function Sidebar({
  sidebarOpen = true,
  onSidebarToggle,
  navigationSections,
  isMobile = false,
  onNavigationClick,
}: SidebarProps) {
  const theme = useTheme();
  const drawerWidth = sidebarOpen ? theme.spacing(30) : theme.spacing(8); // 240px : 64px
  const [expandedSections, setExpandedSections] = useState<Set<string>>(new Set());

  const handleSectionToggle = (sectionTitle: string) => {
    setExpandedSections((prev) => {
      const newSet = new Set(prev);
      if (prev.has(sectionTitle)) {
        newSet.delete(sectionTitle);
      } else {
        newSet.add(sectionTitle);
      }
      return newSet;
    });
  };

  const isSectionExpanded = (sectionTitle: string) => expandedSections.has(sectionTitle);

  return (
    <Box
      sx={{
        width: drawerWidth,
        transition: theme.transitions.create('width', { duration: theme.transitions.duration.short }),
        paddingTop: theme.spacing(10),
        background: `linear-gradient(45deg, ${alpha(theme.palette.secondary.main, 1)} 0%, ${alpha(theme.palette.primary.main, 1)} 100%)`,
        height: '100%',
        overflowY: 'auto',
        display: 'flex',
        flexDirection: 'column',
        justifyContent: 'space-between',
      }}
    >
      <Box sx={{
        display: 'flex',
        flexDirection: 'column',
        width: '100%',
        gap: 0.5,
      }}>
        {navigationSections.map((navItem) => (
          navItem.type === 'section' ? (
            <Box
              key={navItem.title}
              display="flex"
              flexDirection="column"
              sx={{
                backgroundColor: (navItem.items.some(item => item.isActive)
                  && !isSectionExpanded(navItem.title))
                  ? alpha(
                    theme.palette.primary.light, 0.2) : alpha(theme.palette.primary.dark, 0.1),
                outline: `1px solid ${alpha(theme.palette.primary.main, 0.1)}`,
                borderRadius: theme.spacing(1),
                mx: theme.spacing(1),
                gap: 0.5,
              }}
            >
              <ActionButton
                title={navItem.title}
                icon={navItem.icon ?? <Menu fontSize="small" />}
                onClick={() => handleSectionToggle(navItem.title)}
                sidebarOpen={sidebarOpen}
                subIcon={isSectionExpanded(navItem.title) ? <ArrowDropUpOutlined fontSize="small" /> : <ArrowDropDownOutlined fontSize="small" />}
              // isSelected={isSectionExpanded(navItem.title)}
              />
              <Collapse in={isSectionExpanded(navItem.title)} timeout="auto" unmountOnExit>
                <List
                  key={navItem.title}
                  sx={{
                    p: 0,
                    m: 0,
                    gap: 0.5,
                    display: 'flex',
                    flexDirection: 'column',
                  }}
                >
                  {navItem.items.map((item, itemIndex) => (
                    <NavigationItemButton
                      subButton
                      key={itemIndex}
                      item={item}
                      sidebarOpen={sidebarOpen}
                      isMobile={isMobile}
                      onNavigationClick={onNavigationClick}
                    />
                  ))}
                </List>
              </Collapse>
            </Box>
          ) : (
            <Box key={navItem.label} px={1}>
              <NavigationItemButton
                key={navItem.label}
                item={{
                  label: navItem.label,
                  icon: navItem.icon,
                  onClick: navItem.onClick,
                  href: navItem.href,
                  isActive: navItem.isActive,
                  type: 'item',
                }}
                sidebarOpen={sidebarOpen}
                isMobile={isMobile}
                onNavigationClick={onNavigationClick}
              />
            </Box>
          )
        ))}
      </Box>
      <Box sx={{ width: '100%', display: 'flex', flexDirection: 'column' }}>
        <Divider sx={{ backgroundColor: alpha(theme.palette.secondary.contrastText, 0.2) }} orientation="horizontal" />
        <ActionButton
          icon={sidebarOpen ? <ChevronLeftOutlined fontSize="medium" /> : <ChevronRightOutlined fontSize="small" />}
          title={sidebarOpen ? 'Collapse' : 'Expand'}
          onClick={onSidebarToggle}
          sidebarOpen={sidebarOpen}
        />
      </Box>
    </Box >

  );
}
