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
            },
            "alerting": {
              "path": "/org/:orgId/settings/alerting",
              "wildPath": "/org/:orgId/settings/alerting/*",
              "children": {}
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
              "children": {}
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
        "deploymentPipelines": {
          "path": "/org/:orgId/deployment-pipelines",
          "wildPath": "/org/:orgId/deployment-pipelines/*",
          "children": {}
        },
        "environments": {
          "path": "/org/:orgId/environments",
          "wildPath": "/org/:orgId/environments/*",
          "children": {
            "view": {
              "path": "/org/:orgId/environments/:envName",
              "wildPath": "/org/:orgId/environments/:envName/*",
              "children": {
                "identityProvider": {
                  "path": "/org/:orgId/environments/:envName/identity-provider",
                  "wildPath": "/org/:orgId/environments/:envName/identity-provider/*",
                  "children": {}
                }
              }
            }
          }
        },
        "catalog": {
          "path": "/org/:orgId/catalog",
          "wildPath": "/org/:orgId/catalog/*",
          "children": {
            "kindDetails": {
              "path": "/org/:orgId/catalog/kind/:kindId",
              "wildPath": "/org/:orgId/catalog/kind/:kindId/*",
              "children": {}
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
                  "children": {
                    "catalog": {
                      "path": "/org/:orgId/project/:projectId/newAgent/create/catalog",
                      "wildPath": "/org/:orgId/project/:projectId/newAgent/create/catalog/*",
                      "children": {
                        "withKind": {
                          "path": "/org/:orgId/project/:projectId/newAgent/create/catalog/:kindId",
                          "wildPath": "/org/:orgId/project/:projectId/newAgent/create/catalog/:kindId/*",
                          "children": {}
                        }
                      }
                    },
                    "source": {
                      "path": "/org/:orgId/project/:projectId/newAgent/create/source",
                      "wildPath": "/org/:orgId/project/:projectId/newAgent/create/source/*",
                      "children": {}
                    }
                  }
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
                "publish": {
                  "path": "/org/:orgId/project/:projectId/agents/:agentId/publish",
                  "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/publish/*",
                  "children": {
                    "createNewVersion": {
                      "path": "/org/:orgId/project/:projectId/agents/:agentId/publish/create-new-version",
                      "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/publish/create-new-version/*",
                      "children": {}
                    },
                    "editKind": {
                      "path": "/org/:orgId/project/:projectId/agents/:agentId/publish/edit-kind",
                      "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/publish/edit-kind/*",
                      "children": {}
                    },
                    "versionDetails": {
                      "path": "/org/:orgId/project/:projectId/agents/:agentId/publish/version-details/:versionId",
                      "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/publish/version-details/:versionId/*",
                      "children": {}
                    }
                  }
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
                    "deploy": {
                      "path": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/deploy",
                      "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/deploy/*",
                      "children": {}
                    },
                    "security": {
                      "path": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/security",
                      "wildPath": "/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/security/*",
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
