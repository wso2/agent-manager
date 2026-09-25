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

import { Box, Button, Card, CardContent, CircularProgress, Form, MenuItem, Select, Stack, TextField, Typography } from "@wso2/oxygen-ui";
import { Edit } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerWrapper,
  DrawerHeader,
  DrawerContent,
  useDirtyState,
  useDrawerFullscreen,
  useFormValidation,
} from "@agent-management-platform/views";
import { z } from "zod";
import { useListDeploymentPipelines, useListEnvironments, useUpdateProject } from "@agent-management-platform/api-client";
import { type DeploymentPipelineResponse, ProjectResponse, UpdateProjectRequest, INPUT_LIMITS } from "@agent-management-platform/types";
import { MarkdownEditor } from "@agent-management-platform/shared-component";
import { useEffect, useState, useCallback, useMemo } from "react";

function pipelineChainLabel(
  pipeline: DeploymentPipelineResponse,
  envDisplayNameMap: Map<string, string>,
): string | null {
  if (!pipeline.promotionPaths.length) return null;
  const names = [
    ...pipeline.promotionPaths.map((pp) => pp.sourceEnvironmentRef),
    pipeline.promotionPaths[pipeline.promotionPaths.length - 1]?.targetEnvironmentRefs[0]?.name,
  ].filter(Boolean) as string[];
  return names.map((n) => envDisplayNameMap.get(n) ?? n).join(" → ");
}

function SelectedPipelineValue({
  pipeline,
  envDisplayNameMap,
}: {
  pipeline: DeploymentPipelineResponse;
  envDisplayNameMap: Map<string, string>;
}) {
  const chain = pipelineChainLabel(pipeline, envDisplayNameMap);
  return (
    <Stack direction="row" alignItems="center" spacing={1}>
      <Typography variant="body2">{pipeline.displayName}</Typography>
      {chain && <Typography variant="caption" color="text.disabled">{chain}</Typography>}
    </Stack>
  );
}

function PipelineMenuItem({
  pipeline,
  envDisplayNameMap,
}: {
  pipeline: DeploymentPipelineResponse;
  envDisplayNameMap: Map<string, string>;
}) {
  const chain = pipelineChainLabel(pipeline, envDisplayNameMap);
  return (
    <Stack>
      <Typography variant="body2">{pipeline.displayName}</Typography>
      {chain && <Typography variant="caption" color="text.disabled">{chain}</Typography>}
    </Stack>
  );
}

interface EditProjectDrawerProps {
  open: boolean;
  onClose: () => void;
  project: ProjectResponse;
  orgId: string;
}

interface EditProjectFormValues {
  name: string;
  displayName: string;
  description?: string;
  deploymentPipeline: string;
}

const editProjectSchema = z.object({
  displayName: z
    .string()
    .trim()
    .min(3, 'Display name must be at least 3 characters')
    .max(100, 'Display name must be at most 100 characters'),
  name: z
    .string()
    .trim()
    .min(3, 'Name must be at least 3 characters')
    .regex(/^[a-z0-9-]+$/, 'Name must be lowercase letters, numbers, and hyphens only (no spaces)')
    .max(50, 'Name must be at most 50 characters'),
  description: z.string().trim().optional(),
  deploymentPipeline: z
    .string()
    .trim()
    .min(1, 'Deployment pipeline is required'),
});

