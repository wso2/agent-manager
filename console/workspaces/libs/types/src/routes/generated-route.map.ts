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
        "gateways": {
          "path": "/org/:orgId/gateways",
          "wildPath": "/org/:orgId/gateways/*",
          "children": {
            "add": {
              "path": "/org/:orgId/gateways/add",
              "wildPath": "/org/:orgId/gateways/add/*",
              "children": {}
            },
            "view": {
              "path": "/org/:orgId/gateways/view/:gatewayId",
              "wildPath": "/org/:orgId/gateways/view/:gatewayId/*",
              "children": {}
            },
            "edit": {
              "path": "/org/:orgId/gateways/edit/:gatewayId",
              "wildPath": "/org/:orgId/gateways/edit/:gatewayId/*",
              "children": {}
            }
          }
        },
        "llmProviders": {
          "path": "/org/:orgId/llm-providers",
          "wildPath": "/org/:orgId/llm-providers/*",
          "children": {
            "add": {
              "path": "/org/:orgId/llm-providers/add",
              "wildPath": "/org/:orgId/llm-providers/add/*",
              "children": {}
            },
            "view": {
              "path": "/org/:orgId/llm-providers/view/:providerId",
              "wildPath": "/org/:orgId/llm-providers/view/:providerId/*",
              "children": {
                "deploy": {
                  "path": "/org/:orgId/llm-providers/view/:providerId/deploy",
                  "wildPath": "/org/:orgId/llm-providers/view/:providerId/deploy/*",
                  "children": {}
                }
              }
            }
          }
        },
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
            "evaluators": {
              "path": "/org/:orgId/project/:projectId/evaluators",
              "wildPath": "/org/:orgId/project/:projectId/evaluators/*",
              "children": {
                "create": {
                  "path": "/org/:orgId/project/:projectId/evaluators/create",
                  "wildPath": "/org/:orgId/project/:projectId/evaluators/create/*",
                  "children": {}
                },
                "view": {
                  "path": "/org/:orgId/project/:projectId/evaluators/view/:evaluatorId",
                  "wildPath": "/org/:orgId/project/:projectId/evaluators/view/:evaluatorId/*",
                  "children": {}
                },
                "edit": {
                  "path": "/org/:orgId/project/:projectId/evaluators/edit/:evaluatorId",
                  "wildPath": "/org/:orgId/project/:projectId/evaluators/edit/:evaluatorId/*",
                  "children": {}
                }
              }
            },
            "agents": {
              "path": "/org/:orgId/project/:projectId/agents/:agentId",
              "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/*",
              "children": {
                "configure": {
                  "path": "/org/:orgId/project/:projectId/agents/:agentId/configure",
                  "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/configure/*",
                  "children": {}
                },
                "llmProviders": {
                  "path": "/org/:orgId/project/:projectId/agents/:agentId/llm-providers",
                  "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/llm-providers/*",
                  "children": {
                    "add": {
                      "path": "/org/:orgId/project/:projectId/agents/:agentId/llm-providers/add",
                      "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/llm-providers/add/*",
                      "children": {}
                    },
                    "edit": {
                      "path": "/org/:orgId/project/:projectId/agents/:agentId/llm-providers/edit/:configId",
                      "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/llm-providers/edit/:configId/*",
                      "children": {}
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
                "evaluation": {
                  "path": "/org/:orgId/project/:projectId/agents/:agentId/evaluation",
                  "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/evaluation/*",
                  "children": {
                    "evaluators": {
                      "path": "/org/:orgId/project/:projectId/agents/:agentId/evaluation/evaluators",
                      "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/evaluation/evaluators/*",
                      "children": {
                        "create": {
                          "path": "/org/:orgId/project/:projectId/agents/:agentId/evaluation/evaluators/create",
                          "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/evaluation/evaluators/create/*",
                          "children": {}
                        },
                        "view": {
                          "path": "/org/:orgId/project/:projectId/agents/:agentId/evaluation/evaluators/view/:evaluatorId",
                          "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/evaluation/evaluators/view/:evaluatorId/*",
                          "children": {}
                        },
                        "edit": {
                          "path": "/org/:orgId/project/:projectId/agents/:agentId/evaluation/evaluators/edit/:evaluatorId",
                          "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/evaluation/evaluators/edit/:evaluatorId/*",
                          "children": {}
                        }
                      }
                    },
                    "monitor": {
                      "path": "/org/:orgId/project/:projectId/agents/:agentId/evaluation/monitor",
                      "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/evaluation/monitor/*",
                      "children": {
                        "create": {
                          "path": "/org/:orgId/project/:projectId/agents/:agentId/evaluation/monitor/create",
                          "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/evaluation/monitor/create/*",
                          "children": {}
                        },
                        "view": {
                          "path": "/org/:orgId/project/:projectId/agents/:agentId/evaluation/monitor/view/:monitorId",
                          "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/evaluation/monitor/view/:monitorId/*",
                          "children": {
                            "runs": {
                              "path": "/org/:orgId/project/:projectId/agents/:agentId/evaluation/monitor/view/:monitorId/runs",
                              "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/evaluation/monitor/view/:monitorId/runs/*",
                              "children": {}
                            }
                          }
                        },
                        "edit": {
                          "path": "/org/:orgId/project/:projectId/agents/:agentId/evaluation/monitor/edit/:monitorId",
                          "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/evaluation/monitor/edit/:monitorId/*",
                          "children": {}
                        }
                      }
                    }
                  }
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
                          "children": {}
                        },
                        "logs": {
                          "path": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/observability/logs",
                          "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/observability/logs/*",
                          "children": {}
                        },
                        "metrics": {
                          "path": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/observability/metrics",
                          "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/observability/metrics/*",
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
