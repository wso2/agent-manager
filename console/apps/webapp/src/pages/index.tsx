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

import { lazy, type ComponentType, type FC } from "react";
import { metaData as overviewMetadata } from "@agent-management-platform/overview";
import { metaData as buildMetadata } from "@agent-management-platform/build";
import { metaData as deploymentMetadata } from "@agent-management-platform/deploy";
import { metaData as testMetadata } from "@agent-management-platform/test";
import { metaData as tracesMetadata } from "@agent-management-platform/traces";
import { metaData as logsMetadata } from "@agent-management-platform/logs";
import { metaData as metricsMetadata } from "@agent-management-platform/metrics";
import { metaData as evalMetadata } from "@agent-management-platform/eval";

export * from './Login';

// Navigation pages - imported normally (needed upfront for nav)
export const LazyOverviewOrg = overviewMetadata.levels.organization as FC;
export const LazyOverviewProject = overviewMetadata.levels.project as FC;
export const LazyOverviewComponent = overviewMetadata.levels.component as FC;

export const LazyBuildComponent = buildMetadata.levels.component as FC;

export const LazyDeploymentComponent: FC = () => (
    <deploymentMetadata.levels.component />
);

export const LazyTestComponent = testMetadata.levels.component as FC;
export const LazyTracesComponent = tracesMetadata.levels.component as FC;
export const LazyLogsComponent = logsMetadata.levels.component as FC;
export const LazyMetricsComponent = metricsMetadata.levels.component as FC;
export const LazyEvalMonitorsComponent =
  evalMetadata.pages.component.evalMonitors.component as FC;
export const LazyCreateMonitorComponent =
  evalMetadata.pages.component.createMonitor.component as FC;
  export const LazyEditMonitorComponent =
  evalMetadata.pages.component.editMonitor.component as FC;
export const LazyViewMonitorComponent =
  evalMetadata.pages.component.viewMonitor.component as FC;

// Create pages - lazy loaded (only needed when user creates something)
export const LazyAddNewAgent = lazy(() =>
  import("@agent-management-platform/add-new-agent").then((module) => ({
    default: module.metaData.component as ComponentType,
  }))
);

export const LazyAddNewProject = lazy(() =>
  import("@agent-management-platform/add-new-project").then((module) => ({
    default: module.metaData.component as ComponentType,
  }))
);


