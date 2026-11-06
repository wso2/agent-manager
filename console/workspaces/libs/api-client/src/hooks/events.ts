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
  createEventDestination,
  createEventSubscription,
  deleteEventDestination,
  deleteEventSubscription,
  getEventDestination,
  listAvailableEventDestinations,
  listDestinationKinds,
  listEventDeliveries,
  listEventDestinations,
  listEventSubscriptions,
  listEventTypes,
  rotateEventDestinationSecret,
  testEventDestination,
  updateEventDestination,
  updateEventSubscription,
} from "../apis";
import type {
  DestinationKindListResponse,
  EventDeliveryListResponse,
  EventDeliveryQuery,
  EventDestinationListResponse,
  EventDestinationRequest,
  EventDestinationResponse,
  EventDestinationSecretResponse,
  EventDestinationSummaryListResponse,
  EventDestinationTestResponse,
  EventScope,
  EventSubscriptionListResponse,
  EventSubscriptionRequest,
  EventSubscriptionResponse,
  EventTargetParams,
  EventTypeListResponse,
} from "@agent-management-platform/types";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiMutation, useApiQuery } from "./react-query-notifications";
import { useRuntimeConfig } from "./runtime-config";

/** Whether a target names everything its scope needs. */
function targetReady(t: EventTargetParams): boolean {
  if (!t.orgName) return false;
  if (t.scope === "org") return true;
  if (!t.projName) return false;
  return t.scope === "project" || !!t.agentName;
}

/**
 * Whether the deployment delivers platform events (runs an event bus). The
 * service decides, through GET /api/v1/config; the console has no flag of
 * its own.
 */
export function useEventsEnabled(): boolean {
  const { data } = useRuntimeConfig();
  return data?.eventsEnabled === true;
}

/** Hook to list the event types a subscription at a scope can name. */
export function useEventTypes(orgName: string | undefined, scope: EventScope) {
  const { getToken } = useAuthHooks();
  return useApiQuery<EventTypeListResponse>({
    queryKey: ["event-types", orgName, scope],
    queryFn: () => listEventTypes(orgName, scope, getToken),
    enabled: !!orgName,
    staleTime: Infinity,
  });
}

/** Hook to list the destination kinds and the services each delivers to. */
export function useDestinationKinds(orgName: string | undefined) {
  const { getToken } = useAuthHooks();
  return useApiQuery<DestinationKindListResponse>({
    queryKey: ["event-destination-kinds", orgName],
    queryFn: () => listDestinationKinds(orgName, getToken),
    enabled: !!orgName,
    staleTime: Infinity,
  });
}

/** Hook to list the organization's destinations. Needs amp:events:read. */
export function useListEventDestinations(orgName: string | undefined, enabled = true) {
  const { getToken } = useAuthHooks();
  return useApiQuery<EventDestinationListResponse>({
    queryKey: ["event-destinations", orgName],
    queryFn: () => listEventDestinations(orgName, getToken),
    enabled: enabled && !!orgName,
  });
}

/** Hook to list the destinations a project's or an agent's subscriptions can use. */
export function useAvailableEventDestinations(target: EventTargetParams, enabled = true) {
  const { getToken } = useAuthHooks();
  return useApiQuery<EventDestinationSummaryListResponse>({
    queryKey: ["event-destinations", target.orgName, "available", target],
    queryFn: () => listAvailableEventDestinations(target, getToken),
    enabled: enabled && target.scope !== "org" && targetReady(target),
  });
}

/** Hook to get one destination with the subscriptions that use it. */
export function useEventDestination(orgName: string | undefined, id: string | undefined) {
  const { getToken } = useAuthHooks();
  return useApiQuery<EventDestinationResponse>({
    queryKey: ["event-destinations", orgName, "one", id],
    queryFn: () => getEventDestination(orgName, id ?? "", getToken),
    enabled: !!orgName && !!id,
  });
}

/** Invalidates everything a destination or subscription change can affect. */
function useInvalidateEvents() {
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: ["event-destinations"] }),
      queryClient.invalidateQueries({ queryKey: ["event-subscriptions"] }),
    ]);
}

