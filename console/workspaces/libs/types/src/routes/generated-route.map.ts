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