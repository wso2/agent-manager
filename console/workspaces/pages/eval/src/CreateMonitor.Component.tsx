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
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { absoluteRouteMap, type CreateMonitorRequest } from "@agent-management-platform/types";
import { useCreateMonitor, useListEnvironments } from "@agent-management-platform/api-client";
import { type CreateMonitorFormValues } from "./form/schema";
import { MonitorFormWizard } from "./subComponents/MonitorFormWizard";

export const CreateMonitorComponent: React.FC = () => {
    const { agentId, orgId, projectId } = useParams<{
        agentId: string, orgId: string, projectId: string
    }>();
    const navigate = useNavigate();
    const { data, isLoading: isEnvironmentsLoading } = useListEnvironments({
        orgName: orgId,
    });

    const envId = data && data.length > 0 ? data[0].name : undefined;

    const { mutate: createMonitor, isPending, error } = useCreateMonitor({
        orgName: orgId,
        projName: projectId,
        agentName: agentId,
    });

    const defaultTimeRange = useMemo(() => {
        const end = new Date();
        const start = new Date(end.getTime() - 24 * 60 * 60 * 1000);
        return { start, end };
    }, []);

    const initialValues = useMemo<CreateMonitorFormValues>(() => ({
        displayName: "",
        name: "",
        description: "",
        type: "past",
        traceStart: defaultTimeRange.start,
        traceEnd: defaultTimeRange.end,
        intervalMinutes: 60,
        samplingRate: 25,
        evaluators: [],
    }), [defaultTimeRange]);

    const missingParamsMessage = useMemo(() => {
        if (!orgId) return "Organization is required to create a monitor.";
        if (!projectId) return "Project context is required.";
        if (!agentId) return "Select an agent before creating a monitor.";
        if (!envId && !isEnvironmentsLoading) return "Select an environment before creating a monitor.";
        return null;
    }, [agentId, orgId, projectId, envId, isEnvironmentsLoading]);
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

    const handleCreateMonitor = useCallback((values: CreateMonitorFormValues) => {
        if (!orgId || !projectId || !agentId || !envId) {
            return;
        }

        const payload: CreateMonitorRequest = {
            name: values.name.trim(),
            displayName: values.displayName.trim(),
            projectName: projectId,
            agentName: agentId,
            environmentName: envId,
            evaluators: values.evaluators,
            type: values.type,
            intervalMinutes: values.intervalMinutes ?? undefined,
            traceStart: values.traceStart ? values.traceStart.toISOString() : undefined,
            traceEnd: values.traceEnd ? values.traceEnd.toISOString() : undefined,
            samplingRate: (values.samplingRate ?? 0) / 100,
        };

        createMonitor(payload, {
            onSuccess: () => {
                navigate(backHref);
            },
        });
    }, [agentId, backHref, createMonitor, envId, navigate, orgId, projectId]);

    return (
        <MonitorFormWizard
            title="Create Monitor"
            description="Define a new eval monitor by selecting targets, thresholds, and alerting rules."
            backHref={backHref}
            submitLabel="Create Monitor"
            initialValues={initialValues}
            onSubmit={handleCreateMonitor}
            isSubmitting={isPending}
            serverError={error}
            missingParamsMessage={missingParamsMessage}
        />
    );
};

export default CreateMonitorComponent;
