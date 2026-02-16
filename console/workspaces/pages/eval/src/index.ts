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

import { EvalMonitorsComponent } from "./EvalMonitors.Component";
import { CreateMonitorComponent } from "./CreateMonitor.Component";
import { ViewMonitorComponent } from "./ViewMonitor.Component";
import { Package } from "@wso2/oxygen-ui-icons-react";

export const metaData = {
  pages: {
    component: {
      evalMonitors: {
        component: EvalMonitorsComponent,
        icon: Package,
        title: "Monitors",
        description: "Monitor runtime eval signals once data sources are connected.",
        path: "/eval/monitors",
      },
      createMonitor: {
        component: CreateMonitorComponent,
        icon: Package,
        title: "Create Monitor",
        description: "Wizard for registering a new eval monitor.",
        path: "/eval/monitors/create",
      },
      viewMonitor: {
        component: ViewMonitorComponent,
        icon: Package,
        title: "View Monitor",
        description: "Detail page for an eval monitor.",
        path: "/eval/monitors/view",
      },
    },
  },
};
export { EvalMonitorsComponent, CreateMonitorComponent, ViewMonitorComponent };

export default EvalMonitorsComponent;
