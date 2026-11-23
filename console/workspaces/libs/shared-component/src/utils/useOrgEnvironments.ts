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
import { useListEnvironments } from "@agent-management-platform/api-client";
import type { Environment } from "@agent-management-platform/types";

export interface OrgEnvironmentsState {
    /** The organization's environments; empty while loading or if the query failed. */
    environments: Environment[];
    isLoading: boolean;
    isError: boolean;
}

/**
 * The organization's environments, with their loading and error state.
 * Callers should wait for isLoading to be false before rendering the list.
 */
export function useOrgEnvironmentsState(orgId?: string): OrgEnvironmentsState {
    const { data, isLoading, isError } = useListEnvironments({ orgName: orgId });
    return {
        environments: isError ? [] : data ?? [],
        isLoading,
        isError,
    };
}

export function useOrgEnvironments(orgId?: string): Environment[] {
    return useOrgEnvironmentsState(orgId).environments;
}
