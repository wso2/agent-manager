import { Box, IconButton, Typography } from "@wso2/oxygen-ui";
import { X as Close } from "@wso2/oxygen-ui-icons-react";
import { ReactNode } from "react";

export interface DrawerHeaderProps {
  icon: ReactNode;
  title: string;
  onClose: () => void;
}

export function DrawerHeader({ icon, title, onClose }: DrawerHeaderProps) {
  return (
    <Box
      display="flex"
      flexDirection="row"
      justifyContent="space-between"
      alignItems="center"
      mb={1}
      pt={2}
    >
      <Box display="flex" flexDirection="row" alignItems="center" gap={1}>
        {icon}
        <Typography variant="h3">{title}</Typography>
      </Box>
      <IconButton size="small" onClick={onClose}>
        <Close size={16} />
      </IconButton>
    </Box>
  );
}

