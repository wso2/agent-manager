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

/** Where a subscription sits, and the level an event happens at. */
export type EventScope = 'org' | 'project' | 'agent';

/** The event-type entry that subscribes to every type a level can receive. */
export const ALL_EVENTS = '*';

export interface EventType {
  /** The catalog name, e.g. "agent.deployed". */
  type: string;
  scope: EventScope;
  category: string;
  description: string;
}

export interface EventTypeListResponse {
  eventTypes: EventType[];
}

/**
 * How a destination delivers events: "webhook" posts the signed CloudEvent;
 * "notify" sends a text notification to a chat, email, paging or push service.
 */
export type DestinationKind = 'webhook' | 'notify';

/** Settings of a destination, by kind. */
export interface DestinationConfig {
  /** webhook: the HTTPS endpoint. */
  url?: string;
  /** notify: a service id from the destination kinds, e.g. "slack". */
  service?: string;
}

/** A service a destination kind delivers to, such as Slack for "notify". */
export interface DestinationKindService {
  id: string;
  label: string;
  category: string;
  /** The shape of the service URL. */
  urlFormat: string;
  /** How to get the values the URL needs. */
  docsUrl: string;
}

export interface DestinationKindInfo {
  kind: DestinationKind;
  /** Destinations get a generated secret that can be rotated. */
  generatedSecret: boolean;
  /** Destinations take a credential the user enters, which is never returned. */
  suppliedCredential: boolean;
  services: DestinationKindService[];
}

export interface DestinationKindListResponse {
  kinds: DestinationKindInfo[];
}

/**
 * Which subscriptions below the organization may use a destination: those of
 * every project, none, or those of the listed projects. Organization
 * subscriptions may use any destination.
 */
export type DestinationAvailability = 'all' | 'org' | 'projects';

/**
 * healthy: the latest delivery succeeded; failing: deliveries failed since;
 * idle: nothing delivered yet.
 */
export type DestinationHealth = 'healthy' | 'failing' | 'idle';

export interface EventDestinationRequest {
  name: string;
  description?: string;
  kind: DestinationKind;
  /** notify: include url to set or replace the service URL; leave it out to keep it. */
  config: DestinationConfig;
  availability: DestinationAvailability;
  projects?: string[];
}

/** A subscription named by a destination's usage or a delivery. */
export interface EventSubscriptionRef {
  id: string;
  name: string;
  scope: EventScope;
  projectName?: string;
  agentName?: string;
}

export interface EventDestinationResponse {
  id: string;
  name: string;
  description: string;
  kind: DestinationKind;
  /** The notification service, for notify destinations. */
  service?: string;
  /** Masked for callers who cannot manage destinations. */
  config: DestinationConfig;
  availability: DestinationAvailability;
  projects: string[];
  /** The number of subscriptions that send to it. */
  usedBy: number;
  lastDeliveryAt?: string;
  health: DestinationHealth;
  /** Deliveries that failed or are being retried since the latest success. */
  consecutiveFailures: number;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
  /** The generated secret, only in the response that creates the destination. */
  secret?: string;
  /** The subscriptions that send to it, only when getting one destination. */
  subscriptions?: EventSubscriptionRef[];
}

export interface EventDestinationListResponse {
  destinations: EventDestinationResponse[];
}

/** A destination as subscriptions below the organization see it, without its settings. */
export interface EventDestinationSummary {
  id: string;
  name: string;
  kind: DestinationKind;
  service?: string;
  health: DestinationHealth;
}

export interface EventDestinationSummaryListResponse {
  destinations: EventDestinationSummary[];
}

export interface EventDestinationSecretResponse {
  secret: string;
}

export interface EventDestinationTestResponse {
  delivered: boolean;
  statusCode?: number;
  error?: string;
}

export interface EventSubscriptionRequest {
  name: string;
  description?: string;
  destinationId: string;
  /** Event types the level can receive, or just ALL_EVENTS. */
  eventTypes: string[];
  /** Only events in these environments; empty for every environment. */
  environments?: string[];
  enabled?: boolean;
}

export interface EventDeliveryCounts {
  delivered: number;
  failed: number;
  pending: number;
  skipped: number;
}

export interface EventSubscriptionResponse {
  id: string;
  scope: EventScope;
  projectName: string;
  agentName: string;
  name: string;
  description: string;
  destination: EventDestinationSummary;
  eventTypes: string[];
  environments: string[];
  enabled: boolean;
  /** The ID of the user who created it. */
  createdBy: string;
  createdAt: string;
  updatedAt: string;
  /**
   * Deliveries of the last 24 hours; for an inherited subscription, of this
   * level's events only.
   */
  last24h: EventDeliveryCounts;
}

export interface EventSubscriptionListResponse {
  /** The subscriptions this level owns. */
  subscriptions: EventSubscriptionResponse[];
  /** Wider levels' subscriptions that also send this level's events, read-only here. */
  inherited: EventSubscriptionResponse[];
}

export type EventDeliveryStatus = 'pending' | 'delivered' | 'failed' | 'skipped';

export interface EventDeliveryResponse {
  id: string;
  eventId: string;
  eventType: string;
  eventScope: EventScope;
  projectName?: string;
  agentName?: string;
  environment?: string;
  destination?: EventDestinationSummary;
  /** The subscriptions that sent the event to the destination. */
  subscriptions: EventSubscriptionRef[];
  status: EventDeliveryStatus;
  attempts: number;
  responseCode?: number;
  lastError?: string;
  deliveredAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface EventDeliveryListResponse {
  deliveries: EventDeliveryResponse[];
  /** Deliveries matching the query, ignoring limit and offset. */
  total: number;
}

export interface EventDeliveryQuery {
  destinationId?: string;
  subscriptionId?: string;
  status?: EventDeliveryStatus;
  limit?: number;
  offset?: number;
}

/**
 * Identifies the owner of a set of subscriptions: the organization, a project
 * in it, or an agent in a project.
 */
export interface EventTargetParams {
  scope: EventScope;
  orgName?: string;
  projName?: string;
  agentName?: string;
}
