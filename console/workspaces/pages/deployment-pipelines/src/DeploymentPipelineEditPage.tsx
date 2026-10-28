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
  ListingTable,
  Skeleton,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { AlertTriangle, GitBranch } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  PageLayout,
  useFormValidation,
} from "@agent-management-platform/views";
import {
  useListDeploymentPipelines,
  useListEnvironments,
  useUpdateOrgDeploymentPipeline,
} from "@agent-management-platform/api-client";
import {
  getErrorMessage,
  useUnsavedChangesGuard,
} from "@agent-management-platform/shared-component";
import {
  absoluteRouteMap,
  type DeploymentPipelineResponse,
  INPUT_LIMITS,
} from "@agent-management-platform/types";
import {
  editPipelineSchema,
  type EditPipelineFormValues,
  PIPELINE_DISPLAY_NAME_MAX_LENGTH,
} from "./form/schema";
import { chainToPromotionPaths } from "./utils/chainUtils";
import { validatePromotionChain } from "./utils/validatePromotionChain";
import { PipelineChainEditor } from "./subComponents/PipelineChainEditor";

function pipelineToChain(pipeline: DeploymentPipelineResponse): string[] {
  if (pipeline.promotionPaths.length === 0) return [""];
  const validation = validatePromotionChain(pipeline.promotionPaths);
  if (validation.valid && validation.chain && validation.chain.length >= 1) {
    return validation.chain;
  }
  // Fallback for invalid existing paths: collect sources + last target
  const sources = pipeline.promotionPaths.map((p) => p.sourceEnvironmentRef);
  const lastTarget =
    pipeline.promotionPaths[pipeline.promotionPaths.length - 1]
      ?.targetEnvironmentRefs[0]?.name ?? "";
  return [...sources, lastTarget];
}

