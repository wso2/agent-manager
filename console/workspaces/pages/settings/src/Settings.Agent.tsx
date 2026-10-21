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
  Card,
  CardContent,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { generatePath, useParams } from "react-router-dom";
import { z } from "zod";
import {
  TextInput,
  useFormValidation,
  useSnackBar,
} from "@agent-management-platform/views";
import { SlidersHorizontal } from "@wso2/oxygen-ui-icons-react";
import {
  useGetAgent,
  useUpdateAgent,
} from "@agent-management-platform/api-client";
import {
  AGENT_UPDATE_SCOPE,
  getErrorMessage,
  LabelsEditor,
  MarkdownEditor,
  RestrictedAction,
  useScopeAccess,
  useUnsavedChangesGuard,
} from "@agent-management-platform/shared-component";
import {
  absoluteRouteMap,
  INPUT_LIMITS,
  type UpdateAgentRequest,
} from "@agent-management-platform/types";
import { SettingsLayout, type SubNavSection } from "./SettingsLayout";

const agentSettingsRoute =
  absoluteRouteMap.children.org.children.projects.children.agents.children
    .settings;

interface AgentFormValues {
  displayName: string;
  description?: string;
  labels: Record<string, string>;
}

const agentSettingsSchema = z.object({
  displayName: z
    .string()
    .trim()
    .min(1, "Display name is required")
    .min(3, "Display name must be at least 3 characters")
    .max(100, "Display name must be at most 100 characters"),
  description: z.string().trim().optional(),
  // Edited as a unit by LabelsEditor; present so the schema matches the form.
  labels: z.record(z.string(), z.string()),
});

