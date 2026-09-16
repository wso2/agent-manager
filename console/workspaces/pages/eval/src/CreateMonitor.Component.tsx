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

import React, { useCallback, useMemo } from "react";
import { generatePath, useNavigate, useParams, useSearchParams } from "react-router-dom";
import {
  absoluteRouteMap,
  type CreateMonitorRequest,
} from "@agent-management-platform/types";
import {
  useCreateMonitor,
  useGetMonitor,
} from "@agent-management-platform/api-client";
import { Alert, Skeleton, Stack } from "@wso2/oxygen-ui";
import { PageLayout } from "@agent-management-platform/views";
import {
  getErrorMessage,
  usePipelineEnvironments,
} from "@agent-management-platform/shared-component";
import { type CreateMonitorFormValues } from "./form/schema";
import { MonitorFormWizard } from "./subComponents/MonitorFormWizard";
import { slugifyMonitorName } from "./utils/monitorFormUtils";

export const CreateMonitorComponent: React.FC = () => {
  const { agentId, orgId, projectId, envId } = useParams<{
    agentId: string;
    orgId: string;
    projectId: string;
    envId: string;
  }>();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const duplicateFrom = searchParams.get("duplicateFrom") ?? undefined;

  const {
    mutate: createMonitor,
    isPending,
    error,
  } = useCreateMonitor({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });

  const {
    data: sourceMonitor,
    isLoading: isLoadingSource,
    error: sourceError,
  } = useGetMonitor({
    monitorName: duplicateFrom ?? "",
    orgName: orgId ?? "",
    projName: projectId ?? "",
    agentName: agentId ?? "",
  });

  const environments = usePipelineEnvironments(orgId, projectId);

  const defaultTimeRange = useMemo(() => {
    const end = new Date();
    const start = new Date(end.getTime() - 24 * 60 * 60 * 1000);
    return { start, end };
  }, []);

  const initialValues = useMemo<CreateMonitorFormValues>(() => {
    if (duplicateFrom && sourceMonitor) {
      const displayName = `${sourceMonitor.displayName} (Copy)`;
      const samplingRatePercent =
        sourceMonitor.samplingRate !== undefined
          ? Math.min(100, Math.max(1, Math.round(sourceMonitor.samplingRate * 100)))
          : 100;
      return {
        displayName,
        name: slugifyMonitorName(displayName),
        description: sourceMonitor.description ?? "",
        environmentName: sourceMonitor.environmentName,
        type: sourceMonitor.type,
        traceStart: sourceMonitor.traceStart ? new Date(sourceMonitor.traceStart) : null,
        traceEnd: sourceMonitor.traceEnd ? new Date(sourceMonitor.traceEnd) : null,
        intervalMinutes: sourceMonitor.intervalMinutes ?? undefined,
        samplingRate: samplingRatePercent,
        evaluators: sourceMonitor.evaluators ?? [],
        // llmProvider intentionally omitted — user must select/create a new provider
      };
    }

    return {
      displayName: "",
      name: "",
      description: "",
      environmentName: envId ?? "",
      type: "past",
      traceStart: defaultTimeRange.start,
      traceEnd: defaultTimeRange.end,
      intervalMinutes: 60,
      samplingRate: 100,
      evaluators: [],
    };
  }, [duplicateFrom, sourceMonitor, defaultTimeRange, envId]);

  const missingParamsMessage = useMemo(() => {
    if (!orgId) return "Organization is required to create a monitor.";
    if (!projectId) return "Project context is required.";
    if (!agentId) return "Select an agent before creating a monitor.";
    if (!envId) return "Select an environment before creating a monitor.";
    return null;
  }, [agentId, orgId, projectId, envId]);

  const backHref = useMemo(() => {
    if (!orgId || !projectId || !agentId || !envId) {
      return "#";
    }
    return generatePath(
      absoluteRouteMap.children.org.children.projects.children.agents.children
        .environment.children.evaluation.children.monitor.path,
      { orgId, projectId, agentId, envId },
    );
  }, [agentId, orgId, projectId, envId]);

  const handleCreateMonitor = useCallback(
    (values: CreateMonitorFormValues) => {
      if (!orgId || !projectId || !agentId || !envId) {
        return;
      }

      const payload: CreateMonitorRequest = {
        name: values.name.trim(),
        displayName: values.displayName.trim(),
        description: values.description?.trim() || undefined,
        environmentName: values.environmentName,
        evaluators: values.evaluators,
        llmProvider: values.llmProvider,
        type: values.type,
        intervalMinutes: values.intervalMinutes ?? undefined,
        traceStart: values.traceStart
          ? values.traceStart.toISOString()
          : undefined,
        traceEnd: values.traceEnd ? values.traceEnd.toISOString() : undefined,
        samplingRate: (values.samplingRate ?? 100) / 100,
      };

      createMonitor(payload, {
        onSuccess: () => {
          // Land on the monitor list for whichever environment it was actually
          // created in, which may differ from the page's originating envId.
          navigate(
            generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.evaluation.children.monitor
                .path,
              {
                orgId,
                projectId,
                agentId,
                envId: values.environmentName,
              },
            ),
          );
        },
      });
    },
    [agentId, createMonitor, envId, navigate, orgId, projectId],
  );

  const title = duplicateFrom && sourceMonitor
    ? `Duplicate "${sourceMonitor.displayName}"`
    : "Create Monitor";

  const description = duplicateFrom && sourceMonitor
    ? `Pre-filled from "${sourceMonitor.displayName}". Make your changes and submit to create a new monitor.`
    : undefined;

  if (duplicateFrom && isLoadingSource) {
    return (
      <PageLayout docs={["evaluationMonitors", "evaluation", "customEvaluators"]}
        title="Create Monitor"
        disableIcon
        backLabel="Back to Monitors"
        backHref={backHref}
      >
        <Stack spacing={3}>
          <Skeleton variant="rounded" height={60} />
          <Skeleton variant="rounded" height={360} />
        </Stack>
      </PageLayout>
    );
  }

  if (duplicateFrom && sourceError) {
    return (
      <PageLayout docs={["evaluationMonitors", "evaluation", "customEvaluators"]}
        title="Create Monitor"
        disableIcon
        backLabel="Back to Monitors"
        backHref={backHref}
      >
        <Alert severity="error">{getErrorMessage(sourceError)}</Alert>
      </PageLayout>
    );
  }

  return (
    <MonitorFormWizard
      title={title}
      description={description}
      backHref={backHref}
      submitLabel="Create Monitor"
      initialValues={initialValues}
      onSubmit={handleCreateMonitor}
      isSubmitting={isPending}
      serverError={error}
      missingParamsMessage={missingParamsMessage}
      environments={environments}
    />
  );
};

export default CreateMonitorComponent;
