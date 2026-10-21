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
  CircularProgress,
  Form,
  MenuItem,
  Select,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { generatePath, useParams } from "react-router-dom";
import { z } from "zod";
import { SlidersHorizontal } from "@wso2/oxygen-ui-icons-react";
import {
  TextInput,
  useFormValidation,
  useSnackBar,
} from "@agent-management-platform/views";
import {
  useTokenScopes,
  useGetProject,
  useListDeploymentPipelines,
  useListEnvironments,
  useUpdateProject,
} from "@agent-management-platform/api-client";
import {
  getErrorMessage,
  MarkdownEditor,
  PROJECT_UPDATE_SCOPE,
  RestrictedAction,
  useScopeAccess,
  useUnsavedChangesGuard,
} from "@agent-management-platform/shared-component";
import {
  absoluteRouteMap,
  type DeploymentPipelineResponse,
  INPUT_LIMITS,
  type UpdateProjectRequest,
} from "@agent-management-platform/types";
import { SettingsLayout, type SubNavSection } from "./SettingsLayout";

const projectSettingsRoute =
  absoluteRouteMap.children.org.children.projects.children.settings;

/**
 * A pipeline's promotion path as `dev → staging → prod`, using environment
 * display names. Null when the pipeline has no paths configured.
 */
function pipelineChainLabel(
  pipeline: DeploymentPipelineResponse,
  envDisplayNameMap: Map<string, string>,
): string | null {
  if (!pipeline.promotionPaths.length) return null;
  const names = [
    ...pipeline.promotionPaths.map((pp) => pp.sourceEnvironmentRef),
    pipeline.promotionPaths[pipeline.promotionPaths.length - 1]
      ?.targetEnvironmentRefs[0]?.name,
  ].filter(Boolean) as string[];
  return names.map((n) => envDisplayNameMap.get(n) ?? n).join(" → ");
}

/** A pipeline's name with its promotion chain beside it (row) or beneath it (column). */
function PipelineLabel({
  pipeline,
  envDisplayNameMap,
  direction,
}: {
  pipeline: DeploymentPipelineResponse;
  envDisplayNameMap: Map<string, string>;
  direction: "row" | "column";
}) {
  const chain = pipelineChainLabel(pipeline, envDisplayNameMap);
  return (
    <Stack
      direction={direction}
      alignItems={direction === "row" ? "center" : undefined}
      spacing={direction === "row" ? 1 : 0}
    >
      <Typography variant="body2">{pipeline.displayName}</Typography>
      {chain && (
        <Typography variant="caption" color="text.disabled">
          {chain}
        </Typography>
      )}
    </Stack>
  );
}

interface ProjectFormValues {
  displayName: string;
  description?: string;
  deploymentPipeline: string;
}

const projectFields = {
  displayName: z
    .string()
    .trim()
    .min(3, "Display name must be at least 3 characters")
    .max(100, "Display name must be at most 100 characters"),
  description: z.string().trim().optional(),
};

const projectSettingsSchema = z.object({
  ...projectFields,
  deploymentPipeline: z
    .string()
    .trim()
    .min(1, "Deployment pipeline is required"),
});

// Without pipeline read access the field is hidden, so its value is whatever
// the project already had: carried through untouched rather than demanded.
const projectSettingsSchemaNoPipeline = z.object({
  ...projectFields,
  deploymentPipeline: z.string().trim(),
});

/**
 * Project-scope Settings: edits display name, description, and the deployment
 * pipeline when the caller may read pipelines.
 */
