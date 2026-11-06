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

import { httpDELETE, httpGET, httpPOST, httpPUT, SERVICE_BASE } from "../utils";
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

async function tokenOf(getToken?: () => Promise<string>) {
  return getToken ? await getToken() : undefined;
}

/** The organization's events path. */
function orgEventsBase(orgName: string | undefined): string {
  return `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName ?? "default")}/events`;
}

/** The events path of a target: the organization, a project or an agent. */
export function eventsBase(target: EventTargetParams): string {
  if (target.scope === "org") return orgEventsBase(target.orgName);
  if (!target.projName) throw new Error("projName is required");
  const project =
    `${SERVICE_BASE}/orgs/${encodeURIComponent(target.orgName ?? "default")}` +
    `/projects/${encodeURIComponent(target.projName)}`;
  if (target.scope === "project") return `${project}/events`;
  if (!target.agentName) throw new Error("agentName is required");
  return `${project}/agents/${encodeURIComponent(target.agentName)}/events`;
}

export async function listEventTypes(
  orgName: string | undefined,
  scope: EventScope,
  getToken?: () => Promise<string>,
): Promise<EventTypeListResponse> {
  const token = await tokenOf(getToken);
  const res = await httpGET(`${orgEventsBase(orgName)}/event-types`, {
    searchParams: { scope },
    token,
  });
  return res.json();
}

export async function listDestinationKinds(
  orgName: string | undefined,
  getToken?: () => Promise<string>,
): Promise<DestinationKindListResponse> {
  const token = await tokenOf(getToken);
  const res = await httpGET(`${orgEventsBase(orgName)}/destination-kinds`, { token });
  return res.json();
}

export async function listEventDestinations(
  orgName: string | undefined,
  getToken?: () => Promise<string>,
): Promise<EventDestinationListResponse> {
  const token = await tokenOf(getToken);
  const res = await httpGET(`${orgEventsBase(orgName)}/destinations`, { token });
  return res.json();
}

/** Destinations a project's or an agent's subscriptions can use, without their settings. */
export async function listAvailableEventDestinations(
  target: EventTargetParams,
  getToken?: () => Promise<string>,
): Promise<EventDestinationSummaryListResponse> {
  const token = await tokenOf(getToken);
  const res = await httpGET(`${eventsBase(target)}/destinations`, { token });
  return res.json();
}

export async function getEventDestination(
  orgName: string | undefined,
  id: string,
  getToken?: () => Promise<string>,
): Promise<EventDestinationResponse> {
  const token = await tokenOf(getToken);
  const res = await httpGET(`${orgEventsBase(orgName)}/destinations/${encodeURIComponent(id)}`, {
    token,
  });
  return res.json();
}

export async function createEventDestination(
  orgName: string | undefined,
  body: EventDestinationRequest,
  getToken?: () => Promise<string>,
): Promise<EventDestinationResponse> {
  const token = await tokenOf(getToken);
  const res = await httpPOST(`${orgEventsBase(orgName)}/destinations`, body, { token });
  return res.json();
}

export async function updateEventDestination(
  orgName: string | undefined,
  id: string,
  body: EventDestinationRequest,
  getToken?: () => Promise<string>,
): Promise<EventDestinationResponse> {
  const token = await tokenOf(getToken);
  const res = await httpPUT(
    `${orgEventsBase(orgName)}/destinations/${encodeURIComponent(id)}`,
    body,
    { token },
  );
  return res.json();
}

export async function deleteEventDestination(
  orgName: string | undefined,
  id: string,
  getToken?: () => Promise<string>,
): Promise<void> {
  const token = await tokenOf(getToken);
  await httpDELETE(`${orgEventsBase(orgName)}/destinations/${encodeURIComponent(id)}`, { token });
}

export async function rotateEventDestinationSecret(
  orgName: string | undefined,
  id: string,
  getToken?: () => Promise<string>,
): Promise<EventDestinationSecretResponse> {
  const token = await tokenOf(getToken);
  const res = await httpPOST(
    `${orgEventsBase(orgName)}/destinations/${encodeURIComponent(id)}/rotate-secret`,
    {},
    { token },
  );
  return res.json();
}

export async function testEventDestination(
  orgName: string | undefined,
  id: string,
  getToken?: () => Promise<string>,
): Promise<EventDestinationTestResponse> {
  const token = await tokenOf(getToken);
  const res = await httpPOST(
    `${orgEventsBase(orgName)}/destinations/${encodeURIComponent(id)}/test`,
    {},
    { token },
  );
  return res.json();
}

export async function listEventSubscriptions(
  target: EventTargetParams,
  getToken?: () => Promise<string>,
): Promise<EventSubscriptionListResponse> {
  const token = await tokenOf(getToken);
  const res = await httpGET(`${eventsBase(target)}/subscriptions`, { token });
  return res.json();
}

export async function createEventSubscription(
  target: EventTargetParams,
  body: EventSubscriptionRequest,
  getToken?: () => Promise<string>,
): Promise<EventSubscriptionResponse> {
  const token = await tokenOf(getToken);
  const res = await httpPOST(`${eventsBase(target)}/subscriptions`, body, { token });
  return res.json();
}

export async function updateEventSubscription(
  target: EventTargetParams,
  id: string,
  body: EventSubscriptionRequest,
  getToken?: () => Promise<string>,
): Promise<EventSubscriptionResponse> {
  const token = await tokenOf(getToken);
  const res = await httpPUT(`${eventsBase(target)}/subscriptions/${encodeURIComponent(id)}`, body, {
    token,
  });
  return res.json();
}

export async function deleteEventSubscription(
  target: EventTargetParams,
  id: string,
  getToken?: () => Promise<string>,
): Promise<void> {
  const token = await tokenOf(getToken);
  await httpDELETE(`${eventsBase(target)}/subscriptions/${encodeURIComponent(id)}`, { token });
}

export async function listEventDeliveries(
  target: EventTargetParams,
  query: EventDeliveryQuery,
  getToken?: () => Promise<string>,
): Promise<EventDeliveryListResponse> {
  const token = await tokenOf(getToken);
  const searchParams: Record<string, string> = {};
  if (query.destinationId) searchParams.destinationId = query.destinationId;
  if (query.subscriptionId) searchParams.subscriptionId = query.subscriptionId;
  if (query.status) searchParams.status = query.status;
  if (query.limit) searchParams.limit = String(query.limit);
  if (query.offset) searchParams.offset = String(query.offset);
  const res = await httpGET(`${eventsBase(target)}/deliveries`, { searchParams, token });
  return res.json();
}
