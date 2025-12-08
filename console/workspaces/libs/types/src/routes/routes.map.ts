import { type AppRoute } from "./types";

export const rootRouteMap: AppRoute = {
    path: '',
    children: {
    login: {
        path: '/login',
        index: true,
        children: {},
    },
    org: {
        path: '/org/:orgId',
        index: true,
        children: {
            newProject: {
                path: 'newProject',
                index: true,
                children: {},
            },
            projects: {
                path: 'project/:projectId',
                index: true,
                children: {
                    newAgent: {
                        path: 'newAgent',
                        index: true,
                        children: {
                            create: {
                                path: 'create',
                                index: true,
                                children: {},
                            },
                            connect: {
                                path: 'connect',
                                index: true,
                                children: {},
                            },
                        },
                    },
                    agents: {
                        path: 'agents/:agentId',
                        index: true,
                        children: {
                            observe:{
                                path: 'observe',
                                index: true,
                                children: {
                                    traces: {
                                        path: 'traces',
                                        index: true,
                                        children: {
                                            traceDetails: {
                                                path: ':traceId',
                                                index: true,
                                                children: {},
                                            },
                                        },
                                    }
                                }
                            },
                            build: {
                                path: 'build',
                                index: true,
                                children: {},
                            },
                            deployment:{
                                path: "deployment",
                                index: true,
                                children: {},
                            },
                            environment:{
                                path: "environment/:envId",
                                index:false,
                                children:{
                                    deploy: {
                                        path: 'deploy',
                                        index: true,
                                        children: {},
                                    },
                                    tryOut: {
                                        path: 'tryOut',
                                        index: true,
                                        children: {
                                            api:{
                                                path: 'api',
                                                index: true,
                                                children: {},
                                            },
                                            chat:{
                                                path: 'chat',
                                                index: true,
                                                children: {},
                                            },
                                        },
                                    },
                                    observability: {
                                        path: 'observability',
                                        index: true,
                                        children: {
                                            traces: {
                                                path: 'traces',
                                                index: true,
                                                children: {
                                                    traceDetails: {
                                                        path: ':traceId',
                                                        index: true,
                                                        children: {},
                                                    },
                                                },
                                            },
                                        },
                                    },
                                }
                            },
                        },
                    },
                },
            },
        },
    },
    },
}
