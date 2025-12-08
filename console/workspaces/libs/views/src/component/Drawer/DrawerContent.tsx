import { Box } from "@wso2/oxygen-ui";
import { ReactNode } from "react";

export interface DrawerContentProps {
  children: ReactNode;
}

export function DrawerContent({ children }: DrawerContentProps) {
  return (
    <Box
      display="flex"
      flexDirection="column"
      gap={2}
      pt={1}
      sx={{ flex: 1, overflow: "visible" }}
    >
      {children}
    </Box>
  );
}

