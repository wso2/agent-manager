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

import { EvalMonitorsComponent } from "./EvalMonitors.Component";
import { CreateMonitorComponent } from "./CreateMonitor.Component";
import { ViewMonitorComponent } from "./ViewMonitor.Component";
import { EditMonitorComponent } from "./EditMonitor.Component";
import { EvalEvaluatorsComponent } from "./EvalEvaluators.Component";
import { CreateEvaluatorComponent } from "./CreateEvaluator.Component";
import { ViewEvaluatorComponent } from "./ViewEvaluator.Component";
import { EditEvaluatorComponent } from "./EditEvaluator.Component";

import { MonitorCheck, Code } from "@wso2/oxygen-ui-icons-react";

export const metaData = {
  pages: {
    component: {
      evalEvaluators: {
        component: EvalEvaluatorsComponent,
        icon: Code,
        title: "Evaluators",
        description: "Manage built-in and custom evaluators for agent evaluation.",
        path: "/eval/evaluators",
      },
      createEvaluator: {
        component: CreateEvaluatorComponent,
        icon: Code,
        title: "Create Evaluator",
        description: "Wizard for creating a custom evaluator.",
        path: "/eval/evaluators/create",
      },
      viewEvaluator: {
        component: ViewEvaluatorComponent,
        icon: Code,
        title: "View Evaluator",
        description: "Detail page for an evaluator.",
        path: "/eval/evaluators/view",
      },
      editEvaluator: {
        component: EditEvaluatorComponent,
        icon: Code,
        title: "Edit Evaluator",
        description: "Edit an existing custom evaluator.",
      },
      evalMonitors: {
        component: EvalMonitorsComponent,
        icon: MonitorCheck,
        title: "Monitors",
        description:
          "Monitor runtime eval signals once data sources are connected.",
        path: "/eval/monitors",
      },
      createMonitor: {
        component: CreateMonitorComponent,
        icon: MonitorCheck,
        title: "Create Monitor",
        description: "Wizard for registering a new eval monitor.",
        path: "/eval/monitors/create",
      },
      viewMonitor: {
        component: ViewMonitorComponent,
        icon: MonitorCheck,
        title: "View Monitor",
        description: "Detail page for an eval monitor.",
        path: "/eval/monitors/view",
      },
      editMonitor: {
        component: EditMonitorComponent,
        icon: MonitorCheck,
        title: "Edit Monitor",
        description: "Wizard for editing an existing eval monitor.",
      },
    },
  },
};
export {
  EvalMonitorsComponent,
  CreateMonitorComponent,
  ViewMonitorComponent,
  EditMonitorComponent,
  EvalEvaluatorsComponent,
  CreateEvaluatorComponent,
  ViewEvaluatorComponent,
  EditEvaluatorComponent,
};

export default EvalMonitorsComponent;
