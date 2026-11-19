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

import { describe, expect, it } from "vitest";
import { render, waitFor } from "@testing-library/react";
import {
  forgetNewGatewayToken,
  forgetRevokedGatewayToken,
  keepNewGatewayToken,
  newGatewayToken,
  useForgetNewGatewayTokenOnLeave,
} from "./newGatewayToken";

function Page({ gatewayId }: { gatewayId: string }) {
  useForgetNewGatewayTokenOnLeave(gatewayId);
  return null;
}

/** Lets the deferred forget run. */
const settle = () => new Promise<void>((resolve) => setTimeout(resolve, 0));

describe("newGatewayToken", () => {
  it("drops the token when its own record is revoked, not another's", () => {
    keepNewGatewayToken("gw-a", "secret", "token-1");

    forgetRevokedGatewayToken("gw-a", "token-2");
    expect(newGatewayToken("gw-a")).toBe("secret");

    forgetRevokedGatewayToken("gw-a", "token-1");
    expect(newGatewayToken("gw-a")).toBeNull();
  });

  it("is forgotten once the last page showing it is left", async () => {
    keepNewGatewayToken("gw-b", "secret");
    const { unmount } = render(<Page gatewayId="gw-b" />);
    expect(newGatewayToken("gw-b")).toBe("secret");

    unmount();
    await waitFor(() => expect(newGatewayToken("gw-b")).toBeNull());
  });

  it("is kept when another of the gateway's pages takes over", async () => {
    keepNewGatewayToken("gw-c", "secret");
    const setup = render(<Page gatewayId="gw-c" />);
    const view = render(<Page gatewayId="gw-c" />);

    setup.unmount();
    await settle();
    expect(newGatewayToken("gw-c")).toBe("secret");

    view.unmount();
    await waitFor(() => expect(newGatewayToken("gw-c")).toBeNull());
  });

  it("keeps a token that is minted after the page was left", async () => {
    const { unmount } = render(<Page gatewayId="gw-d" />);
    unmount();
    await settle();

    keepNewGatewayToken("gw-d", "late", "token-9");
    expect(newGatewayToken("gw-d")).toBe("late");
    forgetNewGatewayToken("gw-d");
  });
});
