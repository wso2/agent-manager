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

import { renderHook, act } from "@testing-library/react";
import { useDirtyState } from "./useDirtyState";

describe("useDirtyState", () => {
  it("starts clean", () => {
    const { result } = renderHook(() => useDirtyState({ name: "a" }));

    expect(result.current.isDirty).toBe(false);
  });

  it("becomes dirty once the current data differs from the initial snapshot", () => {
    const { result } = renderHook(() => useDirtyState({ name: "a" }));

    act(() => result.current.checkDirty({ name: "b" }));

    expect(result.current.isDirty).toBe(true);
  });

  it("goes back to clean once the current data matches the initial snapshot again", () => {
    const { result } = renderHook(() => useDirtyState({ name: "a" }));

    act(() => result.current.checkDirty({ name: "b" }));
    act(() => result.current.checkDirty({ name: "a" }));

    expect(result.current.isDirty).toBe(false);
  });

  it("resetDirty clears the dirty flag and rebases the snapshot onto the new data", () => {
    const { result } = renderHook(() => useDirtyState({ name: "a" }));

    act(() => result.current.checkDirty({ name: "b" }));
    act(() => result.current.resetDirty({ name: "b" }));

    expect(result.current.isDirty).toBe(false);

    act(() => result.current.checkDirty({ name: "a" }));
    expect(result.current.isDirty).toBe(true);
  });
});
