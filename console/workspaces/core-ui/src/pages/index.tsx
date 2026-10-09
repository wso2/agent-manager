/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import { lazy, type ComponentType } from "react";

export * from "./Login";

// Overview
export const LazyOverviewOrg = lazy(() =>
  import("@agent-management-platform/overview").then((m) => ({
    default: m.metaData.levels!.organization as ComponentType,
  }))
);
export const LazyOverviewProject = lazy(() =>
  import("@agent-management-platform/overview").then((m) => ({
    default: m.metaData.levels!.project as ComponentType,
  }))
);
export const LazyOverviewComponent = lazy(() =>
  import("@agent-management-platform/overview").then((m) => ({
    default: m.metaData.levels!.component as ComponentType,
  }))
);

// Build
export const LazyBuildComponent = lazy(() =>
  import("@agent-management-platform/build").then((m) => ({
    default: m.metaData.levels!.component as ComponentType,
  }))
);

// Security
export const LazySecurityComponent = lazy(() =>
  import("@agent-management-platform/agent-security").then((m) => ({
    default: m.metaData.levels!.component as ComponentType,
  }))
);

// Configure Agent
export const LazyConfigureComponent = lazy(() =>
  import("@agent-management-platform/configure-agent").then((m) => ({
    default: m.metaData.component as ComponentType,
  }))
);

// Agent ID
export const LazyAgentIdComponent = lazy(() =>
  import("@agent-management-platform/env-thunders").then((m) => ({
    default: m.agentIdMetaData.component as ComponentType,
  }))
);
export const LazyAddLLMProvidersComponent = lazy(() =>
  import("@agent-management-platform/configure-agent").then((m) => ({
    default: m.AddLLMProviderComponent as ComponentType,
  }))
);
export const LazyViewLLMProviderComponent = lazy(() =>
  import("@agent-management-platform/configure-agent").then((m) => ({
    default: m.ViewLLMProviderComponent as ComponentType,
  }))
);
export const LazyViewMCPServerComponent = lazy(() =>
  import("@agent-management-platform/configure-agent").then((m) => ({
    default: m.ViewMCPServerComponent as ComponentType,
  }))
);

// Deploy
export const LazyDeploymentComponent = lazy(() =>
  import("@agent-management-platform/deploy").then((m) => ({
    default: m.metaData.levels!.component as ComponentType,
  }))
);

// Test
export const LazyTestComponent = lazy(() =>
  import("@agent-management-platform/test").then((m) => ({
    default: m.metaData.levels!.component as ComponentType,
  }))
);

// Observability
export const LazyTracesComponent = lazy(() =>
  import("@agent-management-platform/traces").then((m) => ({
    default: m.metaData.levels!.component as ComponentType,
  }))
);
export const LazyLogsComponent = lazy(() =>
  import("@agent-management-platform/logs").then((m) => ({
    default: m.metaData.levels!.component as ComponentType,
  }))
);
export const LazyMetricsComponent = lazy(() =>
  import("@agent-management-platform/metrics").then((m) => ({
    default: m.metaData.levels!.component as ComponentType,
  }))
);

// Evaluation
export const LazyEvalEvaluatorsOrg = lazy(() =>
  import("@agent-management-platform/eval").then((m) => ({
    default: m.metaData.pages.organization.evalEvaluators.component as ComponentType,
  }))
);
export const LazyCreateEvaluatorOrg = lazy(() =>
  import("@agent-management-platform/eval").then((m) => ({
    default: m.metaData.pages.organization.createEvaluator.component as ComponentType,
  }))
);
export const LazyViewEvaluatorOrg = lazy(() =>
  import("@agent-management-platform/eval").then((m) => ({
    default: m.metaData.pages.organization.viewEvaluator.component as ComponentType,
  }))
);
export const LazyEditEvaluatorOrg = lazy(() =>
  import("@agent-management-platform/eval").then((m) => ({
    default: m.metaData.pages.organization.editEvaluator.component as ComponentType,
  }))
);
export const LazyEvalMonitorsComponent = lazy(() =>
  import("@agent-management-platform/eval").then((m) => ({
    default: m.metaData.pages.organization.evalMonitors.component as ComponentType,
  }))
);
export const LazyCreateMonitorComponent = lazy(() =>
  import("@agent-management-platform/eval").then((m) => ({
    default: m.metaData.pages.organization.createMonitor.component as ComponentType,
  }))
);
export const LazyEditMonitorComponent = lazy(() =>
  import("@agent-management-platform/eval").then((m) => ({
    default: m.metaData.pages.organization.editMonitor.component as ComponentType,
  }))
);
export const LazyViewMonitorComponent = lazy(() =>
  import("@agent-management-platform/eval").then((m) => ({
    default: m.metaData.pages.organization.viewMonitor.component as ComponentType,
  }))
);
export const LazyCompareMonitorComponent = lazy(() =>
  import("@agent-management-platform/eval").then((m) => ({
    default: m.metaData.pages.organization.compareMonitor.component as ComponentType,
  }))
);

