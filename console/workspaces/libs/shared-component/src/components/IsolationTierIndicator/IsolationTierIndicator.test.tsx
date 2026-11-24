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

import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import {
  getIsolationTierMeta,
  IsolationTierBadge,
  IsolationTierChip,
} from "./IsolationTierIndicator";

describe("getIsolationTierMeta", () => {
  it.each([
    ["runc", 1, "Sandbox L1"],
    ["gvisor", 2, "Sandbox L2"],
    ["kata", 3, "Sandbox L3"],
    ["unknown", 1, "Sandbox L1"],
    [undefined, 1, "Sandbox L1"],
  ])("%s -> tier %s", (tier, level, label) => {
    const meta = getIsolationTierMeta(tier);
    expect(meta.tier).toBe(level);
    expect(meta.shortLabel).toBe(label);
  });
});

describe("IsolationTierChip / IsolationTierBadge", () => {
  it("labels the chip and explains it in a tooltip", async () => {
    render(<IsolationTierChip tier="gvisor" />);
    const chip = screen.getByText("Sandbox L2");
    fireEvent.mouseOver(chip);
    expect(await screen.findByRole("tooltip")).toHaveTextContent("Sandbox Level 2");
  });

  it("renders a shield icon with the level in its tooltip", async () => {
    const { container } = render(<IsolationTierBadge tier="kata" size={22} color="red" />);
    const svg = container.querySelector("svg")!;
    expect(svg).toHaveAttribute("width", "22");
    fireEvent.mouseOver(svg.parentElement!);
    expect(await screen.findByRole("tooltip")).toHaveTextContent("Sandbox Level 3");
  });
});
