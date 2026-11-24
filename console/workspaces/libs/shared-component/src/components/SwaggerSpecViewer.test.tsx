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

import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

type SwaggerProps = {
  spec?: unknown;
  url?: string;
  plugins?: {
    statePlugins?: { spec: { wrapSelectors: Record<string, () => () => unknown> } };
    wrapComponents?: Record<string, () => () => unknown>;
  }[];
  docExpansion: string;
  supportedSubmitMethods: string[];
};
const received: SwaggerProps[] = [];

vi.mock("swagger-ui-react", () => ({
  default: (props: SwaggerProps) => {
    received.push(props);
    return <div data-testid="swagger" />;
  },
}));

import SwaggerSpecViewer from "./SwaggerSpecViewer";

describe("SwaggerSpecViewer", () => {
  it("passes the spec with try-it-out disabled and no plugins by default", async () => {
    received.length = 0;
    const { container } = render(<SwaggerSpecViewer spec={{ openapi: "3.0.0" }} />);
    await screen.findByTestId("swagger");
    const props = received[received.length - 1];
    expect(props.spec).toEqual({ openapi: "3.0.0" });
    expect(props.url).toBeUndefined();
    expect(props.supportedSubmitMethods).toEqual([]);
    expect(props.docExpansion).toBe("list");
    expect(props.plugins).toBeUndefined();
    expect(container.firstChild).toHaveClass("swagger-spec-viewer");
  });

  it("loads from a URL and wires plugins that hide info, servers and auth", async () => {
    received.length = 0;
    const { container } = render(
      <SwaggerSpecViewer
        url="https://x/spec.json"
        spec={{ ignored: true }}
        className="extra"
        hideInfoSection
        hideServers
        hideAuthorizeButton
        hideTagHeaders
        hideOperationHeader
      />,
    );
    await screen.findByTestId("swagger");
    const props = received[received.length - 1];
    expect(props.url).toBe("https://x/spec.json");
    expect(props.spec).toBeUndefined();
    const plugin = props.plugins![0];
    const selectors = plugin.statePlugins!.spec.wrapSelectors;
    expect(selectors.servers()()).toEqual([]);
    expect(selectors.schemes()()).toEqual([]);
    expect(selectors.securityDefinitions()()).toBeNull();
    expect(plugin.wrapComponents!.info()()).toBeNull();
    expect(plugin.wrapComponents!.authorizeBtn()()).toBeNull();
    expect(container.firstChild).toHaveClass(
      "hide-info-section",
      "hide-servers",
      "hide-authorize",
      "hide-tag-headers",
      "hide-operation-header",
      "extra",
    );
  });

  it("uses only component wrappers when just the info section is hidden", async () => {
    received.length = 0;
    render(<SwaggerSpecViewer spec={{}} hideInfoSection />);
    await screen.findByTestId("swagger");
    const plugin = received[received.length - 1].plugins![0];
    expect(plugin.statePlugins).toBeUndefined();
    expect(Object.keys(plugin.wrapComponents!)).toEqual(["info"]);
  });
});