/** Hook to add a destination. The response carries its generated secret once. */
export function useCreateEventDestination(orgName: string | undefined) {
  const { getToken } = useAuthHooks();
  const invalidate = useInvalidateEvents();
  return useApiMutation<EventDestinationResponse, unknown, EventDestinationRequest>({
    action: { verb: "create", target: "destination" },
    mutationFn: (body) => createEventDestination(orgName, body, getToken),
    onSuccess: () => invalidate(),
  });
}

/** Hook to update a destination. */
export function useUpdateEventDestination(orgName: string | undefined) {
  const { getToken } = useAuthHooks();
  const invalidate = useInvalidateEvents();
  return useApiMutation<
    EventDestinationResponse,
    unknown,
    { id: string; body: EventDestinationRequest }
  >({
    action: { verb: "update", target: "destination" },
    mutationFn: ({ id, body }) => updateEventDestination(orgName, id, body, getToken),
    onSuccess: () => invalidate(),
  });
}

/** Hook to delete a destination no subscription uses. */
export function useDeleteEventDestination(orgName: string | undefined) {
  const { getToken } = useAuthHooks();
  const invalidate = useInvalidateEvents();
  return useApiMutation<void, unknown, string>({
    action: { verb: "delete", target: "destination" },
    mutationFn: (id) => deleteEventDestination(orgName, id, getToken),
    onSuccess: () => invalidate(),
  });
}

/** Hook to rotate a destination's signing secret. */
export function useRotateEventDestinationSecret(orgName: string | undefined) {
  const { getToken } = useAuthHooks();
  return useApiMutation<EventDestinationSecretResponse, unknown, string>({
    action: { verb: "update", target: "signing secret" },
    mutationFn: (id) => rotateEventDestinationSecret(orgName, id, getToken),
  });
}

/**
 * Hook to send a test event. A request that reached the service succeeds even
 * when the destination refused the event, so callers show the outcome from
 * the result instead of a success snackbar.
 */
export function useTestEventDestination(orgName: string | undefined) {
  const { getToken } = useAuthHooks();
  return useApiMutation<EventDestinationTestResponse, unknown, string>({
    mutationFn: (id) => testEventDestination(orgName, id, getToken),
    showSuccess: false,
  });
}

/** Hook to list a level's subscriptions and those it inherits. */
export function useListEventSubscriptions(target: EventTargetParams, enabled = true) {
  const { getToken } = useAuthHooks();
  return useApiQuery<EventSubscriptionListResponse>({
    queryKey: ["event-subscriptions", target],
    queryFn: () => listEventSubscriptions(target, getToken),
    enabled: enabled && targetReady(target),
  });
}

/** Hook to create a subscription. */
export function useCreateEventSubscription(target: EventTargetParams) {
  const { getToken } = useAuthHooks();
  const invalidate = useInvalidateEvents();
  return useApiMutation<EventSubscriptionResponse, unknown, EventSubscriptionRequest>({
    action: { verb: "create", target: "subscription" },
    mutationFn: (body) => createEventSubscription(target, body, getToken),
    onSuccess: () => invalidate(),
  });
}

/** Hook to update a subscription. */
export function useUpdateEventSubscription(target: EventTargetParams) {
  const { getToken } = useAuthHooks();
  const invalidate = useInvalidateEvents();
  return useApiMutation<
    EventSubscriptionResponse,
    unknown,
    { id: string; body: EventSubscriptionRequest }
  >({
    action: { verb: "update", target: "subscription" },
    mutationFn: ({ id, body }) => updateEventSubscription(target, id, body, getToken),
    onSuccess: () => invalidate(),
  });
}

/** Hook to delete a subscription. */
export function useDeleteEventSubscription(target: EventTargetParams) {
  const { getToken } = useAuthHooks();
  const invalidate = useInvalidateEvents();
  return useApiMutation<void, unknown, string>({
    action: { verb: "delete", target: "subscription" },
    mutationFn: (id) => deleteEventSubscription(target, id, getToken),
    onSuccess: () => invalidate(),
  });
}

/** Hook to list a page of a level's delivery log, newest first. */
export function useListEventDeliveries(
  target: EventTargetParams,
  query: EventDeliveryQuery,
  enabled = true,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<EventDeliveryListResponse>({
    queryKey: ["event-deliveries", target, query],
    queryFn: () => listEventDeliveries(target, query, getToken),
    enabled: enabled && targetReady(target),
  });
}
