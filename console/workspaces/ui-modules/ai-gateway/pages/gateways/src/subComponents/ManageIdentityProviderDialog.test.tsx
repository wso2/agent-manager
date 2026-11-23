/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
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


import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { FeatureManagementProvider } from "@agent-management-platform/feature-management";
import { ManageIdentityProviderDialog } from "./ManageIdentityProviderDialog";

const hoisted = vi.hoisted(() => ({
  upsert: vi.fn(),
  remove: vi.fn(),
  discover: vi.fn(),
  listGateways: vi.fn(),
  confirmIfUnsaved: vi.fn(),
  environments: [] as Array<{ name: string; displayName: string }>,
}));

// MUI renders are slow on a busy machine; leave headroom over the 5s default.
vi.setConfig({ testTimeout: 30_000 });

vi.mock("@agent-management-platform/api-client", () => ({
  useUpsertIdentityProvider: () => ({ mutateAsync: hoisted.upsert, isPending: false }),
  useDeleteIdentityProvider: () => ({ mutateAsync: hoisted.remove, isPending: false }),
  useDiscoverOidc: () => ({ mutateAsync: hoisted.discover, isPending: false }),
  useListEnvironments: () => ({ data: hoisted.environments }),
  useListGateways: hoisted.listGateways,
}));

vi.mock("@agent-management-platform/shared-component", () => ({
  onlyEnvironmentName: (envs?: { name: string }[]) =>
    envs?.length === 1 ? envs[0].name : undefined,
  useUnsavedChangesGuard: () => ({ allowNavigation: (go: () => void) => go() }),
  useConfirmIfUnsaved: () => (action: () => void, isDirty?: boolean) => {
    hoisted.confirmIfUnsaved(isDirty);
    if (!isDirty) action();
  },
}));

const DEV = { id: "e1", name: "dev", displayName: "Development" };
const PROD = { id: "e2", name: "prod", displayName: "Production" };
const LOCKED = { uuid: "gw-1", name: "Edge", environments: [DEV] };

type Props = Partial<React.ComponentProps<typeof ManageIdentityProviderDialog>>;

const renderDialog = (props: Props = {}, managedMode = false) => {
  const onClose = vi.fn();
  render(
    <FeatureManagementProvider featureFlags={{ enableIdentityProviderManagedMode: managedMode }}>
      <ManageIdentityProviderDialog
        open
        onClose={onClose}
        orgId="acme"
        mode="upsert"
        lockedGateway={LOCKED as never}
        {...props}
      />
    </FeatureManagementProvider>,
  );
  return onClose;
};

const type = (placeholder: string, value: string) =>
  fireEvent.change(screen.getByPlaceholderText(placeholder), { target: { value } });

const fillProvider = () => {
  type("e.g., AcmeIdP", " AcmeIdP ");
  type("https://idp.example.com", "https://idp.acme ");
  type("https://idp.example.com/oauth2/jwks", "https://idp.acme/jwks");
};

