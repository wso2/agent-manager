/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
  useDirtyState,
} from "@agent-management-platform/views";
import {
  useConfirmationDialog,
  useUnsavedChangesGuard,
} from "@agent-management-platform/shared-component";
import type { DeploymentPipelineResponse, Environment } from "@agent-management-platform/types";
import { Avatar, Box, Button, Divider, Form, Stack, Typography } from "@wso2/oxygen-ui";
import { Check, Circle, GitBranch } from "@wso2/oxygen-ui-icons-react";
import { useCallback, useState } from "react";
import { buildPromotionChain, PromotionChainChips } from "./promotionChain";

interface SelectPipelineDrawerProps {
  open: boolean;
  onClose: () => void;
  pipelines: DeploymentPipelineResponse[];
  envMap: Map<string, Environment>;
  selectedName?: string;
  isUpdating?: boolean;
  onSelect: (pipelineName: string) => void;
}

export function SelectPipelineDrawer({
  open,
  onClose,
  pipelines,
  envMap,
  selectedName,
  isUpdating,
  onSelect,
}: SelectPipelineDrawerProps) {
  const [pendingName, setPendingName] = useState(selectedName);
  const { isDirty, checkDirty } = useDirtyState({ pendingName: selectedName });
  const { addConfirmation } = useConfirmationDialog();
  useUnsavedChangesGuard(open && isDirty);

  const handleSelectCard = (pipelineName: string) => {
    setPendingName(pipelineName);
    checkDirty({ pendingName: pipelineName });
  };

  const handleRequestClose = useCallback(() => {
    if (!isDirty) {
      onClose();
      return;
    }
    addConfirmation({
      analytics: { entity: "deployment-pipeline", action: "discard-changes" },
      title: "Discard changes?",
      description: "You have an unsaved pipeline selection. Closing now will discard it.",
      confirmButtonText: "Discard",
      confirmButtonColor: "error",
      onConfirm: onClose,
    });
  }, [addConfirmation, isDirty, onClose]);

  return (
    <DrawerWrapper open={open} onClose={handleRequestClose} minWidth={620} maxWidth={620}>
      <DrawerHeader icon={<GitBranch size={24} />} title="Select Deployment Pipeline" onClose={handleRequestClose} />
      <DrawerContent>
        <Box display="flex" flexDirection="column" gap={2} flexGrow={1}>
          <Stack spacing={1}>
            {pipelines.length === 0 ? (
              <Typography variant="body2" color="text.disabled" sx={{ fontStyle: "italic" }}>
                No deployment pipelines available for this organization.
              </Typography>
            ) : (
              pipelines.map((pipeline) => {
                const isSelected = pipeline.name === pendingName;
                const chain = buildPromotionChain(pipeline.promotionPaths ?? []);
                return (
                  <Form.CardButton
                    key={pipeline.name}
                    selected={isSelected}
                    disabled={isUpdating}
                    onClick={() => handleSelectCard(pipeline.name)}
                    aria-label={`${pipeline.displayName || pipeline.name}. ${isSelected ? "Selected" : "Click to select"}`}
                  >
                    <Form.CardContent>
                      <Stack direction="row" spacing={2} alignItems="flex-start">
                        <Avatar
                          sx={{
                            height: 32,
                            width: 32,
                            backgroundColor: isSelected ? "primary.main" : "secondary.main",
                            color: isSelected ? "common.white" : "text.secondary",
                          }}
                        >
                          {isSelected ? <Check size={16} /> : <Circle size={16} />}
                        </Avatar>
                        <Stack spacing={0.5} flexGrow={1} sx={{ minWidth: 0 }}>
                          <Typography variant="h4">
                            {pipeline.displayName || pipeline.name}
                          </Typography>
                          {pipeline.description && (
                            <Typography variant="body2" color="text.secondary">
                              {pipeline.description}
                            </Typography>
                          )}
                          <Box display="flex" flexWrap="wrap" alignItems="center" gap={0.75} sx={{ pt: 0.5 }}>
                            <PromotionChainChips chain={chain} envMap={envMap} />
                          </Box>
                        </Stack>
                      </Stack>
                    </Form.CardContent>
                  </Form.CardButton>
                );
              })
            )}
          </Stack>
          <Divider />
          <Box display="flex" justifyContent="flex-end" gap={1}>
            <Button variant="outlined" color="inherit" onClick={handleRequestClose} disabled={isUpdating}>
              Cancel
            </Button>
            <Button
              variant="contained"
              color="primary"
              disabled={!isDirty || isUpdating}
              onClick={() => pendingName && onSelect(pendingName)}
            >
              {isUpdating ? "Saving..." : "Save"}
            </Button>
          </Box>
        </Box>
      </DrawerContent>
    </DrawerWrapper>
  );
}
