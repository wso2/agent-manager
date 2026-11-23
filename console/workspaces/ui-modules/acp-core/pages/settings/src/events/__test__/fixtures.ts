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

import type {
  DestinationKindListResponse,
  EventDeliveryResponse,
  EventDestinationResponse,
  EventSubscriptionResponse,
} from "@agent-management-platform/types";

export const destination = (
  over: Partial<EventDestinationResponse> = {},
): EventDestinationResponse => ({
  id: "d1",
  name: "Platform alerts",
  description: "",
  kind: "notify",
  service: "slack",
  config: { service: "slack" },
  availability: "all",
  projects: [],
  usedBy: 0,
  health: "healthy",
  consecutiveFailures: 0,
  createdBy: "u1",
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
  ...over,
});

export const subscription = (
  over: Partial<EventSubscriptionResponse> = {},
): EventSubscriptionResponse => ({
  id: "s1",
  scope: "org",
  projectName: "",
  agentName: "",
  name: "Deploys to Slack",
  description: "Every deploy",
  destination: { id: "d1", name: "Platform alerts", kind: "notify", service: "slack", health: "healthy" },
  eventTypes: ["agent.deployed"],
  environments: [],
  enabled: true,
  createdBy: "u1",
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
  last24h: { delivered: 0, failed: 0, pending: 0, skipped: 0 },
  ...over,
});

export const delivery = (over: Partial<EventDeliveryResponse> = {}): EventDeliveryResponse => ({
  id: "del1",
  eventId: "evt-123",
  eventType: "agent.deployed",
  eventScope: "agent",
  projectName: "shop",
  agentName: "bot",
  environment: "dev",
  destination: { id: "d1", name: "Platform alerts", kind: "notify", service: "slack", health: "healthy" },
  subscriptions: [{ id: "s1", name: "Deploys to Slack", scope: "org" }],
  status: "delivered",
  attempts: 1,
  responseCode: 200,
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
  ...over,
});

export const kinds: DestinationKindListResponse = {
  kinds: [
    {
      kind: "notify",
      generatedSecret: false,
      suppliedCredential: true,
      services: [
        { id: "slack", label: "Slack", category: "Chat", urlFormat: "slack://token@channel", docsPath: "/slack" },
        { id: "teams", label: "Microsoft Teams", category: "Chat", urlFormat: "teams://x", docsPath: "" },
        { id: "pagerduty", label: "PagerDuty", category: "Paging", urlFormat: "pagerduty://key", docsPath: "" },
        { id: "googlechat", label: "Google Chat", category: "Chat", urlFormat: "googlechat://x", docsPath: "" },
        { id: "discord", label: "Discord", category: "Chat", urlFormat: "discord://id/token", docsPath: "" },
        { id: "opsgenie", label: "Opsgenie", category: "Paging", urlFormat: "opsgenie://key", docsPath: "" },
      ],
    },
    { kind: "webhook", generatedSecret: true, suppliedCredential: false, services: [] },
  ],
};