const addButton = () => screen.getByRole("button", { name: "Add" });

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.environments = [DEV, PROD];
  hoisted.listGateways.mockReturnValue({ data: { gateways: [] } });
  hoisted.upsert.mockResolvedValue({});
  hoisted.remove.mockResolvedValue({});
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("ManageIdentityProviderDialog add", () => {
  it("adds a provider to the locked gateway and its only environment", async () => {
    const onClose = renderDialog();
    expect(screen.getByText("Add Identity Provider")).toBeInTheDocument();
    expect(screen.getByDisplayValue("Edge")).toBeInTheDocument();
    expect(screen.queryByText("Environment")).not.toBeInTheDocument();
    expect(addButton()).toBeDisabled();

    fillProvider();
    fireEvent.click(screen.getByRole("checkbox", { name: /Skip TLS verification/ }));
    fireEvent.click(addButton());

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(hoisted.upsert).toHaveBeenCalledWith({
      params: { orgName: "acme", gatewayId: "gw-1", name: "AcmeIdP" },
      body: { issuer: "https://idp.acme", jwksUri: "https://idp.acme/jwks", skipTlsVerify: true },
    });
  });

  it("keeps the drawer open when saving fails", async () => {
    hoisted.upsert.mockRejectedValue(new Error("boom"));
    const onClose = renderDialog();
    fillProvider();
    fireEvent.click(addButton());
    await waitFor(() => expect(hoisted.upsert).toHaveBeenCalled());
    expect(onClose).not.toHaveBeenCalled();
  });

  it("shows the config.toml entry the operator must add, and copies it", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal("navigator", { ...navigator, clipboard: { writeText } });
    renderDialog();
    fillProvider();

    expect(
      screen.getByText(/Add this entry to the gateway's configs\/config.toml/),
    ).toBeInTheDocument();
    const toml = screen.getByText(/\[\[policy_configurations\.jwtauth_v1\.keymanagers\]\]/);
    expect(toml).toHaveTextContent('name = " AcmeIdP "');
    expect(toml).toHaveTextContent('uri = "https://idp.acme/jwks"');

    fireEvent.click(within(toml.parentElement!).getAllByRole("button")[0]);
    await waitFor(() =>
      expect(writeText).toHaveBeenCalledWith(
        expect.stringContaining('issuer = "https://idp.acme "'),
      ),
    );
  });

  it("leaves the config to Agent Manager in managed mode", () => {
    renderDialog({}, true);
    expect(screen.getByText(/Agent Manager patches the gateway configuration/)).toBeInTheDocument();
    expect(screen.queryByText(/config\.toml/)).not.toBeInTheDocument();
  });

  it("fills the issuer and JWKS URI from OIDC discovery", async () => {
    hoisted.discover.mockResolvedValue({
      issuer: "https://login.acme",
      jwksUri: "https://login.acme/keys",
    });
    renderDialog();
    const fetchButton = screen.getByRole("button", { name: "Fetch" });
    expect(fetchButton).toBeDisabled();

    type("https://idp.example.com (or .well-known URL)", " https://login.acme ");
    fireEvent.click(fetchButton);

    await waitFor(() =>
      expect(screen.getByPlaceholderText("https://idp.example.com")).toHaveValue(
        "https://login.acme",
      ),
    );
    expect(hoisted.discover).toHaveBeenCalledWith({ url: "https://login.acme" });
    expect(screen.getByPlaceholderText("https://idp.example.com/oauth2/jwks")).toHaveValue(
      "https://login.acme/keys",
    );
  });

  it("says when discovery fails", async () => {
    hoisted.discover.mockRejectedValue(new Error("404"));
    renderDialog();
    type("https://idp.example.com (or .well-known URL)", "https://bad");
    fireEvent.click(screen.getByRole("button", { name: "Fetch" }));
    expect(await screen.findByText(/Could not discover OIDC configuration/)).toBeInTheDocument();
  });

  it("picks the environment, then a managed gateway in it, when no gateway is locked", async () => {
    hoisted.listGateways.mockImplementation(
      (_params: unknown, options?: { environment: string }) => ({
        data: {
          gateways:
            options?.environment === "prod"
              ? [
                  { uuid: "gw-m", name: "managed", displayName: "Managed GW", hosting: "MANAGED" },
                  { uuid: "gw-s", name: "self", displayName: "Self GW", hosting: "SELF_HOSTED" },
                ]
              : [],
        },
      }),
    );
    const onClose = renderDialog({ lockedGateway: null });

    // Environment leads when no gateway is locked in.
    const [environmentSelect, gatewaySelect] = screen.getAllByRole("combobox");
    expect(environmentSelect).toHaveTextContent("Select an environment");
    expect(gatewaySelect).toHaveTextContent("Select a gateway");
    fireEvent.mouseDown(environmentSelect);
    fireEvent.click(await screen.findByRole("option", { name: "Production" }));
    fireEvent.mouseDown(screen.getAllByRole("combobox")[1]);
    expect(screen.queryByRole("option", { name: "Self GW" })).not.toBeInTheDocument();
    fireEvent.click(await screen.findByRole("option", { name: "Managed GW" }));

    fillProvider();
    fireEvent.click(addButton());
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(hoisted.upsert).toHaveBeenCalledWith(
      expect.objectContaining({
        params: { orgName: "acme", gatewayId: "gw-m", name: "AcmeIdP" },
      }),
    );
  });

  it("asks before discarding edits from the header close, but Cancel closes directly", () => {
    const onClose = renderDialog();
    type("e.g., AcmeIdP", "AcmeIdP");

    fireEvent.click(screen.getByRole("button", { name: /close/i }));
    expect(hoisted.confirmIfUnsaved).toHaveBeenCalledWith(true);
    expect(onClose).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onClose).toHaveBeenCalled();
  });
});

describe("ManageIdentityProviderDialog delete", () => {
  const provider = {
    name: "AcmeIdP",
    type: "custom",
    gatewayId: "gw-1",
    environmentName: "dev",
    issuer: "https://idp.acme",
    jwksUri: "https://idp.acme/jwks",
    skipTlsVerify: false,
  };

  it("removes the chosen provider from its gateway", async () => {
    const onClose = renderDialog({ mode: "delete", provider: provider as never });
    expect(screen.getByText("Remove Identity Provider")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("e.g., AcmeIdP")).toHaveValue("AcmeIdP");
    expect(screen.getByPlaceholderText("e.g., AcmeIdP")).toBeDisabled();
    expect(screen.queryByPlaceholderText("https://idp.example.com")).not.toBeInTheDocument();
    expect(
      screen.getByText(/Remove this entry from the gateway's configs\/config.toml/),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Remove" }));
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(hoisted.remove).toHaveBeenCalledWith({
      orgName: "acme",
      gatewayId: "gw-1",
      name: "AcmeIdP",
    });
    expect(hoisted.upsert).not.toHaveBeenCalled();
  });
});
