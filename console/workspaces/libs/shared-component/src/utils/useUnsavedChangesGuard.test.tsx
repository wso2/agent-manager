/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
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

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { BrowserRouter, useLocation, useNavigate } from "react-router-dom";
import type { ConfirmationEvent } from "../components/ConfirmationDialog/ConfirmationDialogProvider";

// Capture the dialog instead of rendering it, so tests can press Stay/Leave.
const confirmations: ConfirmationEvent[] = [];
vi.mock("../components/ConfirmationDialog/ConfirmationDialogProvider", () => ({
  useConfirmationDialog: () => ({
    addConfirmation: (event: ConfirmationEvent) => confirmations.push(event),
  }),
}));

// Imported before any BrowserRouter mounts, as in the app, so the module's
// popstate listener runs ahead of the router's.
const { useUnsavedChangesGuard, useConfirmIfUnsaved } = await import(
  "./useUnsavedChangesGuard"
);

type Api = {
  navigate: ReturnType<typeof useNavigate>;
  allowNavigation: (action: () => void) => void;
  confirmIfUnsaved: ReturnType<typeof useConfirmIfUnsaved>;
};
const api = {} as Api;

function Harness({ dirty }: { dirty: boolean }) {
  const navigate = useNavigate();
  const location = useLocation();
  const { allowNavigation } = useUnsavedChangesGuard(dirty);
  const confirmIfUnsaved = useConfirmIfUnsaved();
  Object.assign(api, { navigate, allowNavigation, confirmIfUnsaved });
  return <div data-testid="path">{location.pathname + location.search + location.hash}</div>;
}

const renderAt = (path: string, dirty: boolean) => {
  window.history.replaceState(null, "", path);
  return render(
    <BrowserRouter>
      <Harness dirty={dirty} />
    </BrowserRouter>,
  );
};
const currentPath = () => screen.getByTestId("path").textContent;
const lastDialog = () => confirmations[confirmations.length - 1];

beforeEach(() => {
  confirmations.length = 0;
});

afterEach(() => {
  // Resolve any open dialog so the module-level dialogOpen flag resets.
  confirmations.forEach((c) => c.onCancel?.());
  cleanup();
});

describe("useUnsavedChangesGuard", () => {
  it("lets navigation through when the form is clean", () => {
    renderAt("/form", false);
    act(() => api.navigate("/other"));
    expect(currentPath()).toBe("/other");
    expect(confirmations).toHaveLength(0);
  });

  it("blocks navigation while dirty and asks Stay / Leave", () => {
    renderAt("/form", true);
    act(() => api.navigate("/other"));
    expect(currentPath()).toBe("/form");
    expect(window.location.pathname).toBe("/form");
    expect(lastDialog()).toMatchObject({
      title: "You have unsaved changes",
      confirmButtonText: "Leave",
      cancelButtonText: "Stay",
    });
  });

  it("Leave replays the blocked navigation", () => {
    renderAt("/form", true);
    act(() => api.navigate("/other?tab=a"));
    act(() => lastDialog().onConfirm());
    expect(currentPath()).toBe("/other?tab=a");
  });

  it("Stay keeps the page and asks again next time", () => {
    renderAt("/form", true);
    act(() => api.navigate("/other"));
    act(() => lastDialog().onCancel?.());
    expect(currentPath()).toBe("/form");
    act(() => api.navigate("/other"));
    expect(confirmations).toHaveLength(2);
  });

  it("guards query-string changes but not hash-only ones", () => {
    renderAt("/form", true);
    act(() => api.navigate("/form#section"));
    expect(currentPath()).toBe("/form#section");
    expect(confirmations).toHaveLength(0);
    act(() => api.navigate("/form?tab=b", { replace: true }));
    expect(currentPath()).toBe("/form#section");
    expect(confirmations).toHaveLength(1);
  });

  it("Leave replays the navigation under a base path without doubling it", () => {
    const config = (window as unknown as { __RUNTIME_CONFIG__: { basePath?: string } })
      .__RUNTIME_CONFIG__;
    config.basePath = "/console";
    try {
      window.history.replaceState(null, "", "/console/form");
      render(
        <BrowserRouter basename="/console">
          <Harness dirty />
        </BrowserRouter>,
      );
      act(() => api.navigate("/other"));
      expect(window.location.pathname).toBe("/console/form");
      act(() => lastDialog().onConfirm());
      expect(currentPath()).toBe("/other");
      expect(window.location.pathname).toBe("/console/other");
    } finally {
      config.basePath = undefined;
    }
  });

  it("allowNavigation bypasses the guard for deliberate exits", () => {
    renderAt("/form", true);
    act(() => api.allowNavigation(() => api.navigate("/saved")));
    expect(currentPath()).toBe("/saved");
    expect(confirmations).toHaveLength(0);
  });

  it("restores the original History API once nothing is dirty", () => {
    const originalPush = window.history.pushState;
    const { rerender } = renderAt("/form", true);
    expect(window.history.pushState).not.toBe(originalPush);
    rerender(
      <BrowserRouter>
        <Harness dirty={false} />
      </BrowserRouter>,
    );
    expect(window.history.pushState).toBe(originalPush);
  });

  it("guards browser Back and replays it on Leave", async () => {
    renderAt("/list", false);
    act(() => api.navigate("/form"));
    cleanup();
    render(
      <BrowserRouter>
        <Harness dirty />
      </BrowserRouter>,
    );
    expect(currentPath()).toBe("/form");

    act(() => window.history.back());
    await waitFor(() => expect(confirmations).toHaveLength(1));
    await waitFor(() => expect(window.location.pathname).toBe("/form"));
    expect(currentPath()).toBe("/form");

    act(() => lastDialog().onConfirm());
    await waitFor(() => expect(currentPath()).toBe("/list"));
  });
});

describe("useConfirmIfUnsaved", () => {
  it("runs the action straight away when the caller's form is clean", () => {
    renderAt("/form", true);
    const action = vi.fn();
    act(() => api.confirmIfUnsaved(action, false));
    expect(action).toHaveBeenCalledOnce();
    expect(confirmations).toHaveLength(0);
  });

  it("asks first when dirty and runs the action only on Leave", () => {
    renderAt("/form", false);
    const action = vi.fn();
    act(() => api.confirmIfUnsaved(action, true));
    expect(action).not.toHaveBeenCalled();
    act(() => lastDialog().onConfirm());
    expect(action).toHaveBeenCalledOnce();
  });

  it("falls back to any active guard when isDirty is omitted", () => {
    renderAt("/form", false);
    const action = vi.fn();
    act(() => api.confirmIfUnsaved(action));
    expect(action).toHaveBeenCalledOnce();
  });
});
