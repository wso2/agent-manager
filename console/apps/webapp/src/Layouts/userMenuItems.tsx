import { LogOut as LogOutIcon, Settings as SettingsIcon } from '@wso2/oxygen-ui-icons-react';

export const createUserMenuItems = (orgId: string, logout: () => void) => [
  {
    label: 'Settings',
    href: "/unknown" + orgId,
    icon: <SettingsIcon fontSize='inherit' />,
  },
  {
    label: 'Logout',
    onClick: () => logout(),
    icon: <LogOutIcon fontSize='inherit' />,
  },
];
