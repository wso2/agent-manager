/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Form,
  FormControl,
  FormLabel,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Edit } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
  useFormValidation,
} from "@agent-management-platform/views";
import {
  useUpdateOrgDeploymentPipeline,
  useListEnvironments,
} from "@agent-management-platform/api-client";
import { type DeploymentPipelineResponse, INPUT_LIMITS } from "@agent-management-platform/types";
import { editPipelineSchema, type EditPipelineFormValues, PIPELINE_DISPLAY_NAME_MAX_LENGTH } from "../form/schema";
import { chainToPromotionPaths } from "../utils/chainUtils";
import { validatePromotionChain } from "../utils/validatePromotionChain";
import { PipelineChainEditor } from "./PipelineChainEditor";

interface EditDeploymentPipelineDrawerProps {
  open: boolean;
  onClose: () => void;
  pipeline: DeploymentPipelineResponse;
  orgId: string;
}

function pipelineToChain(pipeline: DeploymentPipelineResponse): string[] {
  if (pipeline.promotionPaths.length === 0) return [""];
  const validation = validatePromotionChain(pipeline.promotionPaths);
  if (validation.valid && validation.chain && validation.chain.length >= 1) {
    return validation.chain;
  }
  // Fallback for invalid existing paths: collect sources + last target
  const sources = pipeline.promotionPaths.map((p) => p.sourceEnvironmentRef);
  const lastTarget = pipeline.promotionPaths[pipeline.promotionPaths.length - 1]?.targetEnvironmentRefs[0]?.name ?? "";
  return [...sources, lastTarget];
}

export function EditDeploymentPipelineDrawer(
  { open, onClose, pipeline, orgId }: EditDeploymentPipelineDrawerProps,
) {
  const [formData, setFormData] = useState<EditPipelineFormValues>(() => ({
    displayName: pipeline.displayName,
    description: pipeline.description ?? "",
    chain: pipelineToChain(pipeline),
  }));

  const [submitError, setSubmitError] = useState<string | null>(null);

  const { errors, validateForm, setFieldError, validateField } =
    useFormValidation<EditPipelineFormValues>(editPipelineSchema);

  const {
    mutateAsync: updatePipeline,
    isPending: isUpdating,
    error: updateError,
    reset: resetMutation,
  } = useUpdateOrgDeploymentPipeline();

  const { data: environments } = useListEnvironments({ orgName: orgId });
  const envOptions = useMemo(() => environments ?? [], [environments]);

  useEffect(() => {
    if (open) {
      setFormData({
        displayName: pipeline.displayName,
        description: pipeline.description ?? "",
        chain: pipelineToChain(pipeline),
      });
      setSubmitError(null);
      resetMutation();
    }
  }, [open, pipeline, resetMutation]);

  const handleFieldChange = useCallback(
    (field: "displayName" | "description", value: string) => {
      setFormData((prev) => {
        const next = { ...prev, [field]: value };
        setFieldError(
          field,
          validateField(field, next[field as keyof EditPipelineFormValues], next),
        );
        return next;
      });
    },
    [setFieldError, validateField],
  );

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      setSubmitError(null);

      const result = editPipelineSchema.safeParse(formData);
      if (!result.success) {
        validateForm(formData);
        return;
      }

      try {
        await updatePipeline({
          params: { orgName: orgId, pipelineName: pipeline.name },
          body: {
            displayName: result.data.displayName.trim(),
            description: result.data.description?.trim(),
            promotionPaths: chainToPromotionPaths(result.data.chain),
          },
        });
        onClose();
      } catch {
        // handled by updateError
      }
    },
    [formData, validateForm, updatePipeline, orgId, pipeline.name, onClose],
  );

  const errorMessage = useMemo(() => {
    if (submitError) return submitError;
    if (updateError) return (updateError as Error)?.message ?? "Failed to update pipeline";
    return null;
  }, [submitError, updateError]);

  const allFilled = formData.chain.every((v) => v !== "");

  return (
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader icon={<Edit size={24} />} title="Edit Deployment Pipeline" onClose={onClose} />
      <DrawerContent>
        <form onSubmit={handleSubmit}>
          <Stack spacing={3}>
            {errorMessage && (
              <Alert severity="error">
                <Typography variant="body2">{errorMessage}</Typography>
              </Alert>
            )}

            <Form.Section>
              <Form.Header>Pipeline Details</Form.Header>
              <Form.Stack spacing={2}>
                <FormControl fullWidth error={Boolean(errors.displayName)}>
                  <FormLabel required>Display Name</FormLabel>
                  <TextField
                    slotProps={{ htmlInput: { maxLength: PIPELINE_DISPLAY_NAME_MAX_LENGTH } }}
                    fullWidth
                    size="small"
                    value={formData.displayName}
                    onChange={(e) => handleFieldChange("displayName", e.target.value)}
                    placeholder="e.g., Production Pipeline"
                    error={Boolean(errors.displayName)}
                    helperText={errors.displayName}
                    disabled={isUpdating}
                  />
                </FormControl>
                <FormControl fullWidth>
                  <FormLabel>Description</FormLabel>
                  <TextField
                    slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.DESCRIPTION } }}
                    fullWidth
                    size="small"
                    multiline
                    rows={2}
                    value={formData.description ?? ""}
                    onChange={(e) => handleFieldChange("description", e.target.value)}
                    placeholder="Optional description"
                    disabled={isUpdating}
                  />
                </FormControl>
              </Form.Stack>
            </Form.Section>

            <PipelineChainEditor
              chain={formData.chain}
              envOptions={envOptions}
              onChange={(chain) => setFormData((prev) => ({ ...prev, chain }))}
              disabled={isUpdating}
            />

            <Box display="flex" justifyContent="flex-end" gap={1} mt={2}>
              <Button variant="outlined" color="inherit" onClick={onClose} disabled={isUpdating}>
                Cancel
              </Button>
              <Button
                type="submit"
                variant="contained"
                color="primary"
                disabled={isUpdating || !allFilled}
              >
                {isUpdating ? "Saving..." : "Save"}
              </Button>
            </Box>
          </Stack>
        </form>
      </DrawerContent>
    </DrawerWrapper>
  );
}
