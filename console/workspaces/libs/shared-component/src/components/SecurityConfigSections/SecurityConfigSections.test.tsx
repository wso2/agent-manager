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

import { createRef } from "react";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ConfigurationResponse } from "@agent-management-platform/types";

vi.mock("@agent-management-platform/api-client", () => ({
  useGetAgent: vi.fn(),
  useListIdentityProviders: vi.fn(() => ({ data: undefined })),
}));

import {
  useGetAgent,
  useListIdentityProviders,
} from "@agent-management-platform/api-client";
import { SecurityConfigSections, type SecurityConfigHandle } from "./SecurityConfigSections";

const mockAgent = (type: string, subType?: string) =>
  vi.mocked(useGetAgent).mockReturnValue({
    data: { agentType: { type, subType } },
  } as unknown as ReturnType<typeof useGetAgent>);

const mockIdps = (names: string[]) =>
  vi.mocked(useListIdentityProviders).mockReturnValue({
    data: { list: names.map((name) => ({ name })) },
  } as unknown as ReturnType<typeof useListIdentityProviders>);

const baseConfig = (overrides: Partial<ConfigurationResponse> = {}): ConfigurationResponse => ({
  projectName: "proj",
  agentName: "agent",
  environment: "dev",
  configurations: { env: [] },
  ...overrides,
});

function renderSections(
  cfg: ConfigurationResponse | undefined,
  props: { onValidityChange?: (v: boolean) => void; disabled?: boolean } = {},
) {
  const ref = createRef<SecurityConfigHandle>();
  const utils = render(
    <SecurityConfigSections
      ref={ref}
      orgName="org"
      projName="proj"
      agentName="agent"
      environment="dev"
      open
      configurations={cfg}
      {...props}
    />,
  );
  return { ref, ...utils };
}

const openSection = (title: RegExp) => {
  const heading = screen.getByRole("button", { name: title });
  if (heading.getAttribute("aria-expanded") !== "true") fireEvent.click(heading);
};

const addChip = (placeholder: string, value: string) => {
  const input = screen.getByPlaceholderText(placeholder);
  fireEvent.change(input, { target: { value } });
  fireEvent.keyDown(input, { key: "Enter" });
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(useListIdentityProviders).mockReturnValue({
    data: undefined,
  } as unknown as ReturnType<typeof useListIdentityProviders>);
  mockAgent("agent-api", "chat-api");
});

