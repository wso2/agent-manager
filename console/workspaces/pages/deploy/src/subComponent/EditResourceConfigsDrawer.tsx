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
  Box,
  Button,
  Collapse,
  Form,
  FormControlLabel,
  Switch,
  TextField,
} from "@wso2/oxygen-ui";
import { Settings } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerWrapper,
  DrawerHeader,
  DrawerContent,
  useFormValidation,
} from "@agent-management-platform/views";
import { z } from "zod";
import { useUpdateAgentResourceConfigs } from "@agent-management-platform/api-client";
import type {
  AgentResourceConfigsResponse,
  UpdateAgentResourceConfigsRequest,
} from "@agent-management-platform/types";
import { useCallback, useEffect, useState } from "react";

const cpuPattern = /^[0-9]+(\.[0-9]+)?m?$/i;
const memoryPattern = /^[0-9]+(\.[0-9]+)?(Ki|Mi|Gi|Ti|Pi|Ei)$/i;

const resourceConfigsSchema = z
  .object({
    replicas: z
      .number()
      .int("Replicas must be a whole number")
      .min(0, "Replicas must be at least 0")
      .max(10, "Replicas must be at most 10"),
    cpuRequest: z
      .string()
      .trim()
      .min(1, "CPU request is required")
      .regex(cpuPattern, "Use format like 100m, 0.5, or 1.5"),
    memoryRequest: z
      .string()
      .trim()
      .min(1, "Memory request is required")
      .regex(memoryPattern, "Use format like 256Mi, 512Mi, or 1Gi"),
    cpuLimit: z
      .string()
      .trim()
      .optional()
      .refine((v) => !v || cpuPattern.test(v), "Use format like 500m or 2"),
    memoryLimit: z
      .string()
      .trim()
      .optional()
      .refine((v) => !v || memoryPattern.test(v), "Use format like 512Mi or 2Gi"),
    autoScalingEnabled: z.boolean(),
    minReplicas: z.number().int().min(1).optional(),
    maxReplicas: z.number().int().min(1).optional(),
  })
  .refine(
    (data) => {
      if (!data.autoScalingEnabled) return true;
      return (
        data.minReplicas !== undefined &&
        data.maxReplicas !== undefined &&
        data.minReplicas <= data.maxReplicas
      );
    },
    { message: "Min replicas must be ≤ max replicas", path: ["maxReplicas"] }
  );

type ResourceConfigsFormValues = z.infer<typeof resourceConfigsSchema>;

function toFormValues(config: AgentResourceConfigsResponse): ResourceConfigsFormValues {
  return {
    replicas: config.replicas ?? 1,
    cpuRequest: config.resources?.requests?.cpu ?? "500m",
    memoryRequest: config.resources?.requests?.memory ?? "512Mi",
    cpuLimit: config.resources?.limits?.cpu ?? "",
    memoryLimit: config.resources?.limits?.memory ?? "",
    autoScalingEnabled: config.autoScaling?.enabled ?? false,
    minReplicas: config.autoScaling?.minReplicas ?? 1,
    maxReplicas: config.autoScaling?.maxReplicas ?? 3,
  };
}

function toRequestPayload(
  form: ResourceConfigsFormValues
): UpdateAgentResourceConfigsRequest {
  return {
    replicas: form.replicas,
    resources: {
      requests: {
        cpu: form.cpuRequest,
        memory: form.memoryRequest,
      },
      limits:
        form.cpuLimit || form.memoryLimit
          ? {
              ...(form.cpuLimit && { cpu: form.cpuLimit }),
              ...(form.memoryLimit && { memory: form.memoryLimit }),
            }
          : undefined,
    },
    autoScaling: {
      enabled: form.autoScalingEnabled,
      ...(form.autoScalingEnabled && {
        minReplicas: form.minReplicas ?? 1,
        maxReplicas: form.maxReplicas ?? 3,
      }),
    },
  };
}

