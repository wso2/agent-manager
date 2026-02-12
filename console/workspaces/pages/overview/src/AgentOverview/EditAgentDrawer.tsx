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
import { DrawerWrapper, DrawerHeader, DrawerContent, TextInput, useFormValidation } from "@agent-management-platform/views";
import { z } from "zod";
import { useUpdateAgent } from "@agent-management-platform/api-client";
import { AgentResponse, UpdateAgentRequest } from "@agent-management-platform/types";
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
      clearErrors();
    }
  }, [agent, open, clearErrors]);

  const handleFieldChange = useCallback((field: keyof EditAgentFormValues, value: string) => {
    setFormData(prevData => {
      const newData = { ...prevData, [field]: value };
      const error = validateField(field, value);
      setFieldError(field, error);
      return newData;
    });
  }, [validateField, setFieldError]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    
    if (!validateForm(formData)) {
      return;
    }

    const payload: UpdateAgentRequest = {
      displayName: formData.displayName,
      description: formData.description,
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
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader
        icon={<Edit size={24} />}
        title="Edit Agent"
        onClose={onClose}
      />
      <DrawerContent>
        <form onSubmit={handleSubmit}>
          <Box display="flex" flexDirection="column" gap={2} flexGrow={1}>
            <Card variant="outlined">
              <CardContent sx={{ gap: 1, display: "flex", flexDirection: "column" }}>
                <Typography variant="h5">Agent Details</Typography>
                <Box display="flex" flexDirection="column" gap={1}>
                  <TextInput
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
                  <TextInput
                    placeholder="Short description of what this agent does"
                    label="Description (optional)"
                    fullWidth
                    size="small"
                    multiline
                    minRows={2}
                    maxRows={6}
                    value={formData.description || ''}
                    onChange={(e) => handleFieldChange('description', e.target.value)}
                    error={!!errors.description}
                    helperText={errors.description}
                    disabled={isPending}
                  />
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
                {isPending ? "Updating..." : "Update Agent"}
              </Button>
            </Box>
          </Box>
        </form>
      </DrawerContent>
    </DrawerWrapper>
  );
}
