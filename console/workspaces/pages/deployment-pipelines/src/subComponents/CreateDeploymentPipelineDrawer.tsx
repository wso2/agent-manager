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
import { GitBranch } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
  useFormValidation,
} from "@agent-management-platform/views";
import {
  useCreateDeploymentPipeline,
  useListEnvironments,
} from "@agent-management-platform/api-client";
import { createPipelineSchema, type CreatePipelineFormValues, PIPELINE_DISPLAY_NAME_MAX_LENGTH } from "../form/schema";
import { chainToPromotionPaths } from "../utils/chainUtils";
import { PipelineChainEditor } from "./PipelineChainEditor";
import { INPUT_LIMITS } from "@agent-management-platform/types";

interface CreateDeploymentPipelineDrawerProps {
  open: boolean;
  onClose: () => void;
  orgId: string;
}

const DEFAULT_FORM: CreatePipelineFormValues = {
  displayName: "",
  description: "",
  chain: [],
};

export function CreateDeploymentPipelineDrawer(
  { open, onClose, orgId }: CreateDeploymentPipelineDrawerProps,
) {
  const [formData, setFormData] = useState<CreatePipelineFormValues>(DEFAULT_FORM);

  const { errors, validateForm, setFieldError, validateField } =
    useFormValidation<CreatePipelineFormValues>(createPipelineSchema);

  const {
    mutateAsync: createPipeline,
    isPending,
    error: submitError,
    reset: resetMutation,
  } = useCreateDeploymentPipeline();

  const { data: environments } = useListEnvironments({ orgName: orgId });
  const envOptions = useMemo(() => environments ?? [], [environments]);

  useEffect(() => {
    if (open) {
      setFormData(DEFAULT_FORM);
      resetMutation();
    }
  }, [open, resetMutation]);

  const handleFieldChange = useCallback(
    (field: "displayName" | "description", value: string) => {
      setFormData((prev) => {
        const next = { ...prev, [field]: value };
        setFieldError(field, validateField(field, value, next));
        return next;
      });
    },
    [setFieldError, validateField],
  );

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      const result = createPipelineSchema.safeParse(formData);
      if (!result.success) {
        validateForm(formData);
        return;
      }
      try {
        await createPipeline({
          params: { orgName: orgId },
          body: {
            displayName: result.data.displayName.trim(),
            description: result.data.description?.trim(),
            promotionPaths: chainToPromotionPaths(result.data.chain),
          },
        });
        onClose();
      } catch {
        // handled by submitError
      }
    },
    [formData, validateForm, createPipeline, orgId, onClose],
  );

  const errorMessage = useMemo(
    () => (submitError ? (submitError as Error)?.message ?? "Failed to create pipeline" : null),
    [submitError],
  );

  return (
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader icon={<GitBranch size={24} />} title="Create Deployment Pipeline" onClose={onClose} />
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
                    disabled={isPending}
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
                    disabled={isPending}
                  />
                </FormControl>
              </Form.Stack>
            </Form.Section>

            <PipelineChainEditor
              chain={formData.chain}
              envOptions={envOptions}
              onChange={(chain) => setFormData((prev) => ({ ...prev, chain }))}
              disabled={isPending}
            />

            <Box display="flex" justifyContent="flex-end" gap={1} mt={2}>
              <Button variant="outlined" color="inherit" onClick={onClose} disabled={isPending}>
                Cancel
              </Button>
              <Button
                type="submit"
                variant="contained"
                color="primary"
                disabled={isPending || formData.chain.length === 0}
              >
                {isPending ? "Creating..." : "Create"}
              </Button>
            </Box>
          </Stack>
        </form>
      </DrawerContent>
    </DrawerWrapper>
  );
}
