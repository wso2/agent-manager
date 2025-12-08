import { Drawer, useTheme, DrawerProps } from "@wso2/oxygen-ui";
import { ReactNode } from "react";

export interface DrawerWrapperProps extends Omit<DrawerProps, "children"> {
  children: ReactNode;
  width?: number;
}

export function DrawerWrapper({
  children,
  width = 600,
  sx,
  ...drawerProps
}: DrawerWrapperProps) {
  const theme = useTheme();

  return (
    <Drawer
      anchor="right"
      variant="temporary"
      {...drawerProps}
      sx={[
        {
          "& .MuiDrawer-paper": {
            width,
            px: 2,
            py: 1,
            overflow: "visible",
            backgroundColor: "background.paper",
            borderRadius: 0,
          },
          zIndex: theme.zIndex.drawer + 2,
        },
        ...(Array.isArray(sx) ? sx : sx ? [sx] : []),
      ]}
    >
      {children}
    </Drawer>
  );
}

