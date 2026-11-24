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
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { QueryKey } from "@tanstack/react-query";
import { renderHook, waitFor, type RenderedHook } from "../test-utils/render-hook";

const ui = vi.hoisted(() => ({
  pushSnackBar: vi.fn(),
  logout: vi.fn(),
  isAuthenticated: true,
  track: vi.fn(),
  markSessionExpired: vi.fn(),
}));

vi.mock("@agent-management-platform/views", () => ({
  useSnackBar: () => ({ pushSnackBar: ui.pushSnackBar }),
}));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ isAuthenticated: ui.isAuthenticated, logout: ui.logout }),
}));
vi.mock("../acp-core/hooks/telemetry", () => ({
  ConsoleAction: { ErrorShown: "amp.console.friction.error-shown" },
  markSessionExpired: ui.markSessionExpired,
  useTrack: () => ({ track: ui.track }),
}));

import {
  extractServerErrorMessage,
  MAX_SNACKBAR_REASON_LENGTH,
  useApiMutation,
  useApiQuery,
} from "./react-query-notifications";
import { REQUEST_TOO_LARGE_CODE } from "./utils";

const httpError = (status: number, extra: Record<string, unknown> = {}) =>
  Object.assign(new Error(`HTTP error! status: ${status}`), { status, ...extra });

let rendered: RenderedHook<unknown> | undefined;

beforeEach(() => {
  ui.isAuthenticated = true;
});

afterEach(() => {
  rendered?.unmount();
  rendered = undefined;
  vi.resetAllMocks();
});

async function failingQuery(
  error: unknown,
  queryKey: QueryKey = ["agents"],
  extra: { silent?: boolean } = {},
) {
  const hook = await renderHook(() =>
    useApiQuery({
      queryKey,
      queryFn: () => Promise.reject(error),
      ...extra,
    }),
  );
  rendered = hook;
  await waitFor(() => hook.result.current.isError);
  return hook;
}

describe("useApiQuery", () => {
  it("returns the data without any notification", async () => {
    const hook = await renderHook(() =>
      useApiQuery({ queryKey: ["agents"], queryFn: async () => ["a1"] }),
    );
    rendered = hook;
    await waitFor(() => hook.result.current.isSuccess);
    expect(hook.result.current.data).toEqual(["a1"]);
    expect(ui.pushSnackBar).not.toHaveBeenCalled();
  });

  it.each([
    [["agents"], "Failed to fetch agents"],
    [["agent", { id: 1 }], "Failed to fetch agent"],
    [["projects"], "Failed to fetch projects"],
    [["project"], "Failed to fetch project"],
    [["environments"], "Failed to fetch environments"],
    [["environment"], "Failed to fetch environment"],
    [["llm-providers", "x"], "Failed to fetch Llm Providers"],
    [[{ scope: "x" }], "Failed to fetch data"],
  ])("names the resource in a failed fetch of %j", async (queryKey, message) => {
    await failingQuery(httpError(500), queryKey as QueryKey);
    expect(ui.pushSnackBar).toHaveBeenCalledWith({ message, type: "error" });
  });

  it("shows one snackbar for an error however often the page re-renders", async () => {
    const hook = await failingQuery(httpError(500));
    await hook.rerender();
    await hook.rerender();
    expect(ui.pushSnackBar).toHaveBeenCalledTimes(1);
  });

  it("does not show a snackbar for a 404, which callers treat as absent", async () => {
    await failingQuery(httpError(404));
    expect(ui.pushSnackBar).not.toHaveBeenCalled();
  });

  it("keeps a silent query quiet", async () => {
    await failingQuery(httpError(500), ["agents"], { silent: true });
    expect(ui.pushSnackBar).not.toHaveBeenCalled();
  });

  it("parks a 401 as a session expiry instead of showing an error", async () => {
    window.history.pushState({}, "", "/org/acme/agents");
    await failingQuery(httpError(401), ["agents"], { silent: true });
    expect(ui.pushSnackBar).not.toHaveBeenCalled();
    expect(ui.markSessionExpired).toHaveBeenCalledWith("/org/acme/agents");
    expect(ui.logout).not.toHaveBeenCalled();
  });

  it("reads a 401 from a nested response too", async () => {
    await failingQuery({ response: { status: 401 } });
    expect(ui.markSessionExpired).toHaveBeenCalledTimes(1);
    expect(ui.pushSnackBar).not.toHaveBeenCalled();
  });

  it("logs out once when the token is no longer valid", async () => {
    const hook = await failingQuery({ code: "SPA-AUTH_CLIENT-VM-IV02" });
    await hook.rerender();
    expect(ui.logout).toHaveBeenCalledTimes(1);
    expect(ui.markSessionExpired).toHaveBeenCalledTimes(1);
    expect(ui.pushSnackBar).not.toHaveBeenCalled();
  });

  it("ignores the auth client's not-found error", async () => {
    await failingQuery({ code: "SPA-AUTH_CLIENT-VM-NF01" });
    expect(ui.logout).not.toHaveBeenCalled();
    expect(ui.markSessionExpired).not.toHaveBeenCalled();
    expect(ui.pushSnackBar).not.toHaveBeenCalled();
  });

  it("does nothing before the session is initialised", async () => {
    ui.isAuthenticated = false;
    await failingQuery({ code: "SPA-AUTH_CLIENT-VM-IV02" });
    expect(ui.logout).not.toHaveBeenCalled();
    expect(ui.pushSnackBar).not.toHaveBeenCalled();
  });
});

