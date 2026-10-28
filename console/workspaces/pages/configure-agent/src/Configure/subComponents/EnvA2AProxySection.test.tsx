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

import { act, fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const setProxy = vi.fn();
const removeProxy = vi.fn();
const republish = vi.fn();
const addConfirmation = vi.fn();
const copyToClipboard = vi.fn().mockResolvedValue(true);

vi.mock("@agent-management-platform/api-client", () => ({
  useSetA2AProxy: () => ({ mutate: setProxy, isPending: false }),
  useDeleteA2AProxy: () => ({ mutate: removeProxy, isPending: false }),
  useRefreshA2AProxy: () => ({ mutate: republish, isPending: false }),
}));
vi.mock("@agent-management-platform/shared-component", () => ({
  useConfirmationDialog: () => ({ addConfirmation }),
  copyToClipboard: (text: string) => copyToClipboard(text),
}));

vi.mock("@agent-management-platform/deploy", () => ({
  ProxySecurityDrawer: ({ open }: { open: boolean }) => (open ? <div>security drawer</div> : null),
}));

import { EnvA2AProxySection } from "./EnvA2AProxySection";

const params = { orgName: "org", projName: "proj", agentName: "agent", envId: "dev" };
const cardUrl = "https://agent.example.com/.well-known/agent-card.json";
const gatewayUrl = "https://gw.example/org/proj/agent";
const stored = { status: "published" as const, lastError: "", gatewayUrl, upstreamAuthHeader: "X-Key" };
const save = () => fireEvent.click(screen.getByRole("button", { name: "Save" }));

beforeEach(() => vi.clearAllMocks());

describe("EnvA2AProxySection", () => {
  it("saves a toggle-only change", () => {
    render(<EnvA2AProxySection params={params} cardUrl={cardUrl} />);
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    fireEvent.click(screen.getByLabelText("Route through gateway"));
    save();
    expect(setProxy).toHaveBeenCalledWith({ params, body: {} }, expect.anything());
  });

  it("disables the controls while a routing save is in flight", () => {
    const { rerender } = render(<EnvA2AProxySection params={params} cardUrl={cardUrl} />);
    fireEvent.click(screen.getByLabelText("Route through gateway"));
    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
    rerender(<EnvA2AProxySection params={params} cardUrl={cardUrl} saving />);
    expect(screen.getByLabelText("Route through gateway")).toBeDisabled();
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("keeps the stored secret when only the header changes", () => {
    render(<EnvA2AProxySection params={params} cardUrl={cardUrl} proxy={stored} />);
    expect(screen.getByText("A value is stored.")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Upstream header"), { target: { value: "X-New" } });
    save();
    expect(setProxy).toHaveBeenCalledWith(
      { params, body: { upstreamAuth: { header: "X-New" } } }, expect.anything());
  });

  it("forgets a replaced secret once it is saved under the same header", () => {
    render(<EnvA2AProxySection params={params} cardUrl={cardUrl} proxy={stored} />);
    fireEvent.click(screen.getByRole("button", { name: "Replace" }));
    fireEvent.change(screen.getByLabelText("Header value"), { target: { value: "s3cret" } });
    save();
    expect(setProxy).toHaveBeenCalledWith(
      { params, body: { upstreamAuth: { header: "X-Key", value: "s3cret" } } },
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );
    act(() => setProxy.mock.calls[0][1].onSuccess());
    expect(screen.queryByLabelText("Header value")).not.toBeInTheDocument();
    expect(screen.getByText("A value is stored.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("refuses a new value while the card URL is http", () => {
    render(<EnvA2AProxySection params={params} cardUrl="http://agent.example.com/card.json" proxy={stored} />);
    fireEvent.click(screen.getByRole("button", { name: "Replace" }));
    fireEvent.change(screen.getByLabelText("Header value"), { target: { value: "s3cret" } });
    expect(screen.getByText("The card URL must use https to send a header value")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("asks before clearing the stored value", () => {
    render(<EnvA2AProxySection params={params} cardUrl={cardUrl} proxy={stored} />);
    fireEvent.click(screen.getByRole("button", { name: "Clear" }));
    save();
    expect(setProxy).not.toHaveBeenCalled();
    addConfirmation.mock.calls[0][0].onConfirm();
    expect(setProxy).toHaveBeenCalledWith({ params, body: {} }, expect.anything());
  });

  it("asks before switching to direct", () => {
    render(<EnvA2AProxySection params={params} cardUrl={cardUrl} proxy={stored} />);
    fireEvent.click(screen.getByLabelText("Route through gateway"));
    save();
    expect(removeProxy).not.toHaveBeenCalled();
    addConfirmation.mock.calls[0][0].onConfirm();
    expect(removeProxy).toHaveBeenCalledWith(params);
  });

  it("keeps Save disabled for a value the gateway cannot carry", () => {
    render(<EnvA2AProxySection params={params} cardUrl={cardUrl} />);
    fireEvent.click(screen.getByLabelText("Route through gateway"));
    fireEvent.change(screen.getByLabelText("Upstream header"), { target: { value: "X-Key" } });
    fireEvent.change(screen.getByLabelText("Header value"), { target: { value: "it's" } });
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("shows a failed publish with its reason and offers a retry", () => {
    render(<EnvA2AProxySection params={params} cardUrl={cardUrl} proxy={{ ...stored, status: "failed", lastError: "no JSONRPC interface" }} />);
    expect(screen.getByText("Gateway publish failed")).toBeInTheDocument();
    expect(screen.getByText(/no JSONRPC interface/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(republish).toHaveBeenCalledWith(params);
  });

  it("opens the gateway security settings only while proxied", async () => {
    const { rerender } = render(<EnvA2AProxySection params={params} cardUrl={cardUrl} />);
    expect(screen.queryByRole("button", { name: /security settings/i })).not.toBeInTheDocument();
    rerender(<EnvA2AProxySection params={params} cardUrl={cardUrl} proxy={stored} />);
    fireEvent.click(screen.getByRole("button", { name: /security settings/i }));
    expect(await screen.findByText("security drawer")).toBeInTheDocument();
  });

  it("shows the gateway URL once published and copies it", async () => {
    render(<EnvA2AProxySection params={params} cardUrl={cardUrl} proxy={stored} />);
    expect(screen.getByText(gatewayUrl)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Copy gateway URL" }));
    expect(copyToClipboard).toHaveBeenCalledWith(gatewayUrl);
    fireEvent.mouseOver(screen.getByRole("button", { name: "Copy gateway URL" }));
    expect(await screen.findByText("Copied!")).toBeInTheDocument();
  });

  it("says no ingress gateway is mapped when a published proxy has no gateway URL", () => {
    render(<EnvA2AProxySection params={params} cardUrl={cardUrl} proxy={{ ...stored, gatewayUrl: "" }} />);
    expect(screen.getByText(/No ingress gateway is mapped/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Copy gateway URL" })).not.toBeInTheDocument();
  });

  it("hides the gateway URL until the proxy is published", () => {
    render(<EnvA2AProxySection params={params} cardUrl={cardUrl} proxy={{ ...stored, status: "pending" }} />);
    expect(screen.queryByText(gatewayUrl)).not.toBeInTheDocument();
    expect(screen.queryByText(/No ingress gateway is mapped/)).not.toBeInTheDocument();
  });
});
