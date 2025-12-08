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
