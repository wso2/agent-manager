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
              "children": {
                "create": {
                  "path": "/org/:orgId/project/:projectId/newAgent/create",
                  "wildPath": "/org/:orgId/project/:projectId/newAgent/create/*",
                  "children": {}
                },
                "connect": {
                  "path": "/org/:orgId/project/:projectId/newAgent/connect",
                  "wildPath": "/org/:orgId/project/:projectId/newAgent/connect/*",
                  "children": {}
                }
              }
            },
            "agents": {
              "path": "/org/:orgId/project/:projectId/agents/:agentId",
              "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/*",
              "children": {
                "observe": {
                  "path": "/org/:orgId/project/:projectId/agents/:agentId/observe",
                  "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/observe/*",
                  "children": {
                    "traces": {
                      "path": "/org/:orgId/project/:projectId/agents/:agentId/observe/traces",
                      "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/observe/traces/*",
                      "children": {
                        "traceDetails": {
                          "path": "/org/:orgId/project/:projectId/agents/:agentId/observe/traces/:traceId",
                          "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/observe/traces/:traceId/*",
                          "children": {}
                        }
                      }
                    }
                  }
                },
                "build": {
                  "path": "/org/:orgId/project/:projectId/agents/:agentId/build",
                  "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/build/*",
                  "children": {}
                },
                "deployment": {
                  "path": "/org/:orgId/project/:projectId/agents/:agentId/deployment",
                  "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/deployment/*",
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
                      "children": {
                        "api": {
                          "path": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/tryOut/api",
                          "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/tryOut/api/*",
                          "children": {}
                        },
                        "chat": {
                          "path": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/tryOut/chat",
                          "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/tryOut/chat/*",
                          "children": {}
                        }
                      }
                    },
                    "observability": {
                      "path": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/observability",
                      "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/observability/*",
                      "children": {
                        "traces": {
                          "path": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/observability/traces",
                          "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/observability/traces/*",
                          "children": {
                            "traceDetails": {
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
    }
  }
};