describe("SecurityConfigSections", () => {
  it("renders nothing and sends nothing for an agent without gateway policies", () => {
    mockAgent("internal-agent");
    const { ref, container } = renderSections(baseConfig());
    expect(container).toBeEmptyDOMElement();
    expect(ref.current?.buildBody()).toEqual({});
    expect(useListIdentityProviders).toHaveBeenCalledWith({ orgName: "" });
  });

  it("defaults to API key auth and closed CORS when nothing is configured", () => {
    const { ref } = renderSections(undefined);
    expect(useListIdentityProviders).toHaveBeenCalledWith({ orgName: "org" });
    expect(ref.current?.buildBody()).toEqual({
      enableApiKeySecurity: true,
      enableOAuthSecurity: false,
      corsConfig: {
        enabled: false,
        allowOrigin: ["*"],
        allowMethods: ["GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"],
        allowHeaders: ["authorization", "Content-Type", "Origin", "X-API-Key"],
        allowCredentials: false,
      },
      resilienceTimeoutSeconds: 30,
    });
  });

  it("seeds 'none' when API key security is explicitly off", () => {
    const { ref } = renderSections(baseConfig({ enableApiKeySecurity: false }));
    expect(ref.current?.buildBody()).toMatchObject({
      enableApiKeySecurity: false,
      enableOAuthSecurity: false,
    });
  });

  describe("endpoint authentication", () => {
    it("disables OAuth until the environment has an identity provider", () => {
      renderSections(baseConfig());
      openSection(/Endpoint Authentication/);
      expect(
        screen.getByRole("radio", { name: "OAuth — configure an identity provider first" }),
      ).toBeDisabled();
    });

    it("pre-fills every identity provider when switching to OAuth and sends the OAuth config", () => {
      mockIdps(["idp-a", "idp-b"]);
      const onValidityChange = vi.fn();
      const { ref } = renderSections(baseConfig({ enableApiKeySecurity: true }), {
        onValidityChange,
      });
      openSection(/Endpoint Authentication/);
      fireEvent.click(screen.getByRole("radio", { name: "OAuth" }));

      addChip("Add accepted audience (aud) values", "api://amp");
      const headerName = screen.getByPlaceholderText("Authorization");
      const prefix = screen.getByPlaceholderText("Bearer");
      fireEvent.change(headerName, { target: { value: "  " } });
      fireEvent.change(prefix, { target: { value: "Token" } });

      expect(ref.current?.validate()).toBe(true);
      expect(ref.current?.buildBody()).toMatchObject({
        enableApiKeySecurity: false,
        enableOAuthSecurity: true,
        oauthConfig: {
          issuers: ["idp-a", "idp-b"],
          audiences: ["api://amp"],
          headerName: "Authorization",
          authHeaderPrefix: "Token",
          forwardToken: true,
        },
      });
      expect(onValidityChange).toHaveBeenLastCalledWith(true);
    });

    it("is invalid in OAuth mode without an identity provider", async () => {
      mockIdps(["idp-a"]);
      const onValidityChange = vi.fn();
      const { ref } = renderSections(
        baseConfig({ enableOAuthSecurity: true, oauthConfig: { issuers: ["idp-a"] } }),
        { onValidityChange },
      );
      openSection(/Endpoint Authentication/);

      // Remove the only issuer chip.
      const chip = screen.getByRole("button", { name: "idp-a" });
      fireEvent.click(within(chip).getByTestId("CancelIcon"));

      expect(screen.getByText("Select at least one identity provider.")).toBeInTheDocument();
      expect(ref.current?.validate()).toBe(false);
      await waitFor(() => expect(onValidityChange).toHaveBeenLastCalledWith(false));
    });

    it("switches back to no authentication", () => {
      const { ref } = renderSections(baseConfig({ enableApiKeySecurity: true }));
      openSection(/Endpoint Authentication/);
      fireEvent.click(screen.getByRole("radio", { name: "No authentication" }));
      expect(ref.current?.buildBody()).toMatchObject({ enableApiKeySecurity: false });
    });
  });

  describe("CORS", () => {
    const openCorsAdvanced = () => {
      openSection(/CORS Configuration/);
      fireEvent.click(screen.getByRole("switch", { name: "Enable CORS" }));
      fireEvent.click(screen.getByText("Advanced"));
    };

    it("restricts origins and allows credentials once all-origins is off", () => {
      const { ref } = renderSections(baseConfig());
      openCorsAdvanced();

      fireEvent.click(screen.getByRole("checkbox", { name: "Allow all origins" }));
      addChip("Add origin and press Enter", "https://app.example.com");
      fireEvent.click(screen.getByRole("checkbox", { name: "Allow credentials" }));

      expect(ref.current?.buildBody().corsConfig).toMatchObject({
        enabled: true,
        allowOrigin: ["https://app.example.com"],
        allowCredentials: true,
      });
      expect(screen.getByRole("button", { name: /CORS Configuration/ })).toHaveTextContent(
        "On · 1 origin",
      );

      // Back to all origins drops credentials.
      fireEvent.click(screen.getByRole("checkbox", { name: "Allow all origins" }));
      expect(ref.current?.buildBody().corsConfig).toMatchObject({
        allowOrigin: ["*"],
        allowCredentials: false,
      });
    });

    it("adds allowed methods and headers", () => {
      const { ref } = renderSections(baseConfig());
      openCorsAdvanced();
      addChip("Add method and press Enter", "HEAD");
      addChip("Add header and press Enter", "X-Trace");
      expect(ref.current?.buildBody().corsConfig).toMatchObject({
        allowMethods: ["GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"],
        allowHeaders: ["authorization", "Content-Type", "Origin", "X-API-Key", "X-Trace"],
      });
    });

    it("seeds a stored origin list", () => {
      const { ref } = renderSections(
        baseConfig({
          corsConfig: {
            enabled: true,
            allowOrigin: ["https://a.example", "https://b.example"],
            allowCredentials: true,
          },
        }),
      );
      expect(screen.getByRole("button", { name: /CORS Configuration/ })).toHaveTextContent(
        "On · 2 origins",
      );
      expect(ref.current?.buildBody().corsConfig).toMatchObject({
        allowOrigin: ["https://a.example", "https://b.example"],
        allowCredentials: true,
      });
    });
  });

  describe("request timeout", () => {
    it("shows a whole-minute timeout in minutes and sends seconds", () => {
      const { ref } = renderSections(baseConfig({ resilienceTimeoutSeconds: 600 }));
      expect(screen.getByRole("button", { name: /Request Timeout/ })).toHaveTextContent("10 minutes");
      expect(ref.current?.buildBody().resilienceTimeoutSeconds).toBe(600);

      openSection(/Request Timeout/);
      fireEvent.mouseDown(screen.getByRole("combobox"));
      fireEvent.click(screen.getByRole("option", { name: "Seconds" }));
      expect(ref.current?.buildBody().resilienceTimeoutSeconds).toBe(10);
    });

    it.each([
      ["zero", "0"],
      ["blank", ""],
      ["fractional", "1.5"],
      ["over the cap", "3601"],
    ])("rejects a %s duration and restores the default on blur", (_label, value) => {
      const onValidityChange = vi.fn();
      const { ref } = renderSections(baseConfig({ resilienceTimeoutSeconds: 45 }), {
        onValidityChange,
      });
      openSection(/Request Timeout/);
      const input = screen.getByPlaceholderText("e.g. 30");
      fireEvent.change(input, { target: { value } });

      expect(screen.getByText("Enter a duration between 1s and 60m")).toBeInTheDocument();
      expect(ref.current?.validate()).toBe(false);
      expect(onValidityChange).toHaveBeenLastCalledWith(false);
      // An invalid value is never sent; the default goes instead.
      expect(ref.current?.buildBody().resilienceTimeoutSeconds).toBe(30);

      fireEvent.blur(input);
      expect(input).toHaveValue(30);
      expect(ref.current?.validate()).toBe(true);
    });
  });

  describe("Agent Card CORS (A2A)", () => {
    it("requires an origin for a custom card policy without all-origins", () => {
      mockAgent("agent-api", "a2a-agent");
      const onValidityChange = vi.fn();
      const { ref } = renderSections(baseConfig(), { onValidityChange });
      openSection(/Agent Card CORS/);
      fireEvent.click(screen.getByRole("radio", { name: "Custom" }));

      const card = () => screen.getByRole("button", { name: /Agent Card CORS/ });
      expect(card()).toHaveTextContent("On · all origins");

      // Unchecking all-origins clears the wildcard and leaves no origins.
      const cardSection = screen.getByText("Enable Agent Card CORS").closest(".MuiStack-root") as HTMLElement;
      fireEvent.click(within(cardSection).getByRole("checkbox", { name: "Allow all origins" }));
      expect(screen.getByText("Add at least one origin.")).toBeInTheDocument();
      expect(ref.current?.validate()).toBe(false);
      expect(onValidityChange).toHaveBeenLastCalledWith(false);

      const originInputs = screen.getAllByPlaceholderText("Add origin and press Enter");
      const cardOrigin = originInputs[originInputs.length - 1];
      fireEvent.change(cardOrigin, { target: { value: "https://card.example" } });
      fireEvent.keyDown(cardOrigin, { key: "Enter" });
      fireEvent.click(within(cardSection).getByRole("checkbox", { name: "Allow credentials" }));

      expect(ref.current?.validate()).toBe(true);
      expect(card()).toHaveTextContent("On · 1 origin");
      expect(ref.current?.buildBody().agentCardCorsConfig).toEqual({
        inherit: false,
        enabled: true,
        allowOrigin: ["https://card.example"],
        allowHeaders: ["Content-Type"],
        allowCredentials: true,
      });

      fireEvent.click(within(cardSection).getByRole("checkbox", { name: "Allow all origins" }));
      expect(ref.current?.buildBody().agentCardCorsConfig).toMatchObject({
        allowOrigin: ["*"],
        allowCredentials: false,
      });

      fireEvent.click(screen.getByRole("switch", { name: "Enable Agent Card CORS" }));
      expect(card()).toHaveTextContent("Off");
    });
  });
});
