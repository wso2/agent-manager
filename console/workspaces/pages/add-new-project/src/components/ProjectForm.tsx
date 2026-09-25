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
  CircularProgress,
  Form,
  Select,
  MenuItem,
  TextField,
  Typography,
  Stack,
} from "@wso2/oxygen-ui";
import { useEffect, useMemo, useCallback } from "react";
import { useParams } from "react-router-dom";
import { debounce } from "lodash";
import { useGenerateResourceName, useListDeploymentPipelines, useListEnvironments } from "@agent-management-platform/api-client";
import { MarkdownEditor } from "@agent-management-platform/shared-component";
import { AddProjectFormValues } from "../form/schema";
import { type DeploymentPipelineResponse, INPUT_LIMITS } from "@agent-management-platform/types";

function pipelineChainLabel(
  pipeline: DeploymentPipelineResponse,
  envDisplayNameMap: Map<string, string>,
): string | null {
  if (pipeline.promotionPaths.length === 0) return null;
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
      {chain && (
        <Typography variant="caption" color="text.disabled">{chain}</Typography>
      )}
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
      {chain && <Typography variant="caption" color="text.disbaled">{chain}</Typography>}
    </Stack>
  );
}

interface ProjectFormProps {
  formData: AddProjectFormValues;
  setFormData: React.Dispatch<React.SetStateAction<AddProjectFormValues>>;
  errors: Partial<Record<keyof AddProjectFormValues, string>>;
  validateField: (
    field: keyof AddProjectFormValues,
    value: string,
  ) => string | undefined;
  setFieldError: (
    field: keyof AddProjectFormValues,
    error: string | undefined,
  ) => void;
  checkDirty: (data: AddProjectFormValues) => void;
}

export const ProjectForm = ({
  formData,
  setFormData,
  errors,
  validateField,
  setFieldError,
  checkDirty,
}: ProjectFormProps) => {
  const { orgId } = useParams<{ orgId: string }>();

  const handleFieldChange = useCallback(
    (field: keyof AddProjectFormValues, value: string) => {
      const newData = { ...formData, [field]: value };
      setFormData(newData);
      checkDirty(newData);

      const error = validateField(field, value);
      setFieldError(field, error);
    },
    [formData, setFormData, checkDirty, validateField, setFieldError],
  );

  const { mutate: generateName, isPending: isGeneratingName } =
    useGenerateResourceName({
      orgName: orgId,
    });

  const { data: pipelinesData, isLoading: isPipelinesLoading } =
    useListDeploymentPipelines({ orgName: orgId });

  const pipelines = pipelinesData?.deploymentPipelines ?? [];

  const { data: environments } = useListEnvironments({ orgName: orgId });
  const envDisplayNameMap = useMemo(() => {
    const map = new Map<string, string>();
    (environments ?? []).forEach((e) => map.set(e.name, e.displayName ?? e.name));
    return map;
  }, [environments]);

  // Auto-select the first pipeline once data loads if nothing is selected yet.
  useEffect(() => {
    if (!formData.deploymentPipeline && pipelines.length > 0) {
      const first = pipelines[0].name;
      setFormData((prev) => {
        const next = { ...prev, deploymentPipeline: first };
        checkDirty(next);
        return next;
      });
    }
  }, [pipelines, formData.deploymentPipeline, setFormData, checkDirty]);

  // Create debounced function for name generation
  const debouncedGenerateName = useMemo(
    () =>
      debounce((name: string) => {
        if (name.length < 3) {
          handleFieldChange("name", "");
          return;
        }
        generateName(
          {
            displayName: name,
            resourceType: "project",
          },
          {
            onSuccess: (data) => {
              setFormData((prevData) => {
                const newData = { ...prevData, name: data.name };
                checkDirty(newData);
                return newData;
              });
            },
            onError: (error) => {
              // eslint-disable-next-line no-console
              console.error("Failed to generate name:", error);
            },
          },
        );
      }, 500), // 500ms delay
    [generateName, setFormData, checkDirty],
  );

  // Cleanup debounce on unmount
  useEffect(() => {
    return () => {
      debouncedGenerateName.cancel();
    };
  }, [debouncedGenerateName]);

  // Auto-generate name from display name using API with debounce
  useEffect(() => {
    if (formData.displayName && formData.displayName.length >= 3) {
      debouncedGenerateName(formData.displayName);
    } else {
      // Clear the name field if display name is empty or too short
      debouncedGenerateName.cancel();
      setFormData((prev) => ({ ...prev, name: "" }));
    }
  }, [formData.displayName, setFormData, debouncedGenerateName]);

  return (
    <Form.Stack spacing={3}>
      <Form.Section>
        <Form.Subheader>Project Details</Form.Subheader>
        <Form.Stack spacing={2}>
          <Form.ElementWrapper label="Name" name="displayName">
            <TextField
              slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.NAME } }}
              id="displayName"
              value={formData.displayName}
              onChange={(e) => handleFieldChange("displayName", e.target.value)}
              placeholder="e.g., Customer Support Platform"
              error={!!errors.displayName}
              helperText={
                isGeneratingName ? (
                  <Stack direction="row" alignItems="center" gap={1}>
                    <CircularProgress size={12} />
                    <Typography variant="caption">Validating name...</Typography>
                  </Stack>
                ) : (
                  errors.displayName
                )
              }
              fullWidth
            />
          </Form.ElementWrapper>

          <Form.ElementWrapper
            label="Description (optional)"
            name="description"
          >
            <MarkdownEditor
              id="description"
              value={formData.description || ""}
              onChange={(value) => handleFieldChange("description", value)}
              placeholder="Short description of this project. Markdown is supported."
              error={!!errors.description}
              helperText={errors.description}
            />
          </Form.ElementWrapper>

          <Form.ElementWrapper
            label="Deployment Pipeline"
            name="deploymentPipeline"
          >
            <Select
              id="deploymentPipeline"
              value={formData.deploymentPipeline}
              onChange={(e) =>
                handleFieldChange("deploymentPipeline", e.target.value)
              }
              error={!!errors.deploymentPipeline}
              disabled={isPipelinesLoading}
              displayEmpty
              renderValue={(value) => {
                const selected = pipelines.find((p) => p.name === value);
                if (!selected) return value;
                return (
                  <SelectedPipelineValue
                    pipeline={selected}
                    envDisplayNameMap={envDisplayNameMap}
                  />
                );
              }}
              fullWidth
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
                <MenuItem value="" disabled>
                  No deployment pipelines available
                </MenuItem>
              )}
              {pipelines.map((p) => (
                <MenuItem key={p.name} value={p.name}>
                  <PipelineMenuItem pipeline={p} envDisplayNameMap={envDisplayNameMap} />
                </MenuItem>
              ))}
            </Select>
          </Form.ElementWrapper>
        </Form.Stack>
      </Form.Section>
    </Form.Stack>
  );
};