// LLM Providers
export const LazyLLMProvidersOrg = lazy(() =>
  import("@agent-management-platform/llm-providers").then((m) => ({
    default: m.metaData.levels!.organization as ComponentType,
  }))
);
export const LazyLLMProvidersComponent = lazy(() =>
  import("@agent-management-platform/llm-providers").then((m) => ({
    default: m.metaData.levels!.component as ComponentType,
  }))
);
export const LazyAddLLMProvidersOrg = lazy(() =>
  import("@agent-management-platform/llm-providers").then((m) => ({
    default: m.metaData.levels!.addLLMProvidersOrganization as ComponentType,
  }))
);

// MCP Proxies
export const LazyMCPProxiesOrg = lazy(() =>
  import("@agent-management-platform/mcp-proxies").then((m) => ({
    default: m.metaData.levels!.organization as ComponentType,
  }))
);

// Gateways
export const LazyGatewaysOrg = lazy(() =>
  import("@agent-management-platform/gateways").then((m) => ({
    default: m.metaData.levels!.organization as ComponentType,
  }))
);

// Settings
export const LazySettingsOrg = lazy(() =>
  import("@agent-management-platform/settings").then((m) => ({
    default: m.metaData.levels!.organization as ComponentType,
  }))
);

// Webhooks (project and agent levels; the org level lives in settings)
export const LazyProjectWebhooks = lazy(() =>
  import("@agent-management-platform/settings").then((m) => ({
    default: m.ProjectWebhooks as ComponentType,
  }))
);
export const LazyAgentWebhooks = lazy(() =>
  import("@agent-management-platform/settings").then((m) => ({
    default: m.AgentWebhooks as ComponentType,
  }))
);

// Deployment Pipelines
export const LazyDeploymentPipelinesOrg = lazy(() =>
  import("@agent-management-platform/deployment-pipelines").then((m) => ({
    default: m.metaData.levels!.organization as ComponentType,
  }))
);
export const LazyEnvironmentsOrg = lazy(() =>
  import("@agent-management-platform/deployment-pipelines").then((m) => ({
    default: m.environmentsMetaData.levels!.organization as ComponentType,
  }))
);

// Agent Kind
export const LazyCatalogOrg = lazy(() =>
  import("@agent-management-platform/agent-kind").then((m) => ({
    default: m.metaData.levels!.organization as ComponentType,
  }))
);
export const LazyPublishComponent = lazy(() =>
  import("@agent-management-platform/agent-kind").then((m) => ({
    default: m.metaData.levels!.component as ComponentType,
  }))
);
export const LazyPublishOrg = lazy(() =>
  import("@agent-management-platform/agent-kind").then((m) => ({
    default: m.metaData.levels!.publishOrganization as ComponentType,
  }))
);

// Identity (Environment Thunder Instances) — code-split into its own chunk
export const LazyThunderInstancesOrg = lazy(() =>
  import("@agent-management-platform/env-thunders").then((m) => ({
    default: m.thunderInstancesMetaData.levels!.organization as ComponentType,
  }))
);

// Add / create flows
export const LazyAddNewAgent = lazy(() =>
  import("@agent-management-platform/add-new-agent").then((m) => ({
    default: m.metaData.component as ComponentType,
  }))
);
export const LazyAddNewProject = lazy(() =>
  import("@agent-management-platform/add-new-project").then((m) => ({
    default: m.metaData.component as ComponentType,
  }))
);
