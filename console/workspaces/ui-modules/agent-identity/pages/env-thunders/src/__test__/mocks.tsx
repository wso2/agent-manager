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

// Shared test doubles for this package. Each test file wires them in with
//   vi.mock("@agent-management-platform/api-client", async () =>
//     (await import("../__test__/mocks")).apiClientMock);
// so the test and the mocked module read and write the same state object.

import type { ReactNode } from "react";
import { render, type RenderResult } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { vi, type Mock } from "vitest";

type QueryResult = {
  data?: unknown;
  isLoading?: boolean;
  isError?: boolean;
  error?: unknown;
  refetch?: Mock;
};

const QUERY_HOOKS = [
  "useListEnvironments",
  "useListAgentIdentityAgents",
  "useListAgentIdentityGroups",
  "useListAgentIdentityRoles",
  "useListAgentIdentityScopes",
  "useGetAgentIdentityGroup",
  "useGetAgentIdentityGroupMembers",
  "useGetAgentIdentityGroupRoles",
  "useGetAgentIdentityRole",
  "useGetAgentIdentityRoleAssignments",
  "useGetAgent",
  "useGetAgentGroups",
  "useGetAgentRoles",
  "useGetProject",
] as const;

const MUTATION_HOOKS = [
  "useDeleteAgentIdentityGroup",
  "useDeleteAgentIdentityRole",
  "useCreateAgentIdentityGroup",
  "useCreateAgentIdentityRole",
  "useUpdateAgentIdentityRole",
  "useAddAgentIdentityGroupMembers",
  "useRemoveAgentIdentityGroupMembers",
  "useAddAgentIdentityRoleAssignees",
  "useRemoveAgentIdentityRoleAssignees",
  "useProvisionAgentIdentity",
  "useRetryAgentIdentityProvisioning",
] as const;

export type QueryHook = (typeof QUERY_HOOKS)[number];
export type MutationHook = (typeof MUTATION_HOOKS)[number];

type MutationState = { fn: Mock; isPending: boolean; isError: boolean; error: unknown };

export const apiState = {
  queries: {} as Partial<Record<QueryHook, QueryResult>>,
  mutations: {} as Record<MutationHook, MutationState>,
  /** Calls recorded per query hook, so tests can check the params a page asked for. */
  calls: {} as Partial<Record<QueryHook | "useAgentIdentityBinding", unknown[][]>>,
  /** What useAgentIdentityBinding returns. */
  binding: {} as Record<string, unknown>,
};

// Display names the console resolves for agents ("<raw> label"), so tests can
// tell a resolved name from a raw one. Stable references, like the real hook.
const displayResolver = {
  resolveAgentName: (_projectName: string, agentName: string) => `${agentName} label`,
  resolveProjectName: (projectName: string) => `${projectName} project`,
};

export function resetApiState() {
  apiState.queries = {};
  apiState.calls = {};
  apiState.binding = { binding: undefined, provisioned: false, isLoading: false, isError: false };
  apiState.mutations = Object.fromEntries(
    MUTATION_HOOKS.map((name) => [
      name,
      { fn: vi.fn().mockResolvedValue({}), isPending: false, isError: false, error: null },
    ]),
  ) as Record<MutationHook, MutationState>;
}
resetApiState();

/** The mutateAsync/mutate spy behind a mutation hook. */
export const mutation = (name: MutationHook) => apiState.mutations[name].fn;

const queryHook =
  (name: QueryHook) =>
  (...args: unknown[]) => {
    (apiState.calls[name] ??= []).push(args);
    const result = apiState.queries[name] ?? {};
    return {
      data: undefined,
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
      ...result,
    };
  };

const mutationHook = (name: MutationHook) => () => {
  const state = apiState.mutations[name];
  return {
    mutateAsync: state.fn,
    mutate: state.fn,
    isPending: state.isPending,
    isError: state.isError,
    error: state.error,
  };
};

export const apiClientMock = {
  ...Object.fromEntries(QUERY_HOOKS.map((name) => [name, queryHook(name)])),
  ...Object.fromEntries(MUTATION_HOOKS.map((name) => [name, mutationHook(name)])),
  useOrgAgentDisplayNames: () => displayResolver,
  useAgentIdentityBinding: (...args: unknown[]) => {
    (apiState.calls.useAgentIdentityBinding ??= []).push(args);
    return apiState.binding;
  },
};

export const accessState = {
  canWrite: true,
  flags: {} as Record<string, boolean>,
  access: {} as Record<string, boolean>,
};

export function resetAccessState() {
  accessState.canWrite = true;
  accessState.flags = {};
  accessState.access = {};
}

export const featureManagementMock = {
  useResourceAccess: () => ({ canRead: true, canWrite: accessState.canWrite }),
  useFeatureFlag: (key: string) => accessState.flags[key] ?? true,
  useFeatureAccess: (key: string) => accessState.access[key] ?? true,
};

export const sharedState = {
  addConfirmation: vi.fn(),
  guardDirty: [] as boolean[],
};

/**
 * Overrides for the shared-component hooks that need app-level providers:
 * the confirmation dialog provider and the History-API unsaved-changes guard.
 */
export const sharedComponentOverrides = {
  useConfirmationDialog: () => ({ addConfirmation: sharedState.addConfirmation }),
  useUnsavedChangesGuard: (isDirty: boolean) => {
    sharedState.guardDirty.push(isDirty);
    return { allowNavigation: (action: () => void) => action() };
  },
};

/** The last element of a list (ES2020 lib has no Array.prototype.at). */
export const last = <T,>(items: T[] | undefined): T | undefined =>
  items ? items[items.length - 1] : undefined;

export const ROUTER_FUTURE = { v7_startTransition: true, v7_relativeSplatPath: true } as const;

/** Prints where the router ended up, for asserting navigation. */
export function Destination() {
  const location = useLocation();
  return <div>{`at ${location.pathname}${location.search}`}</div>;
}

/** Renders `element` on `routePath`, starting at `url`, with a catch-all destination. */
export function renderAt(element: ReactNode, routePath: string, url: string): RenderResult {
  return render(
    <MemoryRouter initialEntries={[url]} future={ROUTER_FUTURE}>
      <Routes>
        <Route path={routePath} element={element} />
        <Route path="*" element={<Destination />} />
      </Routes>
    </MemoryRouter>,
  );
}
