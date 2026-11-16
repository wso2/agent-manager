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

import {
  Box,
  Button,
  Form,
  List,
  ListItem,
  ListItemButton,
  ListItemText,
  Typography,
} from "@wso2/oxygen-ui";
import { useCallback, useEffect, useState } from "react";
import {
  Clock as AccessTime,
  Tag,
  Check,
} from "@wso2/oxygen-ui-icons-react";
import {
  DrawerWrapper,
  DrawerHeader,
  DrawerContent,
} from "@agent-management-platform/views";
import { format, isValid, parseISO } from "date-fns";
import type { AgentKindVersionResponse } from "@agent-management-platform/types";

const formatVersionDate = (value: string): string => {
  const date = parseISO(value);
  return isValid(date) ? format(date, "dd MMM yyyy HH:mm:ss") : "—";
};

export interface KindVersionSelectorDrawerProps {
  open: boolean;
  onClose: () => void;
  versions: AgentKindVersionResponse[];
  selectedVersion: string;
  onSelectVersion: (version: string) => void;
}

export function KindVersionSelectorDrawer({
  open,
  onClose,
  versions,
  selectedVersion,
  onSelectVersion,
}: KindVersionSelectorDrawerProps) {
  const [tempSelectedVersion, setTempSelectedVersion] =
    useState<string>(selectedVersion);

  useEffect(() => {
    if (open) {
      setTempSelectedVersion(selectedVersion);
    }
  }, [open, selectedVersion]);

  const handleConfirmSelection = useCallback(() => {
    if (tempSelectedVersion) {
      onSelectVersion(tempSelectedVersion);
    }
  }, [tempSelectedVersion, onSelectVersion]);

  return (
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader
        icon={<Tag size={24} />}
        title="Select Kind Version"
        onClose={onClose}
      />
      <DrawerContent>
        <Form.Stack spacing={3}>
          <Form.Section>
            <Form.Header>Available Versions</Form.Header>
            <Typography variant="body2" color="text.secondary">
              Choose a kind version to deploy. The latest version is shown
              first.
            </Typography>
            <Form.Stack spacing={1} sx={{ mt: 1 }}>
              <List
                disablePadding
                sx={{ gap: 1, display: "flex", flexDirection: "column" }}
              >
                {versions.length === 0 ? (
                  <Box
                    display="flex"
                    justifyContent="center"
                    alignItems="center"
                    sx={{ minHeight: (theme) => theme.spacing(25) }}
                  >
                    <Typography variant="body2" color="text.secondary">
                      No versions available
                    </Typography>
                  </Box>
                ) : (
                  versions.map((ver) => {
                    const isSelected = tempSelectedVersion === ver.version;
                    return (
                      <ListItem
                        key={ver.version}
                        sx={(theme) => ({
                          border: `1px solid ${theme.palette.divider}`,
                          borderRadius: 1,
                        })}
                        disablePadding
                      >
                        <ListItemButton
                          onClick={() => setTempSelectedVersion(ver.version)}
                          selected={isSelected}
                        >
                          <ListItemText
                            primary={ver.version}
                            secondary={
                              <Box display="flex" gap={2}>
                                <Box display="flex" alignItems="center" gap={0.5}>
                                  <AccessTime size={12} />
                                  <Typography variant="caption">
                                    {formatVersionDate(ver.createdAt)}
                                  </Typography>
                                </Box>
                              </Box>
                            }
                          />
                          {isSelected && <Check size={16} />}
                        </ListItemButton>
                      </ListItem>
                    );
                  })
                )}
              </List>
            </Form.Stack>
          </Form.Section>

          <Box display="flex" gap={1} justifyContent="flex-end" width="100%">
            <Button variant="outlined" color="primary" onClick={onClose}>
              Cancel
            </Button>
            <Button
              variant="contained"
              color="primary"
              onClick={handleConfirmSelection}
              disabled={!tempSelectedVersion}
              startIcon={<Check size={16} />}
            >
              Select
            </Button>
          </Box>
        </Form.Stack>
      </DrawerContent>
    </DrawerWrapper>
  );
}
