/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useOrgEnvironments, useOrgEnvironmentsState } from "./useOrgEnvironments";

const useListEnvironments = vi.fn();
vi.mock("@agent-management-platform/api-client", () => ({
  useListEnvironments: (...args: unknown[]) => useListEnvironments(...args),
}));

const envs = [{ name: "dev" }, { name: "prod" }];

beforeEach(() => useListEnvironments.mockReset());

describe("useOrgEnvironmentsState", () => {
  it("queries the org's environments and returns them", () => {
    useListEnvironments.mockReturnValue({ data: envs, isLoading: false, isError: false });
    const { result } = renderHook(() => useOrgEnvironmentsState("acme"));
    expect(useListEnvironments).toHaveBeenCalledWith({ orgName: "acme" });
    expect(result.current).toEqual({ environments: envs, isLoading: false, isError: false });
  });

  it("returns an empty list while loading", () => {
    useListEnvironments.mockReturnValue({ data: undefined, isLoading: true, isError: false });
    const { result } = renderHook(() => useOrgEnvironmentsState("acme"));
    expect(result.current).toEqual({ environments: [], isLoading: true, isError: false });
  });

  it("returns an empty list on error even with stale data", () => {
    useListEnvironments.mockReturnValue({ data: envs, isLoading: false, isError: true });
    const { result } = renderHook(() => useOrgEnvironments("acme"));
    expect(result.current).toEqual([]);
  });
});
