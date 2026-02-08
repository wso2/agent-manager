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
  Avatar,
  Box,
  ButtonBase,
  ColorSchemeToggle,
  ComplexSelect,
  Divider,
  Header,
  Typography,
} from '@wso2/oxygen-ui';
import { ChevronDown } from '@wso2/oxygen-ui-icons-react';
import { User } from './UserMenu';
import { Link } from 'react-router-dom';
import { Logo } from '../../Logo/Logo';

export interface HeaderSelectOption {
  id: string;
  label: string;
  typeLabel?: string;
  avatar?: ReactNode;
  icon?: ReactNode;
  description?: string;
}

export interface HeaderSelectProps {
  label: string;
  onChange: (value: string) => void;
  options: HeaderSelectOption[];
  selectedId?: string;
}

export interface NavBarToolbarProps {
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
  /** Header select props */
  headerSelects?: HeaderSelectProps[];
  /** Home path */
  homePath?: string;
  /** Whether to disable user menu */
  disableUserMenu?: boolean;
}

export function NavBarToolbar({
  disableUserMenu,
  rightElements,
  sidebarOpen,
  onSidebarToggle,
  user,
  onUserMenuOpen,
  headerSelects,
  homePath,
}: NavBarToolbarProps) {
  return (
    <Header>
      {onSidebarToggle && (
        <Header.Toggle collapsed={!sidebarOpen} onToggle={onSidebarToggle} />
      )}
      <Header.Brand>
        <Header.BrandLogo>
          <ButtonBase
            sx={{
              display: 'flex',
              alignItems: 'center',
              gap: 2,
            }}
            component={Link}
            to={homePath ?? '/'}
          >
            <Logo width={200} />
          </ButtonBase>
        </Header.BrandLogo>
      </Header.Brand>
     
      <Header.Switchers showDivider={false}>
        {headerSelects?.map((selectProps) => {
          const { label, options, selectedId, onChange } = selectProps;
          if (options.length === 0) {
            return null;
          }
          const currentValue = selectedId ?? '';
          const selectedOption = options.find(
            (option) => option.id === currentValue
          );
          if (!currentValue) {
            return null;
          }
          return (
            <ComplexSelect
              key={label}
              size="small"
              value={currentValue}
              label={label}
              onChange={(event) => {
                const value = event.target.value as string;
                onChange(value);
              }}
              renderValue={() => {
                const primaryText = selectedOption?.label ?? `Select ${label}`;
                return (
                  <Box display="flex" alignItems="center" gap={1}>
                    {selectedOption?.icon && (
                      <ComplexSelect.MenuItem.Icon>
                        {selectedOption.icon}
                      </ComplexSelect.MenuItem.Icon>
                    )}
                    
                    {selectedOption?.avatar && (
                      <ComplexSelect.MenuItem.Avatar>
                        {selectedOption.avatar}
                      </ComplexSelect.MenuItem.Avatar>
                    )}
                    <Box display="flex" flexDirection="column">
                      <Typography variant="caption" color="text.secondary">
                        {label}
                      </Typography>
                      <ComplexSelect.MenuItem.Text
                        primary={primaryText}
                        secondary={
                          selectedOption?.description ??
                          selectedOption?.typeLabel ?? 'No  Description'
                        }
                      />
                    </Box>
                  </Box>
                );
              }}
              sx={{ minWidth: 200 }}
            >
              <ComplexSelect.ListHeader>{label}</ComplexSelect.ListHeader>
              {options.map((option) => (
                <ComplexSelect.MenuItem key={option.id} value={option.id}>
                  {option.icon && (
                    <ComplexSelect.MenuItem.Icon>
                      {option.icon}
                    </ComplexSelect.MenuItem.Icon>
                  )}
                  <ComplexSelect.MenuItem.Text
                    primary={option.label}
                    secondary={option.description ?? option.typeLabel}
                  />
                </ComplexSelect.MenuItem>
              ))}
            </ComplexSelect>
          );
        })}
      </Header.Switchers>
      <Header.Spacer />
      <Header.Actions>
        {rightElements && (
          <Box sx={{ mr: 2, display: 'flex', alignItems: 'center' }}>
            {rightElements}
          </Box>
        )}
        <ColorSchemeToggle />
        {user && (
          <Divider
            orientation="vertical"
            flexItem
            sx={{ mx: 1, display: { xs: 'none', sm: 'block' } }}
          />
        )}
        {user && (
          <ButtonBase
            onClick={onUserMenuOpen}
            disabled={disableUserMenu}
            sx={{
              padding: 1,
              borderRadius: 1,
            }}
          >
            <Box display="flex" alignItems="center" gap={1}>
              {user.avatar ? (
                <Avatar
                  src={user.avatar}
                  alt={user.name}
                  variant="circular"
                  sx={{
                    height: 40,
                    width: 40,
                  }}
                />
              ) : (
                <Avatar
                  variant="circular"
                  color="primary"
                  sx={{
                    height: 40,
                    width: 40,
                  }}
                >
                  {user.name
                    .split(' ')
                    .map((name) => name.charAt(0).toUpperCase())
                    .join('')}
                </Avatar>
              )}
              <Typography variant="caption" color="text.secondary">
                <ChevronDown size={16} />
              </Typography>
            </Box>
          </ButtonBase>
        )}
      </Header.Actions>
    </Header>
  );
}