type MutationHook = ReturnType<typeof useApiMutation<unknown, unknown, { name: string }>>;

async function mutation(
  options: Parameters<typeof useApiMutation<unknown, unknown, { name: string }>>[0],
): Promise<RenderedHook<MutationHook>> {
  const hook = await renderHook(() => useApiMutation(options));
  rendered = hook as RenderedHook<unknown>;
  return hook;
}

async function run(hook: RenderedHook<MutationHook>) {
  await act(async () => {
    await hook.result.current.mutateAsync({ name: "n1" }).catch(() => undefined);
  });
}

describe("useApiMutation success", () => {
  it("announces the action and still calls the caller's onSuccess", async () => {
    const onSuccess = vi.fn();
    const hook = await mutation({
      action: { verb: "deploy", target: "agent" },
      mutationFn: async () => "ok",
      onSuccess,
    });
    await run(hook);
    expect(ui.pushSnackBar).toHaveBeenCalledWith({
      message: "Agent deployed successfully",
      type: "success",
    });
    expect(onSuccess.mock.calls[0].slice(0, 2)).toEqual(["ok", { name: "n1" }]);
  });

  it("title-cases a multi-word target", async () => {
    const hook = await mutation({
      action: { verb: "rerun", target: "agent identity-provisioning" },
      mutationFn: async () => "ok",
    });
    await run(hook);
    expect(ui.pushSnackBar.mock.calls[0][0].message).toBe(
      "Agent Identity Provisioning triggered successfully",
    );
  });

  it("prefers a caller's success message, static or computed", async () => {
    const hook = await mutation({
      action: { verb: "create", target: "agent" },
      successMessage: (data, vars) => `${vars.name} is ${String(data)}`,
      mutationFn: async () => "live",
    });
    await run(hook);
    expect(ui.pushSnackBar.mock.calls[0][0].message).toBe("n1 is live");

    ui.pushSnackBar.mockClear();
    const plain = await mutation({ successMessage: "Saved", mutationFn: async () => "x" });
    await run(plain);
    expect(ui.pushSnackBar).toHaveBeenCalledWith({ message: "Saved", type: "success" });
  });

  it("stays quiet without an action or message, and uses a generic one when asked", async () => {
    const quiet = await mutation({ mutationFn: async () => "x" });
    await run(quiet);
    expect(ui.pushSnackBar).not.toHaveBeenCalled();

    const generic = await mutation({ showSuccess: true, mutationFn: async () => "x" });
    await run(generic);
    expect(ui.pushSnackBar).toHaveBeenCalledWith({
      message: "Request completed successfully",
      type: "success",
    });
  });

  it("shows nothing to a signed-out user", async () => {
    ui.isAuthenticated = false;
    const onSuccess = vi.fn();
    const hook = await mutation({
      action: { verb: "create", target: "agent" },
      mutationFn: async () => "x",
      onSuccess,
    });
    await run(hook);
    expect(ui.pushSnackBar).not.toHaveBeenCalled();
    expect(onSuccess).toHaveBeenCalled();
  });
});

