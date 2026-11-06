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

import { useQueryClient } from "@tanstack/react-query";
import {
  deleteMonitorAlert,
  getMonitorAlert,
  putMonitorAlert,
} from "../apis";
import type {
  MonitorAlertPathParams,
  MonitorAlertRequest,
  MonitorAlertResponse,
} from "@agent-management-platform/types";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiMutation, useApiQuery } from "./react-query-notifications";


function alertReady(p: MonitorAlertPathParams): boolean {
  return !!(p.orgName && p.projName && p.agentName && p.monitorName);
}

/** Load shared monitor alert settings. */
export function useMonitorAlert(params: MonitorAlertPathParams, enabled = true) {
  const { getToken } = useAuthHooks();
  return useApiQuery<MonitorAlertResponse>({
    queryKey: ["monitor-alert", params],
    queryFn: () => getMonitorAlert(params, getToken),
    enabled: enabled && alertReady(params),
  });
}

/** Save shared monitor alert settings. */
export function useSaveMonitorAlert(params: MonitorAlertPathParams) {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<MonitorAlertResponse, unknown, MonitorAlertRequest>({
    successMessage: "Alert settings saved",
    mutationFn: (body) => putMonitorAlert(params, body, getToken),
    onSuccess: (data) => queryClient.setQueryData(["monitor-alert", params], data),
  });
}

/** Disable monitor alerting. */
export function useDeleteMonitorAlert(params: MonitorAlertPathParams) {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, void>({
    successMessage: "Monitor alerting disabled",
    mutationFn: () => deleteMonitorAlert(params, getToken),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["monitor-alert", params] }),
  });
}

/**
 * Save alert settings for a monitor named at call time, or disable alerting
 * when the settings are null. For forms
 * that save a monitor and its alert together.
 */
export function useApplyMonitorAlert(base: Omit<MonitorAlertPathParams, "monitorName">) {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, { monitorName: string; alert: MonitorAlertRequest | null }>({
    errorMessage: "The monitor was saved, but alert settings were not",
    mutationFn: async ({ monitorName, alert }) => {
      const params = { ...base, monitorName };
      if (alert) {
        await putMonitorAlert(params, alert, getToken);
      } else {
        await deleteMonitorAlert(params, getToken);
      }
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["monitor-alert"] }),
  });
}