export const SettingsProject: React.FC = () => {
  const { orgId, projectId } = useParams<{
    orgId: string;
    projectId: string;
  }>();
  const { pushSnackBar } = useSnackBar();

  // Pipelines are a separate resource: a caller may read the project and not
  // them. Hiding beats disabling here, since an empty list is indistinguishable
  // from "none exist" and would otherwise render as "Select a pipeline" over a
  // project that has one.
  const { enforced, resolved, scopes } = useTokenScopes();
  const scopesReady = !enforced || resolved;
  const canReadPipelines =
    !enforced || scopes.has("amp:deployment-pipeline:read");

  // Reading a project and updating one are separate scopes, so this page is
  // reachable by someone who may only look. The fields stay readable and the
  // save is refused with its reason, rather than the nav item disappearing.
  const updateAccess = useScopeAccess(
    PROJECT_UPDATE_SCOPE,
    "update this project",
  );
  const readOnly = !updateAccess.allowed;

  const {
    data: project,
    isLoading,
    error: loadError,
  } = useGetProject({ orgName: orgId, projName: projectId });

  const [formData, setFormData] = useState<ProjectFormValues | null>(null);
  const [initial, setInitial] = useState<ProjectFormValues | null>(null);

  const schema = useMemo(
    () =>
      canReadPipelines
        ? projectSettingsSchema
        : projectSettingsSchemaNoPipeline,
    [canReadPipelines],
  );
  const { errors, validateField, validateForm, clearErrors, setFieldError } =
    useFormValidation<ProjectFormValues>(schema);

  const { mutate: updateProject, isPending } = useUpdateProject({
    orgName: orgId,
    projName: project?.name,
  });

  const { data: pipelinesData, isLoading: isPipelinesLoading } =
    useListDeploymentPipelines({ orgName: orgId }, undefined, {
      enabled: canReadPipelines,
    });
  const pipelines = useMemo(
    () => pipelinesData?.deploymentPipelines ?? [],
    [pipelinesData?.deploymentPipelines],
  );

  const { data: environments } = useListEnvironments({ orgName: orgId });
  const envDisplayNameMap = useMemo(() => {
    const map = new Map<string, string>();
    (environments ?? []).forEach((e) =>
      map.set(e.name, e.displayName ?? e.name),
    );
    return map;
  }, [environments]);

  // Seeded once; re-seeding on every `project` identity change would wipe
  // in-progress edits each time the query refetched in the background.
  useEffect(() => {
    if (!project || initial) return;
    const saved: ProjectFormValues = {
      displayName: project.displayName,
      description: project.description || "",
      deploymentPipeline: project.deploymentPipeline || "",
    };
    setInitial(saved);
    setFormData(saved);
  }, [project, initial]);

  // Pre-select the first pipeline for a project that has none. It stands apart
  // from the seed rather than waiting inside it, because the list routinely
  // lands after the project does and a one-shot seed would then miss it for
  // good. `initial` deliberately keeps the project's empty value, so the
  // pre-selection shows up as an unsaved edit that can actually be saved.
  useEffect(() => {
    if (readOnly || pipelines.length === 0) return;
    setFormData((prev) =>
      prev && !prev.deploymentPipeline
        ? { ...prev, deploymentPipeline: pipelines[0].name }
        : prev,
    );
  }, [pipelines, readOnly]);

  const isDirty = useMemo(
    () =>
      !!formData &&
      !!initial &&
      JSON.stringify(formData) !== JSON.stringify(initial),
    [formData, initial],
  );
  useUnsavedChangesGuard(isDirty && !readOnly);

  const handleFieldChange = useCallback(
    (field: keyof ProjectFormValues, value: string) => {
      setFieldError(field, validateField(field, value));
      setFormData((prev) => (prev ? { ...prev, [field]: value } : prev));
    },
    [validateField, setFieldError],
  );

  const handleReset = useCallback(() => {
    setFormData(initial);
    clearErrors();
  }, [initial, clearErrors]);

  const handleSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault();
      if (!formData || !project || !validateForm(formData)) return;

      const payload: UpdateProjectRequest = {
        name: project.name,
        displayName: formData.displayName,
        description: formData.description,
        deploymentPipeline: formData.deploymentPipeline,
      };

      updateProject(payload, {
        onSuccess: () => {
          clearErrors();
          setInitial(formData);
          pushSnackBar({ message: "Project updated", type: "success" });
        },
        onError: (err: unknown) => {
          pushSnackBar({
            message: getErrorMessage(err) || "Failed to update project",
            type: "error",
          });
        },
      });
    },
    [formData, project, validateForm, updateProject, clearErrors, pushSnackBar],
  );

  const isValid =
    !!formData &&
    formData.displayName.trim().length >= 3 &&
    Object.keys(errors).length === 0;

  const renderBody = () => {
    if (loadError) {
      return (
        <Alert severity="error">
          {getErrorMessage(loadError) || "Failed to load project."}
        </Alert>
      );
    }

    if (isLoading || !scopesReady || !formData) {
      return (
        <Stack spacing={2}>
          <Skeleton variant="rounded" height={56} />
          <Skeleton variant="rounded" height={160} />
          <Skeleton variant="rounded" height={56} />
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
              <Typography variant="h5">Project Details</Typography>
              <Box display="flex" flexDirection="column" gap={1}>
                <TextInput
                  maxLength={INPUT_LIMITS.NAME}
                  placeholder="e.g., Customer Support Platform"
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
                  placeholder="Short description of this project. Markdown is supported."
                  disabled={isPending || readOnly}
                  value={formData.description || ""}
                  onChange={(value) => handleFieldChange("description", value)}
                  error={!!errors.description}
                  helperText={errors.description}
                />
                {canReadPipelines && (
                  <Form.ElementWrapper
                    label="Deployment Pipeline"
                    name="deploymentPipeline"
                  >
                    <Select
                      id="deploymentPipeline"
                      value={formData.deploymentPipeline}
                      onChange={(e) =>
                        handleFieldChange(
                          "deploymentPipeline",
                          e.target.value as string,
                        )
                      }
                      error={!!errors.deploymentPipeline}
                      disabled={isPipelinesLoading || isPending || readOnly}
                      displayEmpty
                      renderValue={(value) => {
                        const selected = pipelines.find(
                          (p) => p.name === value,
                        );
                        if (!selected) {
                          return (
                            <Typography variant="body2" color="text.disabled">
                              Select a pipeline
                            </Typography>
                          );
                        }
                        return (
                          <PipelineLabel
                            pipeline={selected}
                            envDisplayNameMap={envDisplayNameMap}
                            direction="row"
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
                            <Typography variant="caption">
                              Loading pipelines...
                            </Typography>
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
                          <PipelineLabel
                            pipeline={p}
                            envDisplayNameMap={envDisplayNameMap}
                            direction="column"
                          />
                        </MenuItem>
                      ))}
                    </Select>
                  </Form.ElementWrapper>
                )}
              </Box>
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
            <RestrictedAction decision={updateAccess} feature="update-project">
              <Button
                type="submit"
                variant="contained"
                color="primary"
                disabled={!isValid || isPending || !isDirty}
              >
                {isPending ? "Updating..." : "Update Project"}
              </Button>
            </RestrictedAction>
          </Box>
        </Box>
      </form>
    );
  };

  const sections: SubNavSection[] = [
    {
      title: "Project",
      items: [
        {
          label: "General",
          href: generatePath(projectSettingsRoute.path, { orgId, projectId }),
          wildPath: projectSettingsRoute.wildPath,
          icon: <SlidersHorizontal size={18} />,
        },
      ],
    },
  ];

  return <SettingsLayout sections={sections}>{renderBody()}</SettingsLayout>;
};

export default SettingsProject;
