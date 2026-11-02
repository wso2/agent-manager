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
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import type { GatewayResponse } from "@agent-management-platform/types";

const isManaged = (gateway: GatewayResponse): boolean => gateway.hosting !== "SELF_HOSTED";

const isIngressCapable = (gateway: GatewayResponse): boolean =>
  gateway.gatewayType === "INGRESS" || gateway.gatewayType === "BOTH";

/**
 * The vhost an agent's traces go to: the environment's OTEL RestApi is published on a
 * managed gateway, an ingress-capable one when there is one. A self-hosted gateway is the
 * customer's, never where an agent's telemetry goes.
 */
export function otelGatewayVhost(gateways: GatewayResponse[] | undefined): string | undefined {
  const managed = (gateways ?? []).filter(isManaged);
  return (managed.find(isIngressCapable) ?? managed[0])?.vhost;
}