export function EditProjectDrawer({ open, onClose, project, orgId }: EditProjectDrawerProps) {
  const [formData, setFormData] = useState<EditProjectFormValues>({
    name: project.name,
    displayName: project.displayName,
    description: project.description || '',
    deploymentPipeline: project.deploymentPipeline || '',
  });
  const { isFullscreen, toggle: toggleFullscreen, reset: resetFullscreen } = useDrawerFullscreen();

  const {
    errors, 
    validateField,
    validateForm, 
    clearErrors,
    setFieldError,
  } = useFormValidation<EditProjectFormValues>(editProjectSchema);

  const { checkDirty, resetDirty } = useDirtyState(formData);

  const handleFieldChange = useCallback((field: keyof EditProjectFormValues, value: string) => {
    setFormData(prevData => {
      const newData = { ...prevData, [field]: value };
      checkDirty(newData);
      
      // Validate and update error
      const error = validateField(field, value);
      setFieldError(field, error);
      
      return newData;
    });
  }, [checkDirty, validateField, setFieldError]);

  const { mutate: updateProject, isPending } = useUpdateProject({
    orgName: orgId,
    projName: project.name,
  });

  const { data: pipelinesData, isLoading: isPipelinesLoading } = useListDeploymentPipelines(
    { orgName: orgId },
  );
  const pipelines = useMemo(
    () => pipelinesData?.deploymentPipelines ?? [],
    [pipelinesData?.deploymentPipelines],
  );

  const { data: environments } = useListEnvironments({ orgName: orgId });
  const envDisplayNameMap = useMemo(() => {
    const map = new Map<string, string>();
    (environments ?? []).forEach((e) => map.set(e.name, e.displayName ?? e.name));
    return map;
  }, [environments]);

  // Auto-select the first pipeline if the project has none configured.
  useEffect(() => {
    if (!formData.deploymentPipeline && pipelines.length > 0) {
      setFormData((prev) => ({ ...prev, deploymentPipeline: pipelines[0].name }));
    }
  }, [pipelines, formData.deploymentPipeline]);

  // Reset form when project changes or drawer opens
  useEffect(() => {
    if (open) {
      setFormData({
        name: project.name,
        displayName: project.displayName,
        description: project.description || '',
        deploymentPipeline: project.deploymentPipeline || '',
      });
      clearErrors();
      resetDirty();
      resetFullscreen();
    }
  }, [project, open, clearErrors, resetDirty, resetFullscreen]);

  const handleSubmit = useCallback((e: React.FormEvent) => {
    e.preventDefault();
    
    if (!validateForm(formData)) {
      return;
    }

    const payload: UpdateProjectRequest = {
      name: formData.name,
      displayName: formData.displayName,
      description: formData.description,
      deploymentPipeline: formData.deploymentPipeline,
    };

    updateProject(payload, {
      onSuccess: () => {
        clearErrors();
        resetDirty();
        onClose();
      },
    });
  }, [formData, validateForm, updateProject, onClose, clearErrors, resetDirty]);

  const isValid = useMemo(() => {
    return (
      formData.displayName.trim().length >= 3 && 
      formData.name.trim().length >= 3 &&
      Object.keys(errors).length === 0
    );
  }, [formData.displayName, formData.name, errors]);

  return (
    <DrawerWrapper open={open} onClose={onClose} fullscreen={isFullscreen}>
      <DrawerHeader
        icon={<Edit size={24} />}
        title="Edit Project"
        onClose={onClose}
        isFullscreen={isFullscreen}
        onToggleFullscreen={toggleFullscreen}
      />
      <DrawerContent>
        <form onSubmit={handleSubmit}>
          <Box display="flex" flexDirection="column" gap={2} flexGrow={1}>
            <Card variant="outlined">
              <CardContent sx={{ gap: 1, display: "flex", flexDirection: "column" }}>
                <Box display="flex" flexDirection="column" gap={1}>
                  <Typography variant="h5">Project Details</Typography>
                </Box>
                <Box display="flex" flexDirection="column" gap={1}>
                  <Form.ElementWrapper label="Name" name="displayName">
                    <TextField
                      slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.NAME } }}
                      id="displayName"
                      placeholder="e.g., Customer Support Platform"
                      size="small"
                      fullWidth
                      disabled={isPending}
                      value={formData.displayName}
                      onChange={(e) => handleFieldChange('displayName', e.target.value)}
                      error={!!errors.displayName}
                      helperText={errors.displayName}
                    />
                  </Form.ElementWrapper>
                  <Form.ElementWrapper label="Description (optional)" name="description">
                    <MarkdownEditor
                      id="description"
                      placeholder="Short description of this project. Markdown is supported."
                      disabled={isPending}
                      value={formData.description || ''}
                      onChange={(value) => handleFieldChange('description', value)}
                      error={!!errors.description}
                      helperText={errors.description}
                    />
                  </Form.ElementWrapper>
                  <Form.ElementWrapper label="Deployment Pipeline" name="deploymentPipeline">
                    <Select
                      id="deploymentPipeline"
                      value={formData.deploymentPipeline}
                      onChange={(e) => handleFieldChange('deploymentPipeline', e.target.value as string)}
                      error={!!errors.deploymentPipeline}
                      disabled={isPipelinesLoading || isPending}
                      displayEmpty
                      renderValue={(value) => {
                        const selected = pipelines.find((p) => p.name === value);
                        if (!selected) {
                          return (
                            <Typography variant="body2" color="text.disabled">
                              Select a pipeline
                            </Typography>
                          );
                        }
                        return (
                          <SelectedPipelineValue
                            pipeline={selected}
                            envDisplayNameMap={envDisplayNameMap}
                          />
                        );
                      }}
                      fullWidth
                      size="small"
                    >
                      {isPipelinesLoading && (
                        <MenuItem value="" disabled>
                          <Stack direction="row" alignItems="center" gap={1}>
                            <CircularProgress size={12} />
                            <Typography variant="caption">Loading pipelines...</Typography>
                          </Stack>
                        </MenuItem>
                      )}
                      {!isPipelinesLoading && pipelines.length === 0 && (
                        <MenuItem value="" disabled>No deployment pipelines available</MenuItem>
                      )}
                      {pipelines.map((p) => (
                        <MenuItem key={p.name} value={p.name}>
                          <PipelineMenuItem pipeline={p} envDisplayNameMap={envDisplayNameMap} />
                        </MenuItem>
                      ))}
                    </Select>
                  </Form.ElementWrapper>
                </Box>
              </CardContent>
            </Card>
            <Box display="flex" justifyContent="flex-end" gap={1} mt={2}>
              <Button
                variant="outlined"
                color="inherit"
                onClick={onClose}
                disabled={isPending}
              >
                Cancel
              </Button>
              <Button
                type="submit"
                variant="contained"
                color="primary"
                disabled={!isValid || isPending}
              >
                {isPending ? "Updating..." : "Update Project"}
              </Button>
            </Box>
          </Box>
        </form>
      </DrawerContent>
    </DrawerWrapper>
  );
}
