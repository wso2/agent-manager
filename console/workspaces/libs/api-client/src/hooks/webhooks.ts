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
  createWebhook,
  deleteWebhook,
  listWebhookDeliveries,
  listWebhookEventTypes,
  listWebhooks,
  rotateWebhookSecret,
  testWebhook,
  updateWebhook,
} from "../apis";
import type {
  WebhookDeliveryListResponse,
  WebhookEventTypeListResponse,
  WebhookListResponse,
  WebhookRequest,
  WebhookResponse,
  WebhookScope,
  WebhookSecretResponse,
  WebhookTargetParams,
  WebhookTestResponse,
} from "@agent-management-platform/types";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiMutation, useApiQuery } from "./react-query-notifications";

/** Whether a target names everything its scope needs. */
function targetReady(t: WebhookTargetParams): boolean {
  if (!t.orgName) return false;
  if (t.scope === "org") return true;
  if (!t.projName) return false;
  return t.scope === "project" || !!t.agentName;
}

/** Hook to list the event types a scope's webhooks can subscribe to. */
export function useWebhookEventTypes(orgName: string | undefined, scope: WebhookScope) {
  const { getToken } = useAuthHooks();
  return useApiQuery<WebhookEventTypeListResponse>({
    queryKey: ["webhook-event-types", orgName, scope],
    queryFn: () => listWebhookEventTypes(orgName, scope, getToken),
    enabled: !!orgName,
    staleTime: Infinity,
  });
}

/** Hook to list a scope's webhooks. Pass enabled=false to skip the request. */
export function useListWebhooks(target: WebhookTargetParams, enabled = true) {
  const { getToken } = useAuthHooks();
  return useApiQuery<WebhookListResponse>({
    queryKey: ["webhooks", target],
    queryFn: () => listWebhooks(target, getToken),
    enabled: enabled && targetReady(target),
  });
}

/** Hook to create a webhook. The response carries the signing secret once. */
export function useCreateWebhook(target: WebhookTargetParams) {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<WebhookResponse, unknown, WebhookRequest>({
    action: { verb: "create", target: "webhook" },
    mutationFn: (body) => createWebhook(target, body, getToken),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["webhooks", target] }),
  });
}

/** Hook to update a webhook. */
export function useUpdateWebhook(target: WebhookTargetParams) {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<WebhookResponse, unknown, { id: string; body: WebhookRequest }>({
    action: { verb: "update", target: "webhook" },
    mutationFn: ({ id, body }) => updateWebhook(target, id, body, getToken),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["webhooks", target] }),
  });
}

/** Hook to delete a webhook. */
export function useDeleteWebhook(target: WebhookTargetParams) {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, string>({
    action: { verb: "delete", target: "webhook" },
    mutationFn: (id) => deleteWebhook(target, id, getToken),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["webhooks", target] }),
  });
}

/** Hook to rotate a webhook's signing secret. */
export function useRotateWebhookSecret(target: WebhookTargetParams) {
  const { getToken } = useAuthHooks();
  return useApiMutation<WebhookSecretResponse, unknown, string>({
    action: { verb: "update", target: "webhook signing secret" },
    mutationFn: (id) => rotateWebhookSecret(target, id, getToken),
  });
}

/**
 * Hook to send a test event. A request that reached the service succeeds even
 * when the endpoint refused the event, so callers show the outcome from the
 * result instead of a success snackbar.
 */
export function useTestWebhook(target: WebhookTargetParams) {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<WebhookTestResponse, unknown, string>({
    mutationFn: (id) => testWebhook(target, id, getToken),
    showSuccess: false,
    onSuccess: (_d, id) => queryClient.invalidateQueries({ queryKey: ["webhook-deliveries", target, id] }),
  });
}

/** Hook to list a webhook's recent deliveries. */
export function useWebhookDeliveries(target: WebhookTargetParams, id: string | undefined) {
  const { getToken } = useAuthHooks();
  return useApiQuery<WebhookDeliveryListResponse>({
    queryKey: ["webhook-deliveries", target, id],
    queryFn: () => listWebhookDeliveries(target, id ?? "", 50, getToken),
    enabled: !!id && targetReady(target),
  });
}
