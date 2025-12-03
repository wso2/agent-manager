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

export const generatedRouteMap =  {
  "path": "",
  "wildPath": "*",
  "children": {
    "login": {
      "path": "/login",
      "wildPath": "/login/*",
      "children": {}
    },
    "org": {
      "path": "/org/:orgId",
      "wildPath": "/org/:orgId/*",
      "children": {
        "newProject": {
          "path": "/org/:orgId/newProject",
          "wildPath": "/org/:orgId/newProject/*",
          "children": {}
        },
        "projects": {
          "path": "/org/:orgId/project/:projectId",
          "wildPath": "/org/:orgId/project/:projectId/*",
          "children": {
            "newAgent": {
              "path": "/org/:orgId/project/:projectId/newAgent",
              "wildPath": "/org/:orgId/project/:projectId/newAgent/*",
              "children": {}
            },
            "agents": {
              "path": "/org/:orgId/project/:projectId/agents/:agentId",
              "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/*",
              "children": {
                "traces": {
                  "path": "/org/:orgId/project/:projectId/agents/:agentId/traces/:traceId",
                  "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/traces/:traceId/*",
                  "children": {}
                },
                "build": {
                  "path": "/org/:orgId/project/:projectId/agents/:agentId/build",
                  "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/build/*",
                  "children": {}
                },
                "environment": {
                  "path": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId",
                  "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/*",
                  "children": {
                    "deploy": {
                      "path": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/deploy",
                      "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/deploy/*",
                      "children": {}
                    },
                    "tryOut": {
                      "path": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/tryOut",
                      "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/tryOut/*",
                      "children": {}
                    },
                    "observability": {
                      "path": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/observability",
                      "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/observability/*",
                      "children": {
                        "traces": {
                          "path": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/observability/traces/:traceId",
                          "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/observability/traces/:traceId/*",
                          "children": {}
                        }
                      }
                    }
                  }
                }
              }
            }
          }
        }
      }
    }
  }
};