/** Agent-scope Settings: edits display name, description and labels. */
export const SettingsAgent: React.FC = () => {
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();
  const { pushSnackBar } = useSnackBar();

  // Reading an agent and updating one are separate scopes, so this page is
  // reachable by someone who may only look. The fields stay readable and the
  // save is refused with its reason, rather than the nav item disappearing.
  const updateAccess = useScopeAccess(AGENT_UPDATE_SCOPE, "update this agent");
  const readOnly = !updateAccess.allowed;

  const {
    data: agent,
    isLoading,
    error: loadError,
  } = useGetAgent({ orgName: orgId, projName: projectId, agentName: agentId });

  const [formData, setFormData] = useState<AgentFormValues | null>(null);
  const [initial, setInitial] = useState<AgentFormValues | null>(null);

  const { errors, validateField, validateForm, clearErrors, setFieldError } =
    useFormValidation<AgentFormValues>(agentSettingsSchema);

  const { mutate: updateAgent, isPending } = useUpdateAgent();

  // Seeded once; re-seeding on every `agent` identity change would wipe
  // in-progress edits each time the query refetched in the background.
  useEffect(() => {
    if (!agent || initial) return;
    const seed: AgentFormValues = {
      displayName: agent.displayName,
      description: agent.description || "",
      labels: agent.labels ?? {},
    };
    setFormData(seed);
    setInitial(seed);
  }, [agent, initial]);

  const isDirty = useMemo(
    () =>
      !!formData &&
      !!initial &&
      JSON.stringify(formData) !== JSON.stringify(initial),
    [formData, initial],
  );
  useUnsavedChangesGuard(isDirty);

  const handleFieldChange = useCallback(
    (field: keyof AgentFormValues, value: string) => {
      setFieldError(field, validateField(field, value));
      setFormData((prev) => (prev ? { ...prev, [field]: value } : prev));
    },
    [validateField, setFieldError],
  );

  const handleReset = useCallback(() => {
    setFormData(initial);
    clearErrors();
  }, [initial, clearErrors]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!formData || !agent || !validateForm(formData)) return;

    // Always send labels so removing the last one clears them ({} = clear,
    // absent = leave unchanged on the backend).
    const payload: UpdateAgentRequest = {
      displayName: formData.displayName,
      description: formData.description,
      labels: formData.labels,
    };

    updateAgent(
      {
        params: {
          orgName: orgId ?? "default",
          projName: projectId ?? "default",
          agentName: agent.name,
        },
        body: payload,
      },
      {
        onSuccess: () => {
          clearErrors();
          setInitial(formData);
          pushSnackBar({ message: "Agent updated", type: "success" });
        },
        onError: (err: unknown) => {
          pushSnackBar({
            message: getErrorMessage(err) || "Failed to update agent",
            type: "error",
          });
        },
      },
    );
  };

  const isValid =
    !!formData &&
    !errors.displayName &&
    !errors.description &&
    formData.displayName.trim().length > 0;

  const renderBody = () => {
    if (loadError) {
      return (
        <Alert severity="error">
          {getErrorMessage(loadError) || "Failed to load agent."}
        </Alert>
      );
    }

    if (isLoading || !formData) {
      return (
        <Stack spacing={2}>
          <Skeleton variant="rounded" height={56} />
          <Skeleton variant="rounded" height={160} />
          <Skeleton variant="rounded" height={120} />
        </Stack>
      );
    }

    return (
      <form onSubmit={handleSubmit}>
        <Box display="flex" flexDirection="column" gap={2} flexGrow={1}>
          {readOnly && (
            <Alert severity="info">{updateAccess.reason}</Alert>
          )}
          <Card variant="outlined">
            <CardContent
              sx={{ gap: 1, display: "flex", flexDirection: "column" }}
            >
              <Typography variant="h5">Agent Details</Typography>
              <Box display="flex" flexDirection="column" gap={1}>
                <TextInput
                  maxLength={INPUT_LIMITS.NAME}
                  placeholder="e.g., Customer Support Agent"
                  label="Name"
                  fullWidth
                  size="small"
                  value={formData.displayName}
                  onChange={(e) =>
                    handleFieldChange("displayName", e.target.value)
                  }
                  error={!!errors.displayName}
                  helperText={errors.displayName}
                  disabled={isPending || readOnly}
                />
                <MarkdownEditor
                  id="description"
                  label="Description (optional)"
                  placeholder="Short description of what this agent does. Markdown is supported."
                  value={formData.description || ""}
                  onChange={(value) => handleFieldChange("description", value)}
                  error={!!errors.description}
                  helperText={errors.description}
                  disabled={isPending || readOnly}
                />
              </Box>
            </CardContent>
          </Card>

          <Card variant="outlined">
            <CardContent
              sx={{ gap: 1, display: "flex", flexDirection: "column" }}
            >
              <LabelsEditor
                value={formData.labels}
                onChange={(value) =>
                  setFormData((prev) =>
                    prev ? { ...prev, labels: value } : prev,
                  )
                }
                disabled={isPending || readOnly}
              />
            </CardContent>
          </Card>

          <Box display="flex" justifyContent="flex-end" gap={1} mt={2}>
            <Button
              variant="outlined"
              color="inherit"
              onClick={handleReset}
              disabled={isPending || !isDirty}
            >
              Discard Changes
            </Button>
            <RestrictedAction decision={updateAccess} feature="update-agent">
              <Button
                type="submit"
                variant="contained"
                color="primary"
                disabled={!isValid || isPending || !isDirty}
              >
                {isPending ? "Updating..." : "Update Agent"}
              </Button>
            </RestrictedAction>
          </Box>
        </Box>
      </form>
    );
  };

  const sections: SubNavSection[] = [
    {
      title: "Agent",
      items: [
        {
          label: "General",
          href: generatePath(agentSettingsRoute.path, {
            orgId,
            projectId,
            agentId,
          }),
          wildPath: agentSettingsRoute.wildPath,
          icon: <SlidersHorizontal size={18} />,
        },
      ],
    },
  ];

  return <SettingsLayout sections={sections}>{renderBody()}</SettingsLayout>;
};

export default SettingsAgent;
