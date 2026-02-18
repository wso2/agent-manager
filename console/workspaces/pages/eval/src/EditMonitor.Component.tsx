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
import { Alert, Skeleton, Stack } from "@wso2/oxygen-ui";
import { PageLayout } from "@agent-management-platform/views";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { absoluteRouteMap, type UpdateMonitorRequest } from "@agent-management-platform/types";
import { useGetMonitor, useUpdateMonitor } from "@agent-management-platform/api-client";
import { MonitorFormWizard } from "./subComponents/MonitorFormWizard";
import type { CreateMonitorFormValues } from "./form/schema";

export const EditMonitorComponent: React.FC = () => {
    const { orgId, projectId, agentId, monitorId } = useParams<{
        agentId: string;
        orgId: string;
        projectId: string;
        monitorId: string;
    }>();
    const navigate = useNavigate();

    const { data: monitorData, isLoading, error: fetchError } = useGetMonitor({
        monitorName: monitorId,
        orgName: orgId,
    });

    const { mutate: updateMonitor, isPending: isUpdating, error: updateError } = useUpdateMonitor({
        monitorName: monitorId,
        orgName: orgId,
    });

    const missingParamsMessage = useMemo(() => {
        if (!orgId) return "Organization is required to edit a monitor.";
        if (!projectId) return "Project context is required.";
        if (!agentId) return "Select an agent before editing a monitor.";
        if (!monitorId) return "Monitor identifier is missing.";
        return null;
    }, [agentId, monitorId, orgId, projectId]);

    const backHref = useMemo(() => {
        if (!orgId || !projectId || !agentId) {
            return "#";
        }
        return generatePath(
            absoluteRouteMap.children.org.children.projects.children.agents
                .children.evaluation.children.monitor.path,
            { orgId, projectId, agentId }
        );
    }, [agentId, orgId, projectId]);

    const initialValues = useMemo<CreateMonitorFormValues | null>(() => {
        if (!monitorData) {
            return null;
        }

        const toDate = (value?: string) => (value ? new Date(value) : null);
        const samplingRatePercent = monitorData.samplingRate !== undefined
            ? Math.min(100, Math.max(0, Math.round(monitorData.samplingRate * 100)))
            : undefined;

        return {
            displayName: monitorData.displayName ?? "",
            name: monitorData.name,
            description: "",
            type: monitorData.type,
            traceStart: toDate(monitorData.traceStart),
            traceEnd: toDate(monitorData.traceEnd),
            intervalMinutes: monitorData.intervalMinutes ?? undefined,
            samplingRate: samplingRatePercent,
            evaluators: monitorData.evaluators ?? [],
        };
    }, [monitorData]);

    const handleUpdateMonitor = useCallback((values: CreateMonitorFormValues) => {
        if (!orgId || !monitorId) {
            return;
        }

        const payload: UpdateMonitorRequest = {
            displayName: values.displayName.trim(),
            evaluators: values.evaluators,
            intervalMinutes: values.intervalMinutes ?? undefined,
            samplingRate: values.samplingRate !== undefined ? values.samplingRate / 100 : undefined,
        };

        updateMonitor(payload, {
            onSuccess: () => {
                navigate(backHref);
            },
        });
    }, [backHref, monitorId, navigate, orgId, updateMonitor]);

    if (missingParamsMessage) {
        return (
            <PageLayout
                title="Edit Monitor"
                description="Update monitor configuration and evaluator settings."
                disableIcon
                backLabel="Back to Monitors"
                backHref={backHref}
            >
                <Alert severity="error">
                    {missingParamsMessage}
                </Alert>
            </PageLayout>
        );
    }

    if (isLoading) {
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

    if (fetchError) {
        return (
            <PageLayout
                title="Edit Monitor"
                description="Update monitor configuration and evaluator settings."
                disableIcon
                backLabel="Back to Monitors"
                backHref={backHref}
            >
                <Alert severity="error">
                    {fetchError?.message? fetchError.message : "Failed to load monitor details."}
                </Alert>
            </PageLayout>
        );
    }

    if (!initialValues) {
        return (
            <PageLayout
                title="Edit Monitor"
                description="Update monitor configuration and evaluator settings."
                disableIcon
                backLabel="Back to Monitors"
                backHref={backHref}
            >
                <Alert severity="error">
                    Unable to load monitor details.
                </Alert>
            </PageLayout>
        );
    }

    return (
        <MonitorFormWizard
            title="Edit Monitor"
            description="Update monitor configuration and evaluator settings."
            backHref={backHref}
            submitLabel="Save Changes"
            initialValues={initialValues}
            onSubmit={handleUpdateMonitor}
            isSubmitting={isUpdating}
            serverError={updateError}
        />
    );
};

export default EditMonitorComponent;
