import { ReactNode } from 'react';
import {
  Menu,
  MenuItem,
  ListItemIcon,
  ListItemText,
  Box,
  Typography,
  Divider,
  useTheme,
} from '@wso2/oxygen-ui';
import { Link as RouterLink } from "react-router-dom"

export interface UserMenuItem {
  label: string;
  icon?: ReactNode;
  onClick?: () => void;
  href?: string;
  divider?: boolean;
}

export interface User {
  name: string;
  email?: string;
  avatar?: string;
}
export interface UserMenuProps {
  /** User information */
  user: User;
  /** User menu items */
  userMenuItems: UserMenuItem[];
  /** Menu anchor element */
  anchorEl: null | HTMLElement;
  /** Whether menu is open */
  open: boolean;
  /** Callback when menu closes */
  onClose: () => void;
}

export function UserMenu({
  user,
  userMenuItems,
  anchorEl,
  open,
  onClose,

}: UserMenuProps) {
  const theme = useTheme();

  return (
    <Menu
      id="user-menu"
      anchorEl={anchorEl}
      open={open}
      onClose={onClose}
      transformOrigin={{ horizontal: 'right', vertical: 'top' }}
      anchorOrigin={{ horizontal: 'right', vertical: 'bottom' }}
      slotProps={{
        paper: {
          sx: {
            minWidth: 24,
          }
        }
      }}
    >
      {/* User Info Header */}
      <Box sx={{ px: 2, py: 1.5, borderBottom: `1px solid ${theme.palette.divider}` }}>
        <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
          {user.name}
        </Typography>
        {user.email && (
          <Typography variant="body2" color="text.secondary">
            {user.email}
          </Typography>
        )}
      </Box>

      {/* Menu Items */}
      {userMenuItems.map((item, index) => [
        item.divider && <Divider key={`divider-${index}`} />,
        <RouterLink key={index} to={item.href ?? ''}>
          <MenuItem key={index} onClick={item.onClick} >
            {item.icon && (
              <ListItemIcon>
                {item.icon}
              </ListItemIcon>
            )}
            <ListItemText>
              <Typography variant="body2" color={theme.palette.text.primary}>{item.label}</Typography>
            </ListItemText>
          </MenuItem>
        </RouterLink>

      ]).flat()}
    </Menu>
  );
}
