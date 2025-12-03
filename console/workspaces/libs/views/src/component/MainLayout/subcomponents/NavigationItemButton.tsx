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
    ListItemButton,
    ListItemIcon,
    ListItemText,
    useTheme,
    Typography,
    alpha,
    Tooltip,
} from '@mui/material';
import { Link as RouterLink } from "react-router-dom";
import { NavigationItem } from './Sidebar';

export interface NavigationItemButtonProps {
    item: NavigationItem;
    sidebarOpen: boolean;
    isMobile?: boolean;
    onNavigationClick?: () => void;
    subButton?: boolean;
}

export function NavigationItemButton({
    item,
    sidebarOpen,
    isMobile = false,
    onNavigationClick,
    subButton = false,
}: NavigationItemButtonProps) {
    const theme = useTheme();

    return (
        <Tooltip title={item.label} placement='right' disableHoverListener={sidebarOpen}>
            <ListItemButton
                onClick={() => {
                    if ('onClick' in item && item.onClick) {
                        item.onClick();
                    }
                    if (isMobile) {
                        onNavigationClick?.();
                    }
                }}
                component={item.href ? RouterLink : 'div'}
                to={item.href ?? ''}
                selected={item.isActive}
                sx={{
                    justifyContent: 'start',
                    alignItems: 'center',
                    height: theme.spacing(5.5),
                    //   p: theme.spacing(1),
                    // p:0,
                    m:0,
                    pl: (subButton && sidebarOpen) ? theme.spacing(5) : theme.spacing(1.75),
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
                {sidebarOpen && (
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
                )}
            </ListItemButton>
        </Tooltip>
    );
}

