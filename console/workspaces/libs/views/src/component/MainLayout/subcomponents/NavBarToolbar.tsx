import React, { ReactNode } from 'react';
import {
  Typography,
  Box,
  Avatar,
  useTheme,
  ButtonBase,
  Layout,
  ColorSchemeToggle,
  AppBar,
} from '@wso2/oxygen-ui';
import { ChevronDown } from '@wso2/oxygen-ui-icons-react';
import { User } from './UserMenu';
import { TopSelecter, TopSelecterProps } from './TopSelecter';
import { Link } from 'react-router-dom';

export function Logo() {
  const theme = useTheme();
  const brandColor = theme.palette.text.primary;
  return (
    <Box
      sx={{
        display: 'flex',
        alignItems: 'center',
        gap: 1.5,
        color: brandColor,
      }}
    >
      <Box
        sx={{
          width: theme.spacing(5),
          height: theme.spacing(5),
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          borderRadius: 0.5,
          fontSize: theme.typography.pxToRem(18),
          backgroundColor: theme.palette.primary.main,
          color: theme.palette.primary.contrastText,
        }}
      >
        AI
      </Box>
      <Box sx={{ display: 'flex', flexDirection: 'column', lineHeight: 1.1 }}>
        <Typography
          color="textSecondary"
          sx={{
            fontSize: theme.typography.pxToRem(8),
            letterSpacing: 0.2,
          }}
        >
          WSO2
        </Typography>
        <Typography
          variant="caption"
          color="textPrimary"
          fontSize={theme.typography.pxToRem(12)}
          fontWeight={600}
          sx={{ letterSpacing: 0.05 }}
        >
          Agent Management Platform
        </Typography>
      </Box>
    </Box>
  );
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
  /** Top selectors Props */
  topSelectorsProps?: TopSelecterProps[];
  /** Home path */
  homePath?: string;
}

export function NavBarToolbar({
  leftElements,
  rightElements,
  user,
  onUserMenuOpen,
  topSelectorsProps,
  homePath,
}: NavBarToolbarProps) {
  const theme = useTheme();
  return (
    <Layout.Navbar>
      <AppBar
        position="static"
        sx={{ zIndex: theme.zIndex.drawer + 1, borderRadius: 0 }}
      >
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            width: '100%',
            height: theme.spacing(8),
          }}
        >
          <Box
            paddingRight={1}
            sx={{
              display: 'flex',
              alignItems: 'center',
              height: '100%',
            }}
          >
            <ButtonBase
              sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 2,
                padding: 1,
                marginY: 1,
                borderRadius: 1,
              }}
              component={Link}
              to={homePath ?? '/'}
            >
              <Logo />
            </ButtonBase>
          </Box>
          <Box display="flex" alignItems="center" gap={1}>
            {topSelectorsProps?.map((tsProps) => (
              <TopSelecter key={tsProps.label} {...tsProps} />
            ))}
          </Box>

          {/* Left Elements */}
          {leftElements && (
            <Box sx={{ ml: 2, display: 'flex', alignItems: 'center' }}>
              {leftElements}
            </Box>
          )}

          {/* Spacer */}
          <Box sx={{ flexGrow: 1 }} />

          {/* Right Elements */}
          {rightElements && (
            <Box sx={{ mr: 2, display: 'flex', alignItems: 'center' }}>
              {rightElements}
            </Box>
          )}
          <ColorSchemeToggle/>
          {/* User Menu */}
          {user && (
            <ButtonBase
              onClick={onUserMenuOpen}
              sx={{
                padding: 1,
                borderRadius: 1,
              }}
            >
              <Box display="flex" alignItems="center" gap={1}>
                {user.avatar ? (
                  <Avatar src={user.avatar} alt={user.name} />
                ) : (
                  <Avatar>
                    {user.name
                      .split(' ')
                      .map((name) => name.charAt(0).toUpperCase())
                      .join('')}
                  </Avatar>
                )}
                <Box display="flex" flexDirection="column" textAlign="left">
                  <Typography
                    variant="body2"
                    fontWeight={600}
                    color="textPrimary"
                  >
                    {user.name}
                  </Typography>
                  <Typography variant="caption" color="textSecondary">
                    {user.email}
                  </Typography>
                </Box>
                <ChevronDown size={16} color={theme.palette.text.secondary} />
              </Box>
            </ButtonBase>
          )}
        </Box>
      </AppBar>
    </Layout.Navbar>
  );
}
