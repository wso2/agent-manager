import { ReactNode, useState } from 'react';
import {
  Box,
  List,
  ListItemButton,
  ListItemText,
  useTheme,
  Collapse,
  Tooltip,
  Skeleton,
  Layout,
  ListItemIcon,
} from '@wso2/oxygen-ui';
import {
  ChevronDown as ArrowDropDownOutlined,
  ChevronUp as ArrowDropUpOutlined,
  ChevronLeft as ChevronLeftOutlined,
  ChevronRight as ChevronRightOutlined,
  Menu,
} from '@wso2/oxygen-ui-icons-react';
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
  /** Drawer width */
  drawerWidth: number | string;
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
  return (
    <Tooltip title={title} placement="right" disableHoverListener={sidebarOpen}>
      <ListItemButton
        onClick={onClick}
        selected={isSelected}
        sx={{
          px: 1.5,
          height: 44,
        }}
      >
        {icon && (
          <ListItemIcon sx={{ minWidth: 40 }}>
            {icon}
            {subIcon && (
              <Box
                sx={{ position: 'absolute', right: 6, top: 12, opacity: 0.5 }}
              >
                {subIcon}
              </Box>
            )}
          </ListItemIcon>
        )}
        {sidebarOpen && (
          <ListItemText
            sx={{
              textWrap: 'nowrap',
            }}
            primary={title}
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
  drawerWidth,
}: SidebarProps) {
  const theme = useTheme();
  const [expandedSections, setExpandedSections] = useState<Set<string>>(
    new Set()
  );

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

  const isSectionExpanded = (sectionTitle: string) =>
    expandedSections.has(sectionTitle);

  return (
    <Layout.Sidebar
      sx={{
        width: drawerWidth,
        pt: 1,
        px: 1,
        display: 'flex',
        borderRight: 1, 
        borderColor: 'divider' ,
        justifyContent: 'space-between',
        flexDirection: 'column',
        bgcolor: 'background.default',
        transition: theme.transitions.create('all', {
          duration: theme.transitions.duration.short,
        }),
      }}
    >
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          width: '100%',
          gap: 1,
        }}
      >
        {navigationSections.length === 0 && (
          <Box display="flex" flexDirection="column" gap={1}>
            <Skeleton
              variant="rounded"
              height={44}
              width="100%"
            />
            <Skeleton
              variant="rounded"
              height={44}
              width="100%"
            />
            <Skeleton
              variant="rounded"
              height={44}
              width="100%"
            />
          </Box>
        )}
        {navigationSections.map((navItem) =>
          navItem.type === 'section' ? (
            <Box
              key={navItem.title}
              display="flex"
              flexDirection="column"
              sx={{
                borderRadius: 0.5,
              }}
            >
              <ActionButton
                title={navItem.title}
                icon={navItem.icon ?? <Menu size={16} />}
                onClick={() => handleSectionToggle(navItem.title)}
                sidebarOpen={sidebarOpen}
                subIcon={
                  isSectionExpanded(navItem.title) ? (
                    <ArrowDropUpOutlined size={16} />
                  ) : (
                    <ArrowDropDownOutlined size={16} />
                  )
                }
                isSelected={
                  navItem.items.some((item) => item.isActive) &&
                  !isSectionExpanded(navItem.title)
                }
              />
              <Collapse
                in={isSectionExpanded(navItem.title)}
                timeout="auto"
                unmountOnExit
              >
                <List
                  key={navItem.title}
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
       
          )
        )}
      </Box>
      <Box sx={{ width: '100%', display: 'flex', flexDirection: 'column' }}>
        <ActionButton
          icon={
            sidebarOpen ? (
              <ChevronLeftOutlined fontSize="medium" />
            ) : (
              <ChevronRightOutlined fontSize="small" />
            )
          }
          title={sidebarOpen ? 'Collapse' : 'Expand'}
          onClick={onSidebarToggle}
          sidebarOpen={sidebarOpen}
        />
      </Box>
    </Layout.Sidebar>
  );
}
