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
  deleteAlertEndpoint,
  getAlertEndpoint,
  getMonitorAlertConfig,
  listAlertDeliveries,
  testAlertEndpoint,
  updateMonitorAlertConfig,
  upsertAlertEndpoint,
} from "../apis";
import type {
  AlertDeliveryListResponse,
  AlertEndpointPathParams,
  AlertEndpointRequest,
  AlertEndpointResponse,
  AlertTestResponse,
  ListAlertDeliveriesQuery,
  MonitorAlertConfigPathParams,
  MonitorAlertConfigRequest,
  MonitorAlertConfigResponse,
} from "@agent-management-platform/types";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiMutation, useApiQuery } from "./react-query-notifications";

/**
 * Hook to get the org alert endpoint (null when not configured).
 */
export function useGetAlertEndpoint(params: AlertEndpointPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AlertEndpointResponse | null>({
    queryKey: ["alert-endpoint", params],
    queryFn: () => getAlertEndpoint(params, getToken),
    enabled: !!params.orgName,
  });
}

/**
 * Hook to create or update the org alert endpoint.
 */
export function useUpsertAlertEndpoint() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    AlertEndpointResponse,
    unknown,
    { params: AlertEndpointPathParams; body: AlertEndpointRequest }
  >({
    action: { verb: "update", target: "alert endpoint" },
    mutationFn: ({ params, body }) => upsertAlertEndpoint(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["alert-endpoint"] });
      queryClient.invalidateQueries({ queryKey: ["monitor-alert-config"] });
    },
  });
}

/**
 * Hook to delete the org alert endpoint.
 */
export function useDeleteAlertEndpoint() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, AlertEndpointPathParams>({
    action: { verb: "delete", target: "alert endpoint" },
    mutationFn: (params) => deleteAlertEndpoint(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["alert-endpoint"] });
      queryClient.invalidateQueries({ queryKey: ["monitor-alert-config"] });
    },
  });
}

/**
 * Hook to send a test alert. A request that reached the service succeeds even
 * when the endpoint rejected the alert, so the caller shows the outcome from
 * the result instead of a success snackbar.
 */
export function useTestAlertEndpoint() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<AlertTestResponse, unknown, AlertEndpointPathParams>({
    mutationFn: (params) => testAlertEndpoint(params, getToken),
    showSuccess: false,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["alert-endpoint"] });
    },
  });
}

/**
 * Hook to list recent alert deliveries.
 */
export function useListAlertDeliveries(
  params: AlertEndpointPathParams,
  query?: ListAlertDeliveriesQuery,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AlertDeliveryListResponse>({
    queryKey: ["alert-deliveries", params, query],
    queryFn: () => listAlertDeliveries(params, query, getToken),
    enabled: !!params.orgName,
  });
}

/**
 * Hook to get a monitor's alert configuration.
 */
export function useGetMonitorAlertConfig(params: MonitorAlertConfigPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<MonitorAlertConfigResponse>({
    queryKey: ["monitor-alert-config", params],
    queryFn: () => getMonitorAlertConfig(params, getToken),
    enabled: !!params.orgName && !!params.projName && !!params.agentName && !!params.monitorName,
  });
}

/**
 * Hook to replace a monitor's alert configuration.
 */
export function useUpdateMonitorAlertConfig() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    MonitorAlertConfigResponse,
    unknown,
    { params: MonitorAlertConfigPathParams; body: MonitorAlertConfigRequest }
  >({
    action: { verb: "update", target: "monitor alerting" },
    mutationFn: ({ params, body }) => updateMonitorAlertConfig(params, body, getToken),
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({ queryKey: ["monitor-alert-config", params] });
    },
  });
}
