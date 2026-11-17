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
// each `load`, which keeps them in the package's own chunk.
import type { ComponentType } from "react";
import { BookOpenText, Megaphone } from "@wso2/oxygen-ui-icons-react";
import { useRouteAgent, useListAgentKinds } from "@agent-management-platform/api-client";
import { useIsUIModuleInstalled } from "@agent-management-platform/views";
import {
  absoluteRouteMap,
  PLUGIN_API_VERSION,
  PLUGIN_MOUNT_POINTS,
  type PluginActivationState,
  type PluginManifest,
  type ResourceSummaryState,
} from "@agent-management-platform/types";
import { CatalogIllustration } from "./createAgent/CatalogIllustration";
import { generatePath, useParams } from "react-router-dom";

// Route-map shorthands (absolute paths, used for both routes and links).
const orgNav = absoluteRouteMap.children.org.children;
const agentNav = orgNav.projects.children.agents.children;

/** Internal agents built from source, i.e. not created from a catalog kind. */
const useSourceBuiltAgent = (): PluginActivationState => {
  const { data: agent, isLoading } = useRouteAgent();
  if (isLoading) return undefined;
  return !!agent && agent.provisioning?.type !== "external" && !agent.kindName;
};

/**
 * An agent created from a catalog kind is deployed by the agent runtime, so
 * creating one is offered only while that module is present.
 */
const useAgentRuntimePresent = (): PluginActivationState =>
  useIsUIModuleInstalled("agent-runtime");

const fromCatalog = orgNav.projects.children.newAgent.children.create.children.catalog;

/** An agent created from a catalog kind. */
const useKindAgent = (): PluginActivationState => {
  const { data: agent, isLoading } = useRouteAgent();
  if (isLoading) return undefined;
  return !!agent?.kindName;
};

/** The organization overview's Agent Catalog card. */
const useCatalogSummary = (): ResourceSummaryState => {
  const { orgId = "" } = useParams();
  const { data, isLoading } = useListAgentKinds({ orgName: orgId });
  return {
    count: data?.total,
    isLoading,
    actionLabel: "Browse the catalog",
    to: generatePath(absoluteRouteMap.children.org.children.catalog.path, { orgId }),
  };
};

export const agentKindPlugin: PluginManifest = {
  id: "agent-kind",
  name: "Agent catalog and publishing",
  apiVersion: PLUGIN_API_VERSION,
  contributions: [
    {
      // Organization overview: the Agent Catalog card.
      type: "resource-summary",
      id: "catalog-summary",
      order: 10,
      title: "Agent Catalog",
      description: "Reusable, versioned agent templates your teams can create agents from.",
      icon: BookOpenText,
      useSummary: useCatalogSummary,
    },
    {
      // Agent overview: the catalog kind the agent was created from.
      type: "panel",
      id: "overview-built-from",
      mountPoint: PLUGIN_MOUNT_POINTS.agentOverviewInfo,
      order: 20,
      useActivation: useKindAgent,
      load: () => import("./index").then((m) => ({ default: m.KindInfoPanel as ComponentType })),
    },
    {
      type: "agent-type",
      id: "from-catalog",
      // The wizard used to ask "build new" and then "from catalog".
      wizardSteps: ["build-new", "from-catalog"],
      order: 30,
      title: "Agent from the Catalog",
      description:
        "Create a platform-hosted agent from an Agent Kind published in the Agent Catalog.",
      ctaLabel: "Browse catalog",
      illustration: CatalogIllustration,
      route: fromCatalog,
      useActivation: useAgentRuntimePresent,
    },
    {
      type: "route",
      id: "create-from-catalog-page",
      level: "project",
      path: fromCatalog.path,
      useActivation: useAgentRuntimePresent,
      load: () => import("./index").then((m) => ({ default: m.CatalogKindSelectPage })),
    },
    {
      type: "route",
      id: "create-from-kind-page",
      level: "project",
      path: fromCatalog.children.withKind.path,
      useActivation: useAgentRuntimePresent,
      load: () => import("./index").then((m) => ({ default: m.CatalogAgentPage })),
    },
    {
      type: "route",
      id: "catalog-page",
      level: "org",
      path: orgNav.catalog.path + "/*",
      load: () =>
        import("./index").then((m) => ({
          default: m.metaData.levels!.organization as ComponentType,
        })),
    },
    {
      type: "route",
      id: "publish-page",
      level: "agent",
      path: agentNav.publish.path + "/*",
      load: () =>
        import("./index").then((m) => ({
          default: m.metaData.levels!.publishOrganization as ComponentType,
        })),
    },
    {
      type: "nav-item",
      id: "catalog-nav",
      level: "org",
      section: "main",
      order: 30,
      label: "Agent catalog",
      icon: BookOpenText,
      route: orgNav.catalog,
    },
    {
      type: "nav-item",
      id: "publish-nav",
      level: "agent",
      section: "lifecycle",
      order: 50,
      label: "Publish",
      icon: Megaphone,
      route: agentNav.publish,
      useActivation: useSourceBuiltAgent,
    },
  ],
};
