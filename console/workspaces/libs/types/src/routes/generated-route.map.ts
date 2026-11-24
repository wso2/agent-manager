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
        "profile": {
          "path": "/org/:orgId/profile",
          "wildPath": "/org/:orgId/profile/*",
          "children": {}
        },
        "thunderInstances": {
          "path": "/org/:orgId/thunder-instances",
          "wildPath": "/org/:orgId/thunder-instances/*",
          "children": {
            "agents": {
              "path": "/org/:orgId/thunder-instances/agents",
              "wildPath": "/org/:orgId/thunder-instances/agents/*",
              "children": {
                "detail": {
                  "path": "/org/:orgId/thunder-instances/agents/:projectName/:agentName",
                  "wildPath": "/org/:orgId/thunder-instances/agents/:projectName/:agentName/*",
                  "children": {}
                }
              }
            },
            "roles": {
              "path": "/org/:orgId/thunder-instances/roles",
              "wildPath": "/org/:orgId/thunder-instances/roles/*",
              "children": {
                "create": {
                  "path": "/org/:orgId/thunder-instances/roles/create",
                  "wildPath": "/org/:orgId/thunder-instances/roles/create/*",
                  "children": {}
                },
                "detail": {
                  "path": "/org/:orgId/thunder-instances/roles/:roleId",
                  "wildPath": "/org/:orgId/thunder-instances/roles/:roleId/*",
                  "children": {}
                }
              }
            },
            "groups": {
              "path": "/org/:orgId/thunder-instances/groups",
              "wildPath": "/org/:orgId/thunder-instances/groups/*",
              "children": {
                "create": {
                  "path": "/org/:orgId/thunder-instances/groups/create",
                  "wildPath": "/org/:orgId/thunder-instances/groups/create/*",
                  "children": {}
                },
                "detail": {
                  "path": "/org/:orgId/thunder-instances/groups/:groupId",
                  "wildPath": "/org/:orgId/thunder-instances/groups/:groupId/*",
                  "children": {}
                }
              }
            }
          }
        },
        "gateways": {
          "path": "/org/:orgId/gateways",
          "wildPath": "/org/:orgId/gateways/*",
          "children": {
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
                "edit": {
                  "path": "/org/:orgId/llm-providers/view/:providerId/edit",
                  "wildPath": "/org/:orgId/llm-providers/view/:providerId/edit/*",
                  "children": {}
                },
                "deploy": {
                  "path": "/org/:orgId/llm-providers/view/:providerId/deploy",
                  "wildPath": "/org/:orgId/llm-providers/view/:providerId/deploy/*",
                  "children": {}
                }
              }
            }
          }
        },
        "settings": {
          "path": "/org/:orgId/settings",
          "wildPath": "/org/:orgId/settings/*",
          "children": {
            "identities": {
              "path": "/org/:orgId/settings/identities",
              "wildPath": "/org/:orgId/settings/identities/*",
              "children": {
                "users": {
                  "path": "/org/:orgId/settings/identities/users",
                  "wildPath": "/org/:orgId/settings/identities/users/*",
                  "children": {
                    "detail": {
                      "path": "/org/:orgId/settings/identities/users/:userId",
                      "wildPath": "/org/:orgId/settings/identities/users/:userId/*",
                      "children": {}
                    }
                  }
                },
                "roles": {
                  "path": "/org/:orgId/settings/identities/roles",
                  "wildPath": "/org/:orgId/settings/identities/roles/*",
                  "children": {
                    "detail": {
                      "path": "/org/:orgId/settings/identities/roles/:roleId",
                      "wildPath": "/org/:orgId/settings/identities/roles/:roleId/*",
                      "children": {}
                    }
                  }
                },
                "groups": {
                  "path": "/org/:orgId/settings/identities/groups",
                  "wildPath": "/org/:orgId/settings/identities/groups/*",
                  "children": {
                    "detail": {
                      "path": "/org/:orgId/settings/identities/groups/:groupId",
                      "wildPath": "/org/:orgId/settings/identities/groups/:groupId/*",
                      "children": {}
                    }
                  }
                }
              }
            }
          }
        },
        "mcpProxies": {
          "path": "/org/:orgId/mcp-proxies",
          "wildPath": "/org/:orgId/mcp-proxies/*",
          "children": {
            "add": {
              "path": "/org/:orgId/mcp-proxies/add",
              "wildPath": "/org/:orgId/mcp-proxies/add/*",
              "children": {}
            },
            "view": {
              "path": "/org/:orgId/mcp-proxies/view/:proxyId",
              "wildPath": "/org/:orgId/mcp-proxies/view/:proxyId/*",
              "children": {
                "edit": {
                  "path": "/org/:orgId/mcp-proxies/view/:proxyId/edit",
                  "wildPath": "/org/:orgId/mcp-proxies/view/:proxyId/edit/*",
                  "children": {}
                }
              }
            }
          }
        },
        "evaluators": {
          "path": "/org/:orgId/evaluators",
          "wildPath": "/org/:orgId/evaluators/*",
          "children": {
            "create": {
              "path": "/org/:orgId/evaluators/create",
              "wildPath": "/org/:orgId/evaluators/create/*",
              "children": {}
            },
            "view": {
              "path": "/org/:orgId/evaluators/view/:evaluatorId",
              "wildPath": "/org/:orgId/evaluators/view/:evaluatorId/*",
              "children": {}
            },
            "edit": {
              "path": "/org/:orgId/evaluators/edit/:evaluatorId",
              "wildPath": "/org/:orgId/evaluators/edit/:evaluatorId/*",
              "children": {}
            }
          }
        },
        "events": {
          "path": "/org/:orgId/events",
          "wildPath": "/org/:orgId/events/*",
          "children": {}
        },
        "costManagement": {
          "path": "/org/:orgId/cost-management",
          "wildPath": "/org/:orgId/cost-management/*",
          "children": {
            "unpricedRequests": {
              "path": "/org/:orgId/cost-management/unpriced-requests",
              "wildPath": "/org/:orgId/cost-management/unpriced-requests/*",
              "children": {}
            },
            "budgets": {
              "path": "/org/:orgId/cost-management/budgets",
              "wildPath": "/org/:orgId/cost-management/budgets/*",
              "children": {
                "create": {
                  "path": "/org/:orgId/cost-management/budgets/create",
                  "wildPath": "/org/:orgId/cost-management/budgets/create/*",
                  "children": {}
                },
                "view": {
                  "path": "/org/:orgId/cost-management/budgets/view/:budgetName",
                  "wildPath": "/org/:orgId/cost-management/budgets/view/:budgetName/*",
                  "children": {
                    "edit": {
                      "path": "/org/:orgId/cost-management/budgets/view/:budgetName/edit",
                      "wildPath": "/org/:orgId/cost-management/budgets/view/:budgetName/edit/*",
                      "children": {}
                    }
                  }
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
        "projectListing": {
          "path": "/org/:orgId/projects",
          "wildPath": "/org/:orgId/projects/*",
          "children": {}
        },
        "projects": {
          "path": "/org/:orgId/project/:projectId",
          "wildPath": "/org/:orgId/project/:projectId/*",
          "children": {
            "events": {
              "path": "/org/:orgId/project/:projectId/events",
              "wildPath": "/org/:orgId/project/:projectId/events/*",
              "children": {}
            },
            "costManagement": {
              "path": "/org/:orgId/project/:projectId/cost-management",
              "wildPath": "/org/:orgId/project/:projectId/cost-management/*",
              "children": {
                "unpricedRequests": {
                  "path": "/org/:orgId/project/:projectId/cost-management/unpriced-requests",
                  "wildPath": "/org/:orgId/project/:projectId/cost-management/unpriced-requests/*",
                  "children": {}
                },
                "budgets": {
                  "path": "/org/:orgId/project/:projectId/cost-management/budgets",
                  "wildPath": "/org/:orgId/project/:projectId/cost-management/budgets/*",
                  "children": {
                    "create": {
                      "path": "/org/:orgId/project/:projectId/cost-management/budgets/create",
                      "wildPath": "/org/:orgId/project/:projectId/cost-management/budgets/create/*",
                      "children": {}
                    },
                    "view": {
                      "path": "/org/:orgId/project/:projectId/cost-management/budgets/view/:budgetName",
                      "wildPath": "/org/:orgId/project/:projectId/cost-management/budgets/view/:budgetName/*",
                      "children": {
                        "edit": {
                          "path": "/org/:orgId/project/:projectId/cost-management/budgets/view/:budgetName/edit",
                          "wildPath": "/org/:orgId/project/:projectId/cost-management/budgets/view/:budgetName/edit/*",
                          "children": {}
                        }
                      }
                    }
                  }
                }
              }
            },
            "agentListing": {
              "path": "/org/:orgId/project/:projectId/agents",
              "wildPath": "/org/:orgId/project/:projectId/agents/*",
              "children": {}
            },
            "edit": {
              "path": "/org/:orgId/project/:projectId/edit",
              "wildPath": "/org/:orgId/project/:projectId/edit/*",
              "children": {}
            },
            "newAgent": {
              "path": "/org/:orgId/project/:projectId/newAgent",
              "wildPath": "/org/:orgId/project/:projectId/newAgent/*",
              "children": {
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
                "costManagement": {
                  "path": "/org/:orgId/project/:projectId/agents/:agentId/cost-management",
                  "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/cost-management/*",
                  "children": {
                    "unpricedRequests": {
                      "path": "/org/:orgId/project/:projectId/agents/:agentId/cost-management/unpriced-requests",
                      "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/cost-management/unpriced-requests/*",
                      "children": {}
                    },
                    "budgets": {
                      "path": "/org/:orgId/project/:projectId/agents/:agentId/cost-management/budgets",
                      "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/cost-management/budgets/*",
                      "children": {
                        "view": {
                          "path": "/org/:orgId/project/:projectId/agents/:agentId/cost-management/budgets/view/:budgetName",
                          "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/cost-management/budgets/view/:budgetName/*",
                          "children": {}
                        }
                      }
                    }
                  }
                },
                "edit": {
                  "path": "/org/:orgId/project/:projectId/agents/:agentId/edit",
                  "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/edit/*",
                  "children": {}
                },
                "agentId": {
                  "path": "/org/:orgId/project/:projectId/agents/:agentId/agent-id",
                  "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/agent-id/*",
                  "children": {}
                },
                "configure": {
                  "path": "/org/:orgId/project/:projectId/agents/:agentId/configure",
                  "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/configure/*",
                  "children": {
                    "llmProviders": {
                      "path": "/org/:orgId/project/:projectId/agents/:agentId/configure/llm-providers",
                      "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/configure/llm-providers/*",
                      "children": {
                        "add": {
                          "path": "/org/:orgId/project/:projectId/agents/:agentId/configure/llm-providers/add",
                          "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/configure/llm-providers/add/*",
                          "children": {}
                        },
                        "view": {
                          "path": "/org/:orgId/project/:projectId/agents/:agentId/configure/llm-providers/view/:configId",
                          "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/configure/llm-providers/view/:configId/*",
                          "children": {}
                        },
                        "edit": {
                          "path": "/org/:orgId/project/:projectId/agents/:agentId/configure/llm-providers/edit/:configId",
                          "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/configure/llm-providers/edit/:configId/*",
                          "children": {}
                        }
                      }
                    },
                    "mcpProxies": {
                      "path": "/org/:orgId/project/:projectId/agents/:agentId/configure/mcp-proxies",
                      "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/configure/mcp-proxies/*",
                      "children": {
                        "add": {
                          "path": "/org/:orgId/project/:projectId/agents/:agentId/configure/mcp-proxies/add",
                          "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/configure/mcp-proxies/add/*",
                          "children": {}
                        },
                        "view": {
                          "path": "/org/:orgId/project/:projectId/agents/:agentId/configure/mcp-proxies/view/:proxyId",
                          "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/configure/mcp-proxies/view/:proxyId/*",
                          "children": {}
                        }
                      }
                    }
                  }
                },
                "events": {
                  "path": "/org/:orgId/project/:projectId/agents/:agentId/events",
                  "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/events/*",
                  "children": {}
                },
                "environment": {
                  "path": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId",
                  "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/*",
                  "children": {
                    "evaluation": {
                      "path": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/evaluation",
                      "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/evaluation/*",
                      "children": {
                        "monitor": {
                          "path": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/evaluation/monitor",
                          "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/evaluation/monitor/*",
                          "children": {
                            "create": {
                              "path": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/evaluation/monitor/create",
                              "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/evaluation/monitor/create/*",
                              "children": {}
                            },
                            "view": {
                              "path": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/evaluation/monitor/view/:monitorId",
                              "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/evaluation/monitor/view/:monitorId/*",
                              "children": {
                                "runs": {
                                  "path": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/evaluation/monitor/view/:monitorId/runs",
                                  "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/evaluation/monitor/view/:monitorId/runs/*",
                                  "children": {}
                                }
                              }
                            },
                            "edit": {
                              "path": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/evaluation/monitor/edit/:monitorId",
                              "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/evaluation/monitor/edit/:monitorId/*",
                              "children": {}
                            },
                            "compare": {
                              "path": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/evaluation/monitor/compare/:monitorId",
                              "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/evaluation/monitor/compare/:monitorId/*",
                              "children": {}
                            }
                          }
                        }
                      }
                    },
                    "security": {
                      "path": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/security",
                      "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/security/*",
                      "children": {}
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
