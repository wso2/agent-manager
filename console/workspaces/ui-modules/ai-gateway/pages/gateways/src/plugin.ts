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

// Plugin manifest. The shell imports this module eagerly, so it must not import
// the page components: they are reached only through the dynamic import() in
// `load`, which keeps them in their own chunk.
import { Network } from "@wso2/oxygen-ui-icons-react";
import {
  absoluteRouteMap,
  PLUGIN_API_VERSION,
  type PluginManifest,
} from "@agent-management-platform/types";
import { useGatewayFeatures } from "@agent-management-platform/feature-management";

export const gatewaysPlugin: PluginManifest = {
  id: "gateways",
  name: "Gateways",
  apiVersion: PLUGIN_API_VERSION,
  // Hidden when gateways are managed outside the console or the user lacks
  // gateway read access.
  useActivation: () => useGatewayFeatures().canView,
  contributions: [
    {
      type: "route",
      id: "page",
      level: "org",
      path: absoluteRouteMap.children.org.children.gateways.path + "/*",
      load: () =>
        import("./Gateways.Organization").then((m) => ({
          default: m.GatewaysOrganization,
        })),
    },
    {
      type: "nav-item",
      id: "nav",
      level: "org",
      section: "infrastructure",
      order: 10,
      label: "Gateways",
      icon: Network,
      route: absoluteRouteMap.children.org.children.gateways,
    },
  ],
};
