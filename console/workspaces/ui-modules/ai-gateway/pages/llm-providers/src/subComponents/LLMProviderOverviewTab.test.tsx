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

const hoisted = vi.hoisted(() => ({
  deployments: { data: undefined as unknown, isLoading: false },
  gateways: { data: undefined as unknown, isLoading: false },
  createKey: { mutateAsync: vi.fn(), isPending: false },
  rotateKey: { mutateAsync: vi.fn(), isPending: false },
}));

vi.mock("@agent-management-platform/api-client", () => ({
  ConsoleAction: {},
  useTrack: () => ({ track: vi.fn() }),
  useListLLMDeployments: () => hoisted.deployments,
  useListGateways: () => hoisted.gateways,
  useCreateLLMProviderAPIKey: () => hoisted.createKey,
  useRotateLLMProviderAPIKey: () => hoisted.rotateKey,
}));

// The real viewer lazy-loads swagger-ui; show what it was asked to render instead.
vi.mock("@agent-management-platform/shared-component", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  SwaggerSpecViewer: (props: { url?: string; spec?: unknown }) => (
    <div>{props.url ? `swagger url ${props.url}` : `swagger spec ${JSON.stringify(props.spec)}`}</div>
  ),
}));

import type { LLMProviderResponse } from "@agent-management-platform/types";
import { makeProvider, renderWithProviders } from "../__test__/renderWithProviders";
import { LLMProviderOverviewTab, type LLMProviderOverviewTabProps } from "./LLMProviderOverviewTab";

const onUpdate = vi.fn();
const onGoToDeploymentTab = vi.fn();

const gateway = (uuid: string, envName: string | null, vhost = `${uuid}.example.com`) => ({
  uuid,
  name: uuid,
  displayName: `Gateway ${uuid}`,
  vhost,
  environments: envName ? [{ id: `env-${envName}`, name: envName, displayName: envName }] : [],
});

const renderTab = (
  providerData: LLMProviderResponse | null | undefined,
  props: Partial<LLMProviderOverviewTabProps> = {},
  access: Record<string, boolean> = {},
) =>
  renderWithProviders(
    <LLMProviderOverviewTab
      providerData={providerData}
      openapiSpecUrl={undefined}
      orgName="acme"
      providerId="p-1"
      onUpdate={onUpdate}
      isUpdating={false}
      onGoToDeploymentTab={onGoToDeploymentTab}
      {...props}
    />,
    { access },
  );

