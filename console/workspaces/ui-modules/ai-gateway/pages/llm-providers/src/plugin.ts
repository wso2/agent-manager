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
import { BrainCircuit, Brain } from "@wso2/oxygen-ui-icons-react";
import { useFeatureManagement, useResourceAccess } from "@agent-management-platform/feature-management";
import {
  absoluteRouteMap,
  PLUGIN_API_VERSION,
  type PluginManifest,
  type ResourceSummaryState,
} from "@agent-management-platform/types";
import { generatePath, useParams } from "react-router-dom";
import { useListLLMProviders } from "@agent-management-platform/api-client";

// Route-map shorthands (absolute paths, used for both routes and links).
const orgNav = absoluteRouteMap.children.org.children;
const agentNav = orgNav.projects.children.agents.children;

/** The Resources section shows when the user can read any of its resources. */
const useResourcesNav = () => {
  const { hasAccess } = useFeatureManagement();
  return (
    hasAccess("llm_provider_read") ||
    hasAccess("llm_provider_template_read") ||
    hasAccess("mcp_server_read") ||
    hasAccess("llm_proxy_read")
  );
};

/** The organization overview's LLM Service Providers card. */
const useLLMProvidersSummary = (): ResourceSummaryState => {
  const { orgId = "" } = useParams();
  const { canWrite } = useResourceAccess("llm_provider");
  const { data, isLoading } = useListLLMProviders({ orgName: orgId });
  const llmProviders = absoluteRouteMap.children.org.children.llmProviders;
  return {
    count: data?.total,
    isLoading,
    actionLabel: canWrite ? "Add an LLM provider" : "View LLM providers",
    to: generatePath(canWrite ? llmProviders.children.add.path : llmProviders.path, { orgId }),
  };
};

export const llmProvidersPlugin: PluginManifest = {
  id: "llm-providers",
  name: "LLM service providers",
  apiVersion: PLUGIN_API_VERSION,
  contributions: [
    {
      // Organization overview: the LLM Service Providers card.
      type: "resource-summary",
      id: "llm-providers-summary",
      order: 20,
      title: "LLM Service Providers",
      description: "Model providers your agents call, with guardrails and rate limits.",
      icon: Brain,
      alwaysShow: true,
      useSummary: useLLMProvidersSummary,
    },
    {
      type: "route",
      id: "org-page",
      level: "org",
      path: orgNav.llmProviders.path + "/*",
      load: () =>
        import("./index").then((m) => ({
          default: m.metaData.levels!.organization as ComponentType,
        })),
    },
    {
      type: "route",
      id: "add-org-page",
      level: "org",
      path: orgNav.llmProviders.children.add.path,
      load: () =>
        import("./index").then((m) => ({
          default: m.metaData.levels!.addLLMProvidersOrganization as ComponentType,
        })),
    },
    {
      type: "route",
      id: "agent-page",
      level: "agent",
      path: agentNav.configure.children.llmProviders.path,
      load: () =>
        import("./index").then((m) => ({
          default: m.metaData.levels!.component as ComponentType,
        })),
    },
    {
      type: "nav-item",
      id: "nav",
      level: "org",
      section: "resources",
      order: 10,
      label: "LLM service providers",
      icon: BrainCircuit,
      route: orgNav.llmProviders,
      useActivation: useResourcesNav,
    },
  ],
};