export function DeploymentPipelineEditPage() {
  const { orgId, pipelineName } = useParams<{
    orgId: string;
    pipelineName: string;
  }>();
  const navigate = useNavigate();

  const listHref = orgId
    ? generatePath(
        absoluteRouteMap.children.org.children.deploymentPipelines.path,
        { orgId },
      )
    : "#";

  // There is no org-scoped get-by-name endpoint, so the pipeline comes from the
  // same list query the table already populated — arriving from a row is a cache hit.
  const {
    data: pipelinesData,
    isLoading,
    error: loadError,
  } = useListDeploymentPipelines({ orgName: orgId });

  const pipeline = useMemo(
    () =>
      pipelinesData?.deploymentPipelines.find((p) => p.name === pipelineName),
    [pipelinesData, pipelineName],
  );

  const [formData, setFormData] = useState<EditPipelineFormValues | null>(null);
  const [initialSnapshot, setInitialSnapshot] = useState<string | null>(null);

  const { errors, validateForm, setFieldError, validateField } =
    useFormValidation<EditPipelineFormValues>(editPipelineSchema);

  const {
    mutateAsync: updatePipeline,
    isPending: isUpdating,
    error: updateError,
  } = useUpdateOrgDeploymentPipeline();

  const { data: environments } = useListEnvironments({ orgName: orgId });
  const envOptions = useMemo(() => environments ?? [], [environments]);

  // Seeded once the list resolves; re-seeding on every `pipeline` identity
  // change would discard edits each time the list query refetched.
  useEffect(() => {
    if (!pipeline || initialSnapshot !== null) return;
    const seed: EditPipelineFormValues = {
      displayName: pipeline.displayName,
      description: pipeline.description ?? "",
      chain: pipelineToChain(pipeline),
    };
    setFormData(seed);
    setInitialSnapshot(JSON.stringify(seed));
  }, [pipeline, initialSnapshot]);

  const isDirty = useMemo(
    () =>
      formData !== null &&
      initialSnapshot !== null &&
      JSON.stringify(formData) !== initialSnapshot,
    [formData, initialSnapshot],
  );
  const { allowNavigation } = useUnsavedChangesGuard(isDirty);

  const handleFieldChange = useCallback(
    (field: "displayName" | "description", value: string) => {
      setFormData((prev) => {
        if (!prev) return prev;
        const next = { ...prev, [field]: value };
        setFieldError(
          field,
          validateField(
            field,
            next[field as keyof EditPipelineFormValues],
            next,
          ),
        );
        return next;
      });
    },
    [setFieldError, validateField],
  );

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      if (!formData || !pipeline) return;

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
        // isDirty is still true in this tick, so the guard needs the bypass.
        allowNavigation(() => navigate(listHref));
      } catch {
        // surfaced through updateError
      }
    },
    [
      formData,
      pipeline,
      validateForm,
      updatePipeline,
      orgId,
      allowNavigation,
      navigate,
      listHref,
    ],
  );

  const errorMessage = updateError
    ? ((updateError as Error)?.message ?? "Failed to update pipeline")
    : null;

  const renderBody = () => {
    if (loadError) {
      return (
        <ListingTable.Container>
          <Alert
            severity="error"
            icon={<AlertTriangle size={18} />}
            sx={{ alignSelf: "stretch" }}
          >
            Failed to load pipeline.{" "}
            {getErrorMessage(loadError) || "Please try again."}
          </Alert>
        </ListingTable.Container>
      );
    }

    if (isLoading || (!pipeline && !pipelinesData)) {
      return (
        <Stack spacing={2}>
          <Skeleton variant="rounded" height={40} />
          <Skeleton variant="rounded" height={72} />
          <Skeleton variant="rounded" height={120} />
        </Stack>
      );
    }

    if (!pipeline) {
      return (
        <ListingTable.Container>
          <ListingTable.EmptyState
            illustration={<GitBranch size={64} />}
            title="Pipeline not found"
            description={`No deployment pipeline named "${pipelineName}" exists in this organization.`}
            action={
              <Button variant="contained" onClick={() => navigate(listHref)}>
                Back to Deployment Pipelines
              </Button>
            }
          />
        </ListingTable.Container>
      );
    }

    if (!formData) return null;

    const allFilled = formData.chain.every((v) => v !== "");

    return (
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
                  slotProps={{
                    htmlInput: { maxLength: PIPELINE_DISPLAY_NAME_MAX_LENGTH },
                  }}
                  fullWidth
                  size="small"
                  value={formData.displayName}
                  onChange={(e) =>
                    handleFieldChange("displayName", e.target.value)
                  }
                  placeholder="e.g., Production Pipeline"
                  error={Boolean(errors.displayName)}
                  helperText={errors.displayName}
                  disabled={isUpdating}
                />
              </FormControl>
              <FormControl fullWidth>
                <FormLabel>Description</FormLabel>
                <TextField
                  slotProps={{
                    htmlInput: { maxLength: INPUT_LIMITS.DESCRIPTION },
                  }}
                  fullWidth
                  size="small"
                  multiline
                  rows={2}
                  value={formData.description ?? ""}
                  onChange={(e) =>
                    handleFieldChange("description", e.target.value)
                  }
                  placeholder="Optional description"
                  disabled={isUpdating}
                />
              </FormControl>
            </Form.Stack>
          </Form.Section>

          <PipelineChainEditor
            chain={formData.chain}
            envOptions={envOptions}
            onChange={(chain) =>
              setFormData((prev) => (prev ? { ...prev, chain } : prev))
            }
            disabled={isUpdating}
          />

          <Box display="flex" justifyContent="flex-end" gap={1} mt={2}>
            <Button
              variant="outlined"
              color="inherit"
              onClick={() => navigate(listHref)}
              disabled={isUpdating}
            >
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
    );
  };

  return (
    <PageLayout
      title={pipeline?.displayName ?? pipelineName ?? "Deployment Pipeline"}
      description={pipeline?.description || undefined}
      backHref={listHref}
      backLabel="Back to Deployment Pipelines"
      isLoading={isLoading}
      disableIcon
    >
      {renderBody()}
    </PageLayout>
  );
}

export default DeploymentPipelineEditPage;