beforeEach(() => {
  vi.clearAllMocks();
  onUpdate.mockResolvedValue(makeProvider());
  hoisted.deployments = { data: [], isLoading: false };
  hoisted.gateways = { data: { gateways: [] }, isLoading: false };
  hoisted.createKey.isPending = false;
  hoisted.rotateKey.isPending = false;
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("LLMProviderOverviewTab", () => {
  it("summarises the provider's connection details", () => {
    renderTab(
      makeProvider({ accessControl: { mode: "deny_all", exceptions: [] }, inCatalog: true }),
    );
    expect(screen.getByText("/openai")).toBeInTheDocument();
    expect(screen.getByText("https://api.openai.com/v1")).toBeInTheDocument();
    expect(screen.getByText("api-key")).toBeInTheDocument();
    expect(screen.getByText("deny_all")).toBeInTheDocument();
    expect(screen.getByText("Yes")).toBeInTheDocument();
  });

  it("offers a shortcut to the Deployment tab when the provider is not deployed", () => {
    renderTab(makeProvider({ gateways: [] }));
    expect(screen.getByText("Not deployed")).toBeInTheDocument();
    expect(screen.getByText(/No invoke URLs available/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Deploy to a gateway" }));
    expect(onGoToDeploymentTab).toHaveBeenCalled();
  });

  it("shows environment chips with deployment status and an invoke URL for deployed gateways", () => {
    hoisted.gateways = {
      data: {
        gateways: [
          gateway("gw-1", "Production", "https://prod.example.com/"),
          gateway("gw-2", "Staging"),
          gateway("gw-3", null),
        ],
      },
      isLoading: false,
    };
    hoisted.deployments = {
      data: [
        { gatewayId: "gw-1", status: "DEPLOYED" },
        { gatewayId: "gw-2", status: "FAILED", statusReason: "bad config" },
      ],
      isLoading: false,
    };
    renderTab(makeProvider({ gateways: ["gw-1", "gw-2", "gw-3", "gw-gone"] }));

    expect(screen.getByText("Production")).toBeInTheDocument();
    expect(screen.getByText("Staging · Failed")).toBeInTheDocument();
    // gw-3 has no environment and gw-gone is not in the gateway list.
    expect(screen.getAllByText("Unmapped")).toHaveLength(2);
    expect(screen.getByDisplayValue("https://prod.example.com/openai")).toBeInTheDocument();
    expect(screen.queryByDisplayValue(/gw-2/)).not.toBeInTheDocument();
  });

  it("shows a placeholder for the deployment card while gateways load", () => {
    hoisted.gateways = { data: undefined, isLoading: true };
    renderTab(makeProvider({ gateways: ["gw-1"] }));
    expect(screen.queryByText("Unmapped")).not.toBeInTheDocument();
    expect(screen.queryByText("Not deployed")).not.toBeInTheDocument();
  });

  describe("API key generation", () => {
    beforeEach(() => {
      hoisted.gateways = { data: { gateways: [gateway("gw-1", "Production")] }, isLoading: false };
      hoisted.deployments = { data: [{ gatewayId: "gw-1", status: "DEPLOYED" }], isLoading: false };
    });

    it("creates a key named after the provider and shows it once", async () => {
      hoisted.createKey.mutateAsync.mockResolvedValue({ apiKey: "amp-key-123" });
      renderTab(makeProvider({ gateways: ["gw-1"] }));
      fireEvent.click(screen.getByRole("button", { name: "Generate API key" }));

      expect(await screen.findByDisplayValue("amp-key-123")).toBeInTheDocument();
      const call = hoisted.createKey.mutateAsync.mock.calls[0][0];
      expect(call.params).toEqual({ orgName: "acme", providerId: "p-1" });
      expect(call.body.name).toMatch(/^provider-openai-prod-[a-z0-9]+$/);
      expect(call.body.displayName).toBe(call.body.name);
    });

    it("rotates the key when one already exists", async () => {
      hoisted.createKey.mutateAsync.mockRejectedValue({ status: 409 });
      hoisted.rotateKey.mutateAsync.mockResolvedValue({ apiKey: "rotated-key" });
      renderTab(makeProvider({ gateways: ["gw-1"] }));
      fireEvent.click(screen.getByRole("button", { name: "Generate API key" }));

      expect(await screen.findByDisplayValue("rotated-key")).toBeInTheDocument();
      const call = hoisted.rotateKey.mutateAsync.mock.calls[0][0];
      expect(call.params.keyName).toBe(hoisted.createKey.mutateAsync.mock.calls[0][0].body.name);
      expect(call.body).toEqual({});
    });

    it("reports a failed rotation", async () => {
      hoisted.createKey.mutateAsync.mockRejectedValue(new Error("key already exists"));
      hoisted.rotateKey.mutateAsync.mockRejectedValue(new Error("rotate broke"));
      renderTab(makeProvider({ gateways: ["gw-1"] }));
      fireEvent.click(screen.getByRole("button", { name: "Generate API key" }));
      expect(await screen.findByText("rotate broke")).toBeInTheDocument();
    });

    it("reports a failed create that is not a conflict", async () => {
      hoisted.createKey.mutateAsync.mockRejectedValue(new Error("server down"));
      renderTab(makeProvider({ gateways: ["gw-1"] }));
      fireEvent.click(screen.getByRole("button", { name: "Generate API key" }));
      expect(await screen.findByText("server down")).toBeInTheDocument();
      expect(hoisted.rotateKey.mutateAsync).not.toHaveBeenCalled();
    });

    it("hides key generation from read-only users", () => {
      renderTab(makeProvider({ gateways: ["gw-1"] }), {}, { llm_provider_write: false });
      expect(screen.getByDisplayValue("https://gw-1.example.com/openai")).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Generate API key" })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
    });
  });

  describe("OpenAPI resources", () => {
    it("previews the template's spec URL and saves an edited one", async () => {
      renderTab(makeProvider(), { openapiSpecUrl: "https://specs.example.com/openai.yaml" });
      const input = screen.getByPlaceholderText("https://example.com/openapi.json");
      expect(input).toHaveValue("https://specs.example.com/openai.yaml");
      expect(screen.getByText("swagger url https://specs.example.com/openai.yaml")).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();

      fireEvent.change(input, { target: { value: "https://specs.example.com/v2.json" } });
      fireEvent.click(screen.getByRole("button", { name: "Save" }));
      await waitFor(() =>
        expect(onUpdate).toHaveBeenCalledWith({ openapi: "https://specs.example.com/v2.json" }),
      );
    });

    it("previews an inline spec and asks for one when empty", () => {
      renderTab(makeProvider({ openapi: '{"openapi":"3.0.0","paths":{}}' }));
      expect(screen.getByText(/swagger spec .*"openapi":"3.0.0"/)).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Download" })).toBeDisabled();

      fireEvent.change(screen.getByPlaceholderText("https://example.com/openapi.json"), {
        target: { value: "" },
      });
      expect(
        screen.getByText("Enter a URL or paste an OpenAPI spec above to preview it here."),
      ).toBeInTheDocument();
    });

    it("shows a save error", async () => {
      onUpdate.mockRejectedValue(new Error("save failed"));
      renderTab(makeProvider());
      fireEvent.change(screen.getByPlaceholderText("https://example.com/openapi.json"), {
        target: { value: "https://x.example.com/spec.json" },
      });
      fireEvent.click(screen.getByRole("button", { name: "Save" }));
      expect(await screen.findByText("save failed")).toBeInTheDocument();
    });

    it("downloads the spec from its URL", async () => {
      const fetchMock = vi.fn().mockResolvedValue({ ok: true, text: () => Promise.resolve("{}") });
      vi.stubGlobal("fetch", fetchMock);
      const createObjectURL = vi.fn(() => "blob:spec");
      const revokeObjectURL = vi.fn();
      Object.assign(URL, { createObjectURL, revokeObjectURL });
      const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});

      renderTab(makeProvider(), { openapiSpecUrl: "https://specs.example.com/openai.json" });
      fireEvent.click(screen.getByRole("button", { name: "Download" }));

      await waitFor(() => expect(revokeObjectURL).toHaveBeenCalledWith("blob:spec"));
      expect(fetchMock).toHaveBeenCalledWith("https://specs.example.com/openai.json");
      expect(click).toHaveBeenCalled();
      click.mockRestore();
    });

    it("reports a failed download", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({ ok: false, status: 404, statusText: "Not Found" }),
      );
      renderTab(makeProvider(), { openapiSpecUrl: "https://specs.example.com/openai.yaml" });
      fireEvent.click(screen.getByRole("button", { name: "Download" }));
      expect(
        await screen.findByText("Failed to download spec: 404 Not Found"),
      ).toBeInTheDocument();
    });
  });

  it("shows the load error, and nothing at all without a provider", () => {
    const { container, rerenderWith } = renderTab(undefined, { error: new Error("not found") });
    expect(screen.getByText("not found")).toBeInTheDocument();
    rerenderWith(
      <LLMProviderOverviewTab
        providerData={undefined}
        openapiSpecUrl={undefined}
        orgName="acme"
        providerId="p-1"
        onUpdate={onUpdate}
        isUpdating={false}
      />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("shows placeholders while loading", () => {
    renderTab(makeProvider(), { isLoading: true });
    expect(screen.queryByText("/openai")).not.toBeInTheDocument();
    expect(screen.queryByText("OpenAPI Resources")).not.toBeInTheDocument();
  });
});
