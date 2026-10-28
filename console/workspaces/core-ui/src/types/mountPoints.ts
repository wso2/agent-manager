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

export enum MountPoints {
    ComponentLevelPage = "component-level-page",
    OrgLevelPage = "org-level-page",
    ProjectLevelPage = "project-level-page",
    TopLeftPanel = "top-left-panel",
    TopRightPanel = "top-right-panel",
    TopLogo = "top-logo",
    BottomLeftPanel = "bottom-left-panel",
    BottomRightPanel = "bottom-right-panel",
    LeftNavItem = "left-nav-item",
    // A deployment can inject its own environment create/delete drawer components
    // at these mount points. When nothing is injected, the built-in script-based
    // drawers are used (the self-hosted default).
    EnvironmentCreateDrawer = "environment-create-drawer",
    EnvironmentDeleteDrawer = "environment-delete-drawer",
    // A deployment can inject its own private-repository source component (repo URL,
    // branch, app path and credential selection) at this mount point. When nothing is
    // injected, the built-in PAT git-secret UI is used (the self-hosted default).
    PrivateRepoSource = "private-repo-source",
}
