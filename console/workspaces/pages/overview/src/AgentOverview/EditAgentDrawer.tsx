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

import { Box, Button, Card, CardContent, Typography } from "@wso2/oxygen-ui";
import { Edit } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerWrapper,
  DrawerHeader,
  DrawerContent,
  TextInput,
  useDrawerFullscreen,
  useFormValidation,
} from "@agent-management-platform/views";
import { z } from "zod";
import { useUpdateAgent } from "@agent-management-platform/api-client";
import { AgentResponse, UpdateAgentRequest, INPUT_LIMITS } from "@agent-management-platform/types";
import { LabelsEditor, MarkdownEditor } from "@agent-management-platform/shared-component";
import { useEffect, useState, useCallback } from "react";

interface EditAgentDrawerProps {
  open: boolean;
  onClose: () => void;
  agent: AgentResponse;
  orgId: string;
  projectId: string;
}

interface EditAgentFormValues {
  name: string;
  displayName: string;
  description?: string;
}

const editAgentSchema = z.object({
  displayName: z
    .string()
    .trim()
    .min(1, 'Display name is required')
    .min(3, 'Display name must be at least 3 characters')
    .max(100, 'Display name must be at most 100 characters'),
  name: z
    .string()
    .trim()
    .min(1, 'Name is required')
    .regex(/^[a-z0-9-]+$/, 'Name must be lowercase letters, numbers, and hyphens only (no spaces)')
    .min(3, 'Name must be at least 3 characters')
    .max(50, 'Name must be at most 50 characters'),
  description: z.string().trim().optional(),
});

export function EditAgentDrawer({ open, onClose, agent, orgId, projectId }: EditAgentDrawerProps) {
  const [formData, setFormData] = useState<EditAgentFormValues>({
    name: agent.name,
    displayName: agent.displayName,
    description: agent.description || '',
  });
  const [labels, setLabels] = useState<Record<string, string>>(agent.labels ?? {});
  const { isFullscreen, toggle: toggleFullscreen, reset: resetFullscreen } = useDrawerFullscreen();

  const { errors, validateField, validateForm, clearErrors, setFieldError } =
    useFormValidation<EditAgentFormValues>(editAgentSchema);

  const { mutate: updateAgent, isPending } = useUpdateAgent();

  // Reset form when agent changes or drawer opens
  useEffect(() => {
    if (open) {
      setFormData({
        name: agent.name,
        displayName: agent.displayName,
        description: agent.description || '',
      });
      setLabels(agent.labels ?? {});
      clearErrors();
      resetFullscreen();
    }
  }, [agent, open, clearErrors, resetFullscreen]);

  const handleFieldChange = useCallback((field: keyof EditAgentFormValues, value: string) => {
    const error = validateField(field, value);
    setFieldError(field, error);
    setFormData(prevData => ({ ...prevData, [field]: value }));
  }, [validateField, setFieldError]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    
    if (!validateForm(formData)) {
      return;
    }

    // Always send labels so removing the last one clears them ({} = clear,
    // absent = leave unchanged on the backend).
    const payload: UpdateAgentRequest = {
      displayName: formData.displayName,
      description: formData.description,
      labels,
    };

    updateAgent(
      {
        params: {
          orgName: orgId,
          projName: projectId,
          agentName: agent.name,
        },
        body: payload,
      },
      {
        onSuccess: () => {
          clearErrors();
          onClose();
        },
      }
    );
  };

  const isValid =
    !errors.displayName && !errors.description && formData.displayName.trim().length > 0;

  return (
    <DrawerWrapper open={open} onClose={onClose} fullscreen={isFullscreen}>
      <DrawerHeader
        icon={<Edit size={24} />}
        title="Edit Agent"
        onClose={onClose}
        isFullscreen={isFullscreen}
        onToggleFullscreen={toggleFullscreen}
      />
      <DrawerContent>
        <form onSubmit={handleSubmit}>
          <Box display="flex" flexDirection="column" gap={2} flexGrow={1}>
            <Card variant="outlined">
              <CardContent sx={{ gap: 1, display: "flex", flexDirection: "column" }}>
                <Typography variant="h5">Agent Details</Typography>
                <Box display="flex" flexDirection="column" gap={1}>
                  <TextInput
                    maxLength={INPUT_LIMITS.NAME}
                    placeholder="e.g., Customer Support Agent"
                    label="Name"
                    fullWidth
                    size="small"
                    value={formData.displayName}
                    onChange={(e) => handleFieldChange('displayName', e.target.value)}
                    error={!!errors.displayName}
                    helperText={errors.displayName}
                    disabled={isPending}
                  />
                  <MarkdownEditor
                    id="description"
                    label="Description (optional)"
                    placeholder="Short description of what this agent does. Markdown is supported."
                    value={formData.description || ''}
                    onChange={(value) => handleFieldChange('description', value)}
                    error={!!errors.description}
                    helperText={errors.description}
                    disabled={isPending}
                  />
                </Box>
              </CardContent>
            </Card>

            <Card variant="outlined">
              <CardContent sx={{ gap: 1, display: "flex", flexDirection: "column" }}>
                <LabelsEditor
                  value={labels}
                  onChange={setLabels}
                  disabled={isPending}
                />
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
                {isPending ? "Updating..." : "Update Agent"}
              </Button>
            </Box>
          </Box>
        </form>
      </DrawerContent>
    </DrawerWrapper>
  );
}
