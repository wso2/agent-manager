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
import { DrawerWrapper, DrawerHeader, DrawerContent, TextInput } from "@agent-management-platform/views";
import { useForm, FormProvider } from "react-hook-form";
import { yupResolver } from "@hookform/resolvers/yup";
import * as yup from "yup";
import { useUpdateAgent } from "@agent-management-platform/api-client";
import { AgentResponse, UpdateAgentRequest } from "@agent-management-platform/types";
import { useEffect } from "react";

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

const editAgentSchema = yup.object({
  displayName: yup
    .string()
    .trim()
    .required('Display name is required')
    .min(3, 'Display name must be at least 3 characters')
    .max(100, 'Display name must be at most 100 characters'),
  name: yup
    .string()
    .trim()
    .required('Name is required')
    .matches(/^[a-z0-9-]+$/, 'Name must be lowercase letters, numbers, and hyphens only (no spaces)')
    .min(3, 'Name must be at least 3 characters')
    .max(50, 'Name must be at most 50 characters'),
  description: yup.string().trim(),
});

export function EditAgentDrawer({ open, onClose, agent, orgId, projectId }: EditAgentDrawerProps) {
  const methods = useForm<EditAgentFormValues>({
    resolver: yupResolver(editAgentSchema),
    defaultValues: {
      name: agent.name,
      displayName: agent.displayName,
      description: agent.description || '',
    },
  });

  const { mutate: updateAgent, isPending } = useUpdateAgent();

  // Reset form when agent changes
  useEffect(() => {
    methods.reset({
      name: agent.name,
      displayName: agent.displayName,
      description: agent.description || '',
    });
  }, [agent, methods]);

  const handleSubmit = (data: EditAgentFormValues) => {
    const payload: UpdateAgentRequest = {
      name: data.name,
      displayName: data.displayName,
      description: data.description,
      provisioning: agent.provisioning,
      agentType: agent.agentType,
      runtimeConfigs: agent.runtimeConfigs,
      inputInterface: agent.inputInterface,
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
          onClose();
        },
      }
    );
  };

  return (
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader
        icon={<Edit size={24} />}
        title="Edit Agent"
        onClose={onClose}
      />
      <DrawerContent>
        <FormProvider {...methods}>
          <form onSubmit={methods.handleSubmit(handleSubmit)}>
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
                      error={!!methods.formState.errors.displayName}
                      helperText={
                        methods.formState.errors.displayName?.message as string
                      }
                      {...methods.register("displayName")}
                    />
                    <TextInput
                      placeholder="Short description of what this agent does"
                      label="Description (optional)"
                      fullWidth
                      size="small"
                      multiline
                      minRows={2}
                      maxRows={6}
                      error={!!methods.formState.errors.description}
                      helperText={methods.formState.errors.description?.message as string}
                      {...methods.register("description")}
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
                  disabled={isPending}
                >
                  {isPending ? "Updating..." : "Update Agent"}
                </Button>
              </Box>
            </Box>
          </form>
        </FormProvider>
      </DrawerContent>
    </DrawerWrapper>
  );
}
