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

import { fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@agent-management-platform/api-client", () => ({
  ConsoleAction: {},
  useTrack: () => ({ track: vi.fn() }),
}));

// The real panel is a two-column transfer list; this stub shows the state the
// tab hands it and exposes its callbacks.
vi.mock("@agent-management-platform/shared-component", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  AccessControlPanel: (props: {
    items: { key: string; summary?: string; operationSpec: Record<string, unknown> | null }[];
    mode: string;
    exceptionKeys: string[];
    isDirty: boolean;
    isLoading: boolean;
    readOnly: boolean;
    status: { message: string } | null;
    onModeChange: (m: string) => void;
    onExceptionKeysChange: (k: string[]) => void;
    onSave: () => void;
    onDiscard: () => void;
    onImportSpec: (f: File) => void;
  }) => (
    <div>
      <div>{`items: ${props.items.map((i) => i.key).join(", ")}`}</div>
      <div>{`specs: ${props.items.map((i) => Object.keys((i.operationSpec?.paths as object) ?? {}).join("")).join(", ")}`}</div>
      <div>{`mode: ${props.mode}`}</div>
      <div>{`exceptions: ${props.exceptionKeys.join(", ")}`}</div>
      <div>{props.isDirty ? "dirty" : "clean"}</div>
      <div>{props.isLoading ? "loading" : "loaded"}</div>
      <div>{props.readOnly ? "read only" : "editable"}</div>
      {props.status && <div>{props.status.message}</div>}
      <button onClick={() => props.onModeChange("deny")}>deny all</button>
      <button onClick={() => props.onExceptionKeysChange(["GET::/models", "POST::/chat"])}>
        set exceptions
      </button>
      <button onClick={props.onSave}>save</button>
      <button onClick={props.onDiscard}>discard</button>
      <input
        aria-label="import spec"
        type="file"
        onChange={(e) => props.onImportSpec(e.target.files![0])}
      />
    </div>
  ),
}));

import type { AccessControlMode, LLMProviderResponse } from "@agent-management-platform/types";
import { makeProvider, makeSpec, renderWithProviders } from "../__test__/renderWithProviders";
import { LLMProviderAccessControlTab } from "./LLMProviderAccessControlTab";

const onUpdate = vi.fn();
const spec = makeSpec(["POST /chat", "GET /models", "DELETE /models"]);

const renderTab = (
  providerData: LLMProviderResponse | null | undefined,
  { access = {} as Record<string, boolean>, openapiSpecUrl = undefined as string | undefined } = {},
) =>
  renderWithProviders(
    <LLMProviderAccessControlTab
      providerData={providerData}
      openapiSpecUrl={openapiSpecUrl}
      onUpdate={onUpdate}
      isUpdating={false}
    />,
    { access },
  );

const importFile = (text: string) => {
  const file = new File([text], "spec.json", { type: "application/json" });
  Object.defineProperty(file, "text", { value: () => Promise.resolve(text) });
  fireEvent.change(screen.getByLabelText("import spec"), { target: { files: [file] } });
};

beforeEach(() => {
  onUpdate.mockReset();
  onUpdate.mockResolvedValue(makeProvider());
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("LLMProviderAccessControlTab", () => {
  it("lists the spec's operations and loads the stored deny-all exceptions", () => {
    renderTab(
      makeProvider({
        openapi: spec,
        // Older providers store the mode hyphenated; the tab normalises it.
        accessControl: {
          mode: "deny-all" as AccessControlMode,
          exceptions: [{ path: "/chat", methods: ["POST"] }],
        },
      }),
    );
    expect(screen.getByText("items: POST::/chat, DELETE::/models, GET::/models")).toBeInTheDocument();
    expect(screen.getByText("specs: /chat, /models, /models")).toBeInTheDocument();
    expect(screen.getByText("mode: deny")).toBeInTheDocument();
    expect(screen.getByText("exceptions: POST::/chat")).toBeInTheDocument();
    expect(screen.getByText("clean")).toBeInTheDocument();
    expect(screen.getByText("editable")).toBeInTheDocument();
  });

  it("saves the mode and exceptions grouped by path", async () => {
    renderTab(makeProvider({ openapi: spec, accessControl: { mode: "allow_all", exceptions: [] } }));
    expect(screen.getByText("mode: allow")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "deny all" }));
    fireEvent.click(screen.getByRole("button", { name: "set exceptions" }));
    expect(screen.getByText("dirty")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "save" }));

    await screen.findByText("Access control updated successfully.");
    expect(onUpdate).toHaveBeenCalledWith({
      accessControl: {
        mode: "deny_all",
        exceptions: [
          { path: "/models", methods: ["GET"] },
          { path: "/chat", methods: ["POST"] },
        ],
      },
    });
  });

  it("reports a failed save", async () => {
    onUpdate.mockRejectedValue(new Error("x"));
    renderTab(makeProvider({ openapi: spec }));
    fireEvent.click(screen.getByRole("button", { name: "deny all" }));
    fireEvent.click(screen.getByRole("button", { name: "save" }));
    expect(await screen.findByText("Failed to update access control.")).toBeInTheDocument();
  });

  it("discard restores the saved mode and exceptions", () => {
    renderTab(makeProvider({ openapi: spec }));
    fireEvent.click(screen.getByRole("button", { name: "deny all" }));
    fireEvent.click(screen.getByRole("button", { name: "set exceptions" }));
    fireEvent.click(screen.getByRole("button", { name: "discard" }));
    expect(screen.getByText("mode: allow")).toBeInTheDocument();
    expect(screen.getByText("exceptions:")).toBeInTheDocument();
    expect(screen.getByText("clean")).toBeInTheDocument();
  });

  it("imports a specification as an unsaved change", async () => {
    renderTab(makeProvider());
    expect(screen.getByText("items:")).toBeInTheDocument();
    importFile(makeSpec(["GET /health"]));

    expect(await screen.findByText("Specification imported. Click Save to apply.")).toBeInTheDocument();
    expect(screen.getByText("items: GET::/health")).toBeInTheDocument();
    expect(screen.getByText("dirty")).toBeInTheDocument();
  });

  it("rejects a specification without resources", async () => {
    renderTab(makeProvider());
    importFile(JSON.stringify({ openapi: "3.0.0", paths: {} }));
    expect(await screen.findByText("No resources found in specification.")).toBeInTheDocument();
    expect(screen.getByText("clean")).toBeInTheDocument();
  });

  it("loads the spec from the template URL and reports a failed fetch", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({ ok: false, status: 500, statusText: "Server Error" }),
    );
    renderTab(makeProvider(), { openapiSpecUrl: "https://specs.example.com/broken.json" });
    expect(await screen.findByText("Failed to load OpenAPI spec.")).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText("loaded")).toBeInTheDocument());
  });

  it("is read-only without write access", () => {
    renderTab(makeProvider({ openapi: spec }), { access: { llm_provider_write: false } });
    expect(screen.getByText("read only")).toBeInTheDocument();
  });

  it("renders nothing without a provider", () => {
    const { container } = renderTab(undefined);
    expect(container).toBeEmptyDOMElement();
  });
});
