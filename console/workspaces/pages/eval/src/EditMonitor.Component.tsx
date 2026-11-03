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
import { getErrorMessage } from "@agent-management-platform/shared-component";
import { Alert, Skeleton, Stack } from "@wso2/oxygen-ui";
import { PageLayout } from "@agent-management-platform/views";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  absoluteRouteMap,
  type UpdateMonitorRequest,
} from "@agent-management-platform/types";
import {
  useApplyMonitorAlert,
  useGetMonitor,
  useMonitorAlert,
  useEventsEnabled,
  useUpdateMonitor,
} from "@agent-management-platform/api-client";
import {
  MonitorFormWizard,
  type MonitorAlertChoice,
} from "./subComponents/MonitorFormWizard";
import {
  toAlertRequest,
  alertSettingsFromResponse,
  type AlertSettingsValue,
} from "./subComponents/MonitorAlertSettings";
import type { CreateMonitorFormValues } from "./form/schema";

export const EditMonitorComponent: React.FC = () => {
  const { orgId, projectId, agentId, envId, monitorId } = useParams<{
    agentId: string;
    orgId: string;
    projectId: string;
    envId: string;
    monitorId: string;
  }>();
  const navigate = useNavigate();

  const {
    data: monitorData,
    isLoading,
    error: fetchError,
  } = useGetMonitor({
    monitorName: monitorId ?? "",
    orgName: orgId ?? "",
    projName: projectId ?? "",
    agentName: agentId ?? "",
  });

  const {
    mutate: updateMonitor,
    isPending: isUpdating,
    error: updateError,
  } = useUpdateMonitor({
    monitorName: monitorId ?? "",
    orgName: orgId ?? "",
    projName: projectId ?? "",
    agentName: agentId ?? "",
  });

  const alertsEnabled = useEventsEnabled();
  const { data: currentAlert, isLoading: isLoadingAlert, error: alertError } = useMonitorAlert(
    { orgName: orgId, projName: projectId, agentName: agentId, monitorName: monitorId },
    alertsEnabled,
  );
  const { mutate: applyAlert, isPending: isSavingAlert } = useApplyMonitorAlert({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });
  const initialAlert = useMemo<AlertSettingsValue | null>(
    () => (currentAlert?.enabled ? alertSettingsFromResponse(currentAlert) : null),
    [currentAlert],
  );

  const missingParamsMessage = useMemo(() => {
    if (!orgId) return "Organization is required to edit a monitor.";
    if (!projectId) return "Project context is required.";
    if (!agentId) return "Select an agent before editing a monitor.";
    if (!monitorId) return "Monitor identifier is missing.";
    return null;
  }, [agentId, monitorId, orgId, projectId]);

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

  const initialValues = useMemo<CreateMonitorFormValues | null>(() => {
    if (!monitorData) {
      return null;
    }

    const toDate = (value?: string) => (value ? new Date(value) : null);
    const samplingRatePercent =
      monitorData.samplingRate !== undefined
        ? Math.min(100, Math.max(0, Math.round(monitorData.samplingRate * 100)))
        : undefined;

    return {
      displayName: monitorData.displayName ?? "",
      name: monitorData.name,
      description: "",
      environmentName: monitorData.environmentName,
      type: monitorData.type,
      traceStart: toDate(monitorData.traceStart),
      traceEnd: toDate(monitorData.traceEnd),
      intervalMinutes: monitorData.intervalMinutes ?? undefined,
      samplingRate: samplingRatePercent,
      evaluators: monitorData.evaluators ?? [],
      llmProvider: monitorData.llmProvider
        ? { providerName: monitorData.llmProvider.providerName }
        : undefined,
    };
  }, [monitorData]);

  const handleUpdateMonitor = useCallback(
    (
      values: CreateMonitorFormValues,
      allowNavigation: (action: () => void) => void,
      alert: MonitorAlertChoice,
    ) => {
      if (!orgId || !projectId || !agentId || !monitorId) {
        return;
      }

      // Only include llmProvider in the PATCH when the user explicitly changed it.
      // If the backend returned null (e.g. metadata resolution failure) and the user
      // didn't touch the field, omitting it prevents accidental deprovisioning.
      const initialProviderName = initialValues?.llmProvider?.providerName;
      const currentProviderName = values.llmProvider?.providerName;
      const llmProviderChanged = currentProviderName !== initialProviderName;

      const payload: UpdateMonitorRequest = {
        displayName: values.displayName.trim(),
        evaluators: values.evaluators,
        ...(llmProviderChanged
          ? { llmProvider: values.llmProvider ?? null }
          : {}),
        intervalMinutes: values.intervalMinutes ?? undefined,
        traceStart: values.traceStart?.toISOString(),
        traceEnd: values.traceEnd?.toISOString(),
        samplingRate:
          values.samplingRate !== undefined
            ? values.samplingRate / 100
            : undefined,
      };

      // Only a changed alert is saved: an untouched form leaves it as it is.
      const alertRequest = alert ? toAlertRequest(alert) : null;
      const initialRequest = initialAlert ? toAlertRequest(initialAlert) : null;
      const alertChanged =
        alert !== undefined && JSON.stringify(alertRequest) !== JSON.stringify(initialRequest);

      updateMonitor(payload, {
        onSuccess: () => {
          const goBack = () => allowNavigation(() => navigate(backHref));
          if (alertChanged) {
            applyAlert({ monitorName: monitorId, alert: alertRequest }, { onSettled: goBack });
            return;
          }
          goBack();
        },
      });
    },
    [
      agentId, applyAlert, backHref, initialValues, initialAlert, monitorId, navigate, orgId,
      projectId, updateMonitor,
    ],
  );

  if (missingParamsMessage) {
    return (
      <PageLayout
        title="Edit Monitor"
        description="Update monitor configuration and evaluator settings."
        disableIcon
        backLabel="Back to Monitors"
        backHref={backHref}
      >
        <Alert severity="error">{missingParamsMessage}</Alert>
      </PageLayout>
    );
  }

  if (isLoading || (alertsEnabled && isLoadingAlert)) {
    return (
      <PageLayout
        title="Edit Monitor"
        description="Update monitor configuration and evaluator settings."
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

  if (fetchError || (alertsEnabled && alertError)) {
    return (
      <PageLayout
        title="Edit Monitor"
        description="Update monitor configuration and evaluator settings."
        disableIcon
        backLabel="Back to Monitors"
        backHref={backHref}
      >
        <Alert severity="error">
          {fetchError
            ? getErrorMessage(fetchError)
            : "Failed to load alert settings. Please try again."}
        </Alert>
      </PageLayout>
    );
  }

  if (!initialValues) {
    return (
      <PageLayout
        title="Edit Monitor"
        disableIcon
        backLabel="Back to Monitors"
        backHref={backHref}
      >
        <Alert severity="error">Unable to load monitor details.</Alert>
      </PageLayout>
    );
  }

  return (
    <MonitorFormWizard
      title={"Edit " + (monitorData?.displayName ?? "Monitor")}
      backHref={backHref}
      submitLabel="Save Changes"
      initialValues={initialValues}
      onSubmit={handleUpdateMonitor}
      isSubmitting={isUpdating || isSavingAlert}
      serverError={updateError}
      isTypeEditable={false}
      initialAlert={initialAlert}
    />
  );
};

export default EditMonitorComponent;