describe("useApiMutation errors", () => {
  async function failWith(
    error: unknown,
    extra: Partial<Parameters<typeof useApiMutation<unknown, unknown, { name: string }>>[0]> = {},
  ) {
    const onError = vi.fn();
    const hook = await mutation({
      action: { verb: "create", target: "agent" },
      mutationFn: () => Promise.reject(error),
      onError,
      ...extra,
    });
    await run(hook);
    expect(onError).toHaveBeenCalled();
    return ui.pushSnackBar.mock.calls.map(([n]) => n.message);
  }

  it("shows the server's message with a short reason and reports the error", async () => {
    const error = httpError(409, { body: { message: "Name taken", reason: "duplicate" } });
    error.message = "Name taken";
    expect(await failWith(error)).toEqual(["Name taken: duplicate"]);
    expect(ui.track).toHaveBeenCalledWith("amp.console.friction.error-shown", {
      operation: "create:agent",
      status: 409,
    });
  });

  it("drops a reason too long for the snackbar", async () => {
    const reason = "x".repeat(MAX_SNACKBAR_REASON_LENGTH + 1);
    const error = Object.assign(new Error("Name taken"), { body: { reason } });
    expect(await failWith(error)).toEqual(["Name taken"]);
  });

  it("falls back to a generic message rather than a transport status", async () => {
    expect(await failWith(httpError(500))).toEqual(["Failed to submit agent"]);
  });

  it("uses the server body's message when the error message is a transport status", async () => {
    expect(await failWith(httpError(500, { body: { message: "Gateway down" } }))).toEqual([
      "Gateway down",
    ]);
  });

  it("prefers the caller's error message over the server's", async () => {
    const error = Object.assign(new Error("raw"), { status: 503 });
    expect(await failWith(error, { errorMessage: (_e, v) => `Could not save ${v.name}` }))
      .toEqual(["Could not save n1"]);
  });

  it("names the size limit ahead of any caller message", async () => {
    const error = Object.assign(new Error("Request is too large (2 KB of 1 KB)."), {
      status: 413,
      code: REQUEST_TOO_LARGE_CODE,
    });
    expect(await failWith(error, { errorMessage: "Generic" })).toEqual([
      "Request is too large (2 KB of 1 KB).",
    ]);
  });

  it("reports an unknown operation and status for a bare failure", async () => {
    const hook = await mutation({ mutationFn: () => Promise.reject("boom") });
    await run(hook);
    expect(ui.pushSnackBar).toHaveBeenCalledWith({ message: "Failed to submit data", type: "error" });
    expect(ui.track).toHaveBeenCalledWith("amp.console.friction.error-shown", {
      operation: "unknown",
      status: 0,
    });
  });

  it("stays quiet for an abort, for showError false, for 401 and when signed out", async () => {
    expect(await failWith(Object.assign(new Error("a"), { name: "AbortError" }))).toEqual([]);
    expect(await failWith(httpError(500), { showError: false })).toEqual([]);
    expect(await failWith(httpError(401))).toEqual([]);
    expect(ui.markSessionExpired).toHaveBeenCalledTimes(1);
    ui.isAuthenticated = false;
    expect(await failWith(httpError(500))).toEqual([]);
    expect(ui.track).not.toHaveBeenCalled();
  });
});

describe("extractServerErrorMessage", () => {
  it("returns nothing for values that are not errors", () => {
    expect(extractServerErrorMessage(undefined)).toBeUndefined();
    expect(extractServerErrorMessage("text")).toBeUndefined();
    expect(extractServerErrorMessage({ message: "   " })).toBeUndefined();
  });

  it("keeps a long reason when the caller has room for it", () => {
    const reason = "x".repeat(200);
    expect(extractServerErrorMessage({ message: "Blocked", body: { reason } })).toBe(
      `Blocked: ${reason}`,
    );
  });
});
