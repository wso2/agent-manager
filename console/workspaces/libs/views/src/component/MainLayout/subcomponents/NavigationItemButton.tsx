import {
  ListItem,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Tooltip,
} from '@wso2/oxygen-ui';
import { Link as RouterLink } from 'react-router-dom';
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
  return (
    <ListItem disablePadding>
      <Tooltip
        title={item.label}
        placement="right"
        disableHoverListener={sidebarOpen}
      >
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
            pl: subButton && sidebarOpen ? 5 : 1.375,
            height: 44,
          }}
        >
          {item.icon && <ListItemIcon sx={{ minWidth: 40 }}>{item.icon}</ListItemIcon>}
          {sidebarOpen && (
            <ListItemText sx={{ textWrap: 'nowrap' }} primary={item.label} />
          )}
        </ListItemButton>
      </Tooltip>
    </ListItem>
  );
}
