import { Box, Typography, Button, List, ListItem, ListItemButton, ListItemText } from "@wso2/oxygen-ui";
import { useCallback, useEffect, useState } from "react";
import { Clock as AccessTime, GitCommit, Package, Check } from "@wso2/oxygen-ui-icons-react";
import { DrawerWrapper, DrawerHeader, DrawerContent } from "@agent-management-platform/views";
import dayjs from "dayjs";

export interface BuildSelectorDrawerProps {
  open: boolean;
  onClose: () => void;
  builds: Array<{
    buildName: string;
    commitId?: string;
    startedAt: string;
    status?: string;
  }>;
  selectedBuild: string;
  onSelectBuild: (buildName: string) => void;
}

export function BuildSelectorDrawer({
  open,
  onClose,
  builds,
  selectedBuild,
  onSelectBuild,
}: BuildSelectorDrawerProps) {
  const [tempSelectedBuild, setTempSelectedBuild] =
    useState<string>(selectedBuild);

  // Update temp selection when drawer opens (only when transitioning from closed to open)
  useEffect(() => {
    if (open) {
      setTempSelectedBuild(selectedBuild);
    }
  }, [open, selectedBuild]);

  const handleConfirmSelection = useCallback(() => {
    if (tempSelectedBuild) {
      onSelectBuild(tempSelectedBuild);
    }
  }, [tempSelectedBuild, onSelectBuild]);

  const handleBuildClick = useCallback((buildName: string) => {
    setTempSelectedBuild(buildName);
  }, []);

  return (
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader
        icon={<Package size={24} />}
        title="Select Build"
        onClose={onClose}
      />
      <DrawerContent>
        <Typography variant="body2" color="text.secondary">
          Choose a build to deploy. Only completed builds are available.
        </Typography>
        <Box>
          <List disablePadding sx={{ gap: 1, display: 'flex', flexDirection: 'column' }}>
            {builds.length === 0 ? (
              <Box
                display="flex"
                justifyContent="center"
                alignItems="center"
                minHeight={200}
              >
                <Typography variant="body2" color="text.secondary">
                  No builds available
                </Typography>
              </Box>
            ) : (
              builds.map((build) => {
                const isSelected = tempSelectedBuild === build.buildName;
                return (
                  <ListItem key={build.buildName} sx={{ border: '1px solid', borderRadius: 1, borderColor: 'divider' }} disablePadding>
                    <ListItemButton
                      onClick={() => handleBuildClick(build.buildName)}
                      selected={isSelected}
                    >
                      <ListItemText
                        primary={build.buildName}
                        secondary={
                          <Box display="flex" gap={2}>
                            <Box display="flex" alignItems="center" gap={0.5}>
                              <GitCommit size={16} />
                              <Typography variant="caption">
                                {build.commitId?.substring(0, 12) || "N/A"}
                              </Typography>
                            </Box>
                            <Box display="flex" alignItems="center" gap={0.5}>
                              <AccessTime size={12} />
                              <Typography variant="caption">
                                {dayjs(build.startedAt).format(
                                  "DD MMM YYYY HH:mm:ss"
                                )}
                              </Typography>
                            </Box>
                          </Box>
                        }
                      />
                    </ListItemButton>
                  </ListItem>
                );
              })
            )}
          </List>
        </Box>
        <Box
          display="flex"
          gap={1}
          justifyContent="flex-end"
          width="100%"
          mt={3}
        >
          <Button variant="outlined" color="primary" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="contained"
            color="primary"
            onClick={handleConfirmSelection}
            disabled={!tempSelectedBuild}
            startIcon={<Check size={16} />}
          >
            Select
          </Button>
        </Box>
      </DrawerContent>
    </DrawerWrapper>
  );
}

