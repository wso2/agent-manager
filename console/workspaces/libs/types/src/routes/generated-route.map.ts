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
      "path": "/organizations/:orgId",
      "wildPath": "/organizations/:orgId/*",
      "children": {
        "profile": {
          "path": "/organizations/:orgId/profile",
          "wildPath": "/organizations/:orgId/profile/*",
          "children": {}
        },
        "thunderInstances": {
          "path": "/organizations/:orgId/thunder-instances",
          "wildPath": "/organizations/:orgId/thunder-instances/*",
          "children": {
            "agents": {
              "path": "/organizations/:orgId/thunder-instances/agents",
              "wildPath": "/organizations/:orgId/thunder-instances/agents/*",
              "children": {
                "detail": {
                  "path": "/organizations/:orgId/thunder-instances/agents/:projectName/:agentName",
                  "wildPath": "/organizations/:orgId/thunder-instances/agents/:projectName/:agentName/*",
                  "children": {}
                }
              }
            },
            "roles": {
              "path": "/organizations/:orgId/thunder-instances/roles",
              "wildPath": "/organizations/:orgId/thunder-instances/roles/*",
              "children": {
                "create": {
                  "path": "/organizations/:orgId/thunder-instances/roles/create",
                  "wildPath": "/organizations/:orgId/thunder-instances/roles/create/*",
                  "children": {}
                },
                "detail": {
                  "path": "/organizations/:orgId/thunder-instances/roles/:roleId",
                  "wildPath": "/organizations/:orgId/thunder-instances/roles/:roleId/*",
                  "children": {}
                }
              }
            },
            "groups": {
              "path": "/organizations/:orgId/thunder-instances/groups",
              "wildPath": "/organizations/:orgId/thunder-instances/groups/*",
              "children": {
                "create": {
                  "path": "/organizations/:orgId/thunder-instances/groups/create",
                  "wildPath": "/organizations/:orgId/thunder-instances/groups/create/*",
                  "children": {}
                },
                "detail": {
                  "path": "/organizations/:orgId/thunder-instances/groups/:groupId",
                  "wildPath": "/organizations/:orgId/thunder-instances/groups/:groupId/*",
                  "children": {}
                }
              }
            }
          }
        },
        "gateways": {
          "path": "/organizations/:orgId/gateways",
          "wildPath": "/organizations/:orgId/gateways/*",
          "children": {
            "view": {
              "path": "/organizations/:orgId/gateways/view/:gatewayId",
              "wildPath": "/organizations/:orgId/gateways/view/:gatewayId/*",
              "children": {}
            },
            "edit": {
              "path": "/organizations/:orgId/gateways/edit/:gatewayId",
              "wildPath": "/organizations/:orgId/gateways/edit/:gatewayId/*",
              "children": {}
            }
          }
        },
        "llmProviders": {
          "path": "/organizations/:orgId/llm-providers",
          "wildPath": "/organizations/:orgId/llm-providers/*",
          "children": {
            "add": {
              "path": "/organizations/:orgId/llm-providers/add",
              "wildPath": "/organizations/:orgId/llm-providers/add/*",
              "children": {}
            },
            "view": {
              "path": "/organizations/:orgId/llm-providers/view/:providerId",
              "wildPath": "/organizations/:orgId/llm-providers/view/:providerId/*",
              "children": {
                "edit": {
                  "path": "/organizations/:orgId/llm-providers/view/:providerId/edit",
                  "wildPath": "/organizations/:orgId/llm-providers/view/:providerId/edit/*",
                  "children": {}
                },
                "deploy": {
                  "path": "/organizations/:orgId/llm-providers/view/:providerId/deploy",
                  "wildPath": "/organizations/:orgId/llm-providers/view/:providerId/deploy/*",
                  "children": {}
                }
              }
            }
          }
        },
        "settings": {
          "path": "/organizations/:orgId/settings",
          "wildPath": "/organizations/:orgId/settings/*",
          "children": {
            "identities": {
              "path": "/organizations/:orgId/settings/identities",
              "wildPath": "/organizations/:orgId/settings/identities/*",
              "children": {
                "users": {
                  "path": "/organizations/:orgId/settings/identities/users",
                  "wildPath": "/organizations/:orgId/settings/identities/users/*",
                  "children": {
                    "detail": {
                      "path": "/organizations/:orgId/settings/identities/users/:userId",
                      "wildPath": "/organizations/:orgId/settings/identities/users/:userId/*",
                      "children": {}
                    }
                  }
                },
                "roles": {
                  "path": "/organizations/:orgId/settings/identities/roles",
                  "wildPath": "/organizations/:orgId/settings/identities/roles/*",
                  "children": {
                    "detail": {
                      "path": "/organizations/:orgId/settings/identities/roles/:roleId",
                      "wildPath": "/organizations/:orgId/settings/identities/roles/:roleId/*",
                      "children": {}
                    }
                  }
                },
                "groups": {
                  "path": "/organizations/:orgId/settings/identities/groups",
                  "wildPath": "/organizations/:orgId/settings/identities/groups/*",
                  "children": {
                    "detail": {
                      "path": "/organizations/:orgId/settings/identities/groups/:groupId",
                      "wildPath": "/organizations/:orgId/settings/identities/groups/:groupId/*",
                      "children": {}
                    }
                  }
                }
              }
            }
          }
        },
        "mcpProxies": {
          "path": "/organizations/:orgId/mcp-proxies",
          "wildPath": "/organizations/:orgId/mcp-proxies/*",
          "children": {
            "add": {
              "path": "/organizations/:orgId/mcp-proxies/add",
              "wildPath": "/organizations/:orgId/mcp-proxies/add/*",
              "children": {}
            },
            "view": {
              "path": "/organizations/:orgId/mcp-proxies/view/:proxyId",
              "wildPath": "/organizations/:orgId/mcp-proxies/view/:proxyId/*",
              "children": {
                "edit": {
                  "path": "/organizations/:orgId/mcp-proxies/view/:proxyId/edit",
                  "wildPath": "/organizations/:orgId/mcp-proxies/view/:proxyId/edit/*",
                  "children": {}
                }
              }
            }
          }
        },
        "evaluators": {
          "path": "/organizations/:orgId/evaluators",
          "wildPath": "/organizations/:orgId/evaluators/*",
          "children": {
            "create": {
              "path": "/organizations/:orgId/evaluators/create",
              "wildPath": "/organizations/:orgId/evaluators/create/*",
              "children": {}
            },
            "view": {
              "path": "/organizations/:orgId/evaluators/view/:evaluatorId",
              "wildPath": "/organizations/:orgId/evaluators/view/:evaluatorId/*",
              "children": {}
            },
            "edit": {
              "path": "/organizations/:orgId/evaluators/edit/:evaluatorId",
              "wildPath": "/organizations/:orgId/evaluators/edit/:evaluatorId/*",
              "children": {}
            }
          }
        },
        "events": {
          "path": "/organizations/:orgId/events",
          "wildPath": "/organizations/:orgId/events/*",
          "children": {}
        },
        "deploymentPipelines": {
          "path": "/organizations/:orgId/deployment-pipelines",
          "wildPath": "/organizations/:orgId/deployment-pipelines/*",
          "children": {
            "view": {
              "path": "/organizations/:orgId/deployment-pipelines/:pipelineName",
              "wildPath": "/organizations/:orgId/deployment-pipelines/:pipelineName/*",
              "children": {}
            }
          }
        },
        "environments": {
          "path": "/organizations/:orgId/environments",
          "wildPath": "/organizations/:orgId/environments/*",
          "children": {
            "view": {
              "path": "/organizations/:orgId/environments/:envName",
              "wildPath": "/organizations/:orgId/environments/:envName/*",
              "children": {
                "edit": {
                  "path": "/organizations/:orgId/environments/:envName/edit",
                  "wildPath": "/organizations/:orgId/environments/:envName/edit/*",
                  "children": {}
                },
                "identityProvider": {
                  "path": "/organizations/:orgId/environments/:envName/identity-provider",
                  "wildPath": "/organizations/:orgId/environments/:envName/identity-provider/*",
                  "children": {}
                }
              }
            }
          }
        },
        "catalog": {
          "path": "/organizations/:orgId/catalog",
          "wildPath": "/organizations/:orgId/catalog/*",
          "children": {
            "kindDetails": {
              "path": "/organizations/:orgId/catalog/kind/:kindId",
              "wildPath": "/organizations/:orgId/catalog/kind/:kindId/*",
              "children": {}
            }
          }
        },
        "newProject": {
          "path": "/organizations/:orgId/newProject",
          "wildPath": "/organizations/:orgId/newProject/*",
          "children": {}
        },
        "projectListing": {
          "path": "/organizations/:orgId/projects",
          "wildPath": "/organizations/:orgId/projects/*",
          "children": {}
        },
        "projects": {
          "path": "/organizations/:orgId/projects/:projectId",
          "wildPath": "/organizations/:orgId/projects/:projectId/*",
          "children": {
            "events": {
              "path": "/organizations/:orgId/projects/:projectId/events",
              "wildPath": "/organizations/:orgId/projects/:projectId/events/*",
              "children": {}
            },
            "mcpProxies": {
              "path": "/org/:orgId/project/:projectId/mcp-proxies",
              "wildPath": "/org/:orgId/project/:projectId/mcp-proxies/*",
              "children": {
                "add": {
                  "path": "/org/:orgId/project/:projectId/mcp-proxies/add",
                  "wildPath": "/org/:orgId/project/:projectId/mcp-proxies/add/*",
                  "children": {}
                },
                "view": {
                  "path": "/org/:orgId/project/:projectId/mcp-proxies/view/:proxyId",
                  "wildPath": "/org/:orgId/project/:projectId/mcp-proxies/view/:proxyId/*",
                  "children": {
                    "edit": {
                      "path": "/org/:orgId/project/:projectId/mcp-proxies/view/:proxyId/edit",
                      "wildPath": "/org/:orgId/project/:projectId/mcp-proxies/view/:proxyId/edit/*",
                      "children": {}
                    }
                  }
                }
              }
            },
            "agentListing": {
              "path": "/organizations/:orgId/projects/:projectId/agents",
              "wildPath": "/organizations/:orgId/projects/:projectId/agents/*",
              "children": {}
            },
            "edit": {
              "path": "/organizations/:orgId/projects/:projectId/edit",
              "wildPath": "/organizations/:orgId/projects/:projectId/edit/*",
              "children": {}
            },
            "newAgent": {
              "path": "/organizations/:orgId/projects/:projectId/newAgent",
              "wildPath": "/organizations/:orgId/projects/:projectId/newAgent/*",
              "children": {
                "create": {
                  "path": "/organizations/:orgId/projects/:projectId/newAgent/create",
                  "wildPath": "/organizations/:orgId/projects/:projectId/newAgent/create/*",
                  "children": {
                    "catalog": {
                      "path": "/organizations/:orgId/projects/:projectId/newAgent/create/catalog",
                      "wildPath": "/organizations/:orgId/projects/:projectId/newAgent/create/catalog/*",
                      "children": {
                        "withKind": {
                          "path": "/organizations/:orgId/projects/:projectId/newAgent/create/catalog/:kindId",
                          "wildPath": "/organizations/:orgId/projects/:projectId/newAgent/create/catalog/:kindId/*",
                          "children": {}
                        }
                      }
                    },
                    "source": {
                      "path": "/organizations/:orgId/projects/:projectId/newAgent/create/source",
                      "wildPath": "/organizations/:orgId/projects/:projectId/newAgent/create/source/*",
                      "children": {}
                    }
                  }
                },
                "connect": {
                  "path": "/organizations/:orgId/projects/:projectId/newAgent/connect",
                  "wildPath": "/organizations/:orgId/projects/:projectId/newAgent/connect/*",
                  "children": {}
                }
              }
            },
            "agents": {
              "path": "/organizations/:orgId/projects/:projectId/agents/:agentId",
              "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/*",
              "children": {
                "edit": {
                  "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/edit",
                  "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/edit/*",
                  "children": {}
                },
                "agentId": {
                  "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/agent-id",
                  "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/agent-id/*",
                  "children": {}
                },
                "configure": {
                  "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/configure",
                  "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/configure/*",
                  "children": {
                    "llmProviders": {
                      "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/configure/llm-providers",
                      "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/configure/llm-providers/*",
                      "children": {
                        "add": {
                          "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/configure/llm-providers/add",
                          "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/configure/llm-providers/add/*",
                          "children": {}
                        },
                        "view": {
                          "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/configure/llm-providers/view/:configId",
                          "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/configure/llm-providers/view/:configId/*",
                          "children": {}
                        },
                        "edit": {
                          "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/configure/llm-providers/edit/:configId",
                          "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/configure/llm-providers/edit/:configId/*",
                          "children": {}
                        }
                      }
                    },
                    "mcpProxies": {
                      "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/configure/mcp-proxies",
                      "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/configure/mcp-proxies/*",
                      "children": {
                        "add": {
                          "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/configure/mcp-proxies/add",
                          "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/configure/mcp-proxies/add/*",
                          "children": {}
                        },
                        "view": {
                          "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/configure/mcp-proxies/view/:proxyId",
                          "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/configure/mcp-proxies/view/:proxyId/*",
                          "children": {}
                        }
                      }
                    }
                  }
                },
                "build": {
                  "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/build",
                  "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/build/*",
                  "children": {}
                },
                "events": {
                  "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/events",
                  "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/events/*",
                  "children": {}
                },
                "deployment": {
                  "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/deployment",
                  "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/deployment/*",
                  "children": {}
                },
                "publish": {
                  "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/publish",
                  "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/publish/*",
                  "children": {
                    "createNewVersion": {
                      "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/publish/create-new-version",
                      "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/publish/create-new-version/*",
                      "children": {}
                    },
                    "editKind": {
                      "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/publish/edit-kind",
                      "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/publish/edit-kind/*",
                      "children": {}
                    },
                    "versionDetails": {
                      "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/publish/version-details/:versionId",
                      "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/publish/version-details/:versionId/*",
                      "children": {}
                    }
                  }
                },
                "environment": {
                  "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId",
                  "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/*",
                  "children": {
                    "evaluation": {
                      "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/evaluation",
                      "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/evaluation/*",
                      "children": {
                        "monitor": {
                          "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/evaluation/monitor",
                          "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/evaluation/monitor/*",
                          "children": {
                            "create": {
                              "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/evaluation/monitor/create",
                              "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/evaluation/monitor/create/*",
                              "children": {}
                            },
                            "view": {
                              "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/evaluation/monitor/view/:monitorId",
                              "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/evaluation/monitor/view/:monitorId/*",
                              "children": {
                                "runs": {
                                  "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/evaluation/monitor/view/:monitorId/runs",
                                  "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/evaluation/monitor/view/:monitorId/runs/*",
                                  "children": {}
                                }
                              }
                            },
                            "edit": {
                              "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/evaluation/monitor/edit/:monitorId",
                              "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/evaluation/monitor/edit/:monitorId/*",
                              "children": {}
                            },
                            "compare": {
                              "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/evaluation/monitor/compare/:monitorId",
                              "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/evaluation/monitor/compare/:monitorId/*",
                              "children": {}
                            }
                          }
                        }
                      }
                    },
                    "deploy": {
                      "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/deploy",
                      "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/deploy/*",
                      "children": {}
                    },
                    "security": {
                      "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/security",
                      "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/security/*",
                      "children": {}
                    },
                    "tryOut": {
                      "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/tryOut",
                      "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/tryOut/*",
                      "children": {
                        "api": {
                          "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/tryOut/api",
                          "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/tryOut/api/*",
                          "children": {}
                        },
                        "chat": {
                          "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/tryOut/chat",
                          "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/tryOut/chat/*",
                          "children": {}
                        }
                      }
                    },
                    "observability": {
                      "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/observability",
                      "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/observability/*",
                      "children": {
                        "traces": {
                          "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/observability/traces",
                          "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/observability/traces/*",
                          "children": {}
                        },
                        "logs": {
                          "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/observability/logs",
                          "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/observability/logs/*",
                          "children": {}
                        },
                        "metrics": {
                          "path": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/observability/metrics",
                          "wildPath": "/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/observability/metrics/*",
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