export interface EditResourceConfigsDrawerProps {
  open: boolean;
  onClose: () => void;
  resourceConfigs: AgentResourceConfigsResponse | undefined;
  orgName: string;
  projName: string;
  agentName: string;
  environment?: string;
}

export function EditResourceConfigsDrawer({
  open,
  onClose,
  resourceConfigs,
  orgName,
  projName,
  agentName,
  environment,
}: EditResourceConfigsDrawerProps) {
  const [formData, setFormData] = useState<ResourceConfigsFormValues>({
    replicas: 1,
    cpuRequest: "500m",
    memoryRequest: "512Mi",
    cpuLimit: "",
    memoryLimit: "",
    autoScalingEnabled: false,
    minReplicas: 1,
    maxReplicas: 3,
  });

  const {
    errors,
    validateField,
    validateForm,
    clearErrors,
    setFieldError,
  } = useFormValidation<ResourceConfigsFormValues>(resourceConfigsSchema);

  const { mutate: updateConfigs, isPending } = useUpdateAgentResourceConfigs();

  useEffect(() => {
    if (open && resourceConfigs) {
      setFormData(toFormValues(resourceConfigs));
      clearErrors();
    }
  }, [open, resourceConfigs, clearErrors]);

  const handleFieldChange = useCallback(
    (field: keyof ResourceConfigsFormValues, value: unknown) => {
      setFormData((prev) => {
        const newData = { ...prev, [field]: value };
        const error = validateField(field, value, newData);
        setFieldError(field, error);
        return newData;
      });
    },
    [validateField, setFieldError]
  );

  const handleSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault();
      if (!validateForm(formData)) return;

      const payload = toRequestPayload(formData);
      updateConfigs(
        {
          params: { orgName, projName, agentName },
          body: payload,
          query: environment ? { environment } : undefined,
        },
        {
          onSuccess: () => {
            clearErrors();
            onClose();
          },
        }
      );
    },
    [
      formData,
      validateForm,
      updateConfigs,
      orgName,
      projName,
      agentName,
      environment,
      onClose,
      clearErrors,
    ]
  );

  const isValid =
    formData.cpuRequest.trim().length > 0 &&
    formData.memoryRequest.trim().length > 0 &&
    formData.replicas >= 0 &&
    formData.replicas <= 10 &&
    (!formData.autoScalingEnabled ||
      ((formData.minReplicas ?? 1) <= (formData.maxReplicas ?? 3) &&
        (formData.minReplicas ?? 1) >= 1 &&
        (formData.maxReplicas ?? 3) >= 1)) &&
    Object.values(errors).every((e) => !e);

  if (!resourceConfigs) return null;

  return (
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader
        icon={<Settings size={24} />}
        title="Edit Resource Configurations"
        onClose={onClose}
      />
      <DrawerContent>
        <form onSubmit={handleSubmit}>
          <Form.Stack spacing={3}>
            <Form.Section>
              <Form.Header>Replicas</Form.Header>
              <Form.Stack spacing={2}>
                <Form.ElementWrapper label="Replicas" name="replicas">
                  <TextField
                    id="replicas"
                    type="number"
                    size="small"
                    fullWidth
                    disabled={isPending || formData.autoScalingEnabled}
                    value={formData.replicas}
                    onChange={(e) =>
                      handleFieldChange("replicas", parseInt(e.target.value, 10) || 0)
                    }
                    error={!!errors.replicas}
                    helperText={errors.replicas}
                    slotProps={{ input: { inputProps: { min: 0, max: 10 } } }}
                  />
                </Form.ElementWrapper>
                <FormControlLabel
                  control={
                    <Switch
                      checked={formData.autoScalingEnabled}
                      onChange={(_, checked) =>
                        handleFieldChange("autoScalingEnabled", checked)
                      }
                      disabled={isPending}
                    />
                  }
                  label="Enable autoscaling"
                />
                <Collapse in={formData.autoScalingEnabled}>
                  <Form.Stack spacing={2} sx={{ mt: 1 }}>
                    <Form.ElementWrapper label="Min replicas" name="minReplicas">
                      <TextField
                        id="minReplicas"
                        type="number"
                        size="small"
                        fullWidth
                        disabled={isPending}
                        value={formData.minReplicas ?? 1}
                        onChange={(e) =>
                          handleFieldChange(
                            "minReplicas",
                            parseInt(e.target.value, 10) || 1
                          )
                        }
                        error={!!errors.minReplicas}
                        helperText={errors.minReplicas}
                        slotProps={{ input: { inputProps: { min: 1 } } }}
                      />
                    </Form.ElementWrapper>
                    <Form.ElementWrapper label="Max replicas" name="maxReplicas">
                      <TextField
                        id="maxReplicas"
                        type="number"
                        size="small"
                        fullWidth
                        disabled={isPending}
                        value={formData.maxReplicas ?? 3}
                        onChange={(e) =>
                          handleFieldChange(
                            "maxReplicas",
                            parseInt(e.target.value, 10) || 3
                          )
                        }
                        error={!!errors.maxReplicas}
                        helperText={errors.maxReplicas}
                        slotProps={{ input: { inputProps: { min: 1 } } }}
                      />
                    </Form.ElementWrapper>
                  </Form.Stack>
                </Collapse>
              </Form.Stack>
            </Form.Section>

            <Form.Section>
              <Form.Header>Resource Requests</Form.Header>
              <Form.Stack spacing={2}>
                <Form.ElementWrapper label="CPU request" name="cpuRequest">
                  <TextField
                    id="cpuRequest"
                    placeholder="e.g., 500m, 0.5, 1"
                    size="small"
                    fullWidth
                    disabled={isPending}
                    value={formData.cpuRequest}
                    onChange={(e) => handleFieldChange("cpuRequest", e.target.value)}
                    error={!!errors.cpuRequest}
                    helperText={errors.cpuRequest || "e.g., 100m, 0.5, 1.5"}
                  />
                </Form.ElementWrapper>
                <Form.ElementWrapper label="Memory request" name="memoryRequest">
                  <TextField
                    id="memoryRequest"
                    placeholder="e.g., 512Mi, 1Gi"
                    size="small"
                    fullWidth
                    disabled={isPending}
                    value={formData.memoryRequest}
                    onChange={(e) => handleFieldChange("memoryRequest", e.target.value)}
                    error={!!errors.memoryRequest}
                    helperText={errors.memoryRequest || "e.g., 256Mi, 512Mi, 1Gi"}
                  />
                </Form.ElementWrapper>
              </Form.Stack>
            </Form.Section>

            <Form.Section>
              <Form.Header>Resource Limits (optional)</Form.Header>
              <Form.Stack spacing={2}>
                <Form.ElementWrapper label="CPU limit" name="cpuLimit">
                  <TextField
                    id="cpuLimit"
                    placeholder="e.g., 1, 2"
                    size="small"
                    fullWidth
                    disabled={isPending}
                    value={formData.cpuLimit}
                    onChange={(e) => handleFieldChange("cpuLimit", e.target.value)}
                    error={!!errors.cpuLimit}
                    helperText={errors.cpuLimit}
                  />
                </Form.ElementWrapper>
                <Form.ElementWrapper label="Memory limit" name="memoryLimit">
                  <TextField
                    id="memoryLimit"
                    placeholder="e.g., 1Gi, 2Gi"
                    size="small"
                    fullWidth
                    disabled={isPending}
                    value={formData.memoryLimit}
                    onChange={(e) => handleFieldChange("memoryLimit", e.target.value)}
                    error={!!errors.memoryLimit}
                    helperText={errors.memoryLimit}
                  />
                </Form.ElementWrapper>
              </Form.Stack>
            </Form.Section>

            <Box display="flex" justifyContent="flex-end" gap={1} mt={2}>
              <Button
                variant="outlined"
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
                {isPending ? "Saving..." : "Save"}
              </Button>
            </Box>
          </Form.Stack>
        </form>
      </DrawerContent>
    </DrawerWrapper>
  );
}
