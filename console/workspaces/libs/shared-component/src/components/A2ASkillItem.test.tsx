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

import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import { A2ASkillItem } from "./A2ASkillItem";
import type { ParsedA2ASkill } from "../utils/a2aCard";

const skill: ParsedA2ASkill = {
  id: "s1",
  name: "order-status",
  description: "Where an order is",
  tags: ["orders", "orders"],
  examples: ["Where is order 7?"],
  inputModes: ["text/plain"],
  outputModes: ["application/json"],
};

const renderItem = (value: ParsedA2ASkill) =>
  render(
    <ThemeProvider theme={createTheme()}>
      <A2ASkillItem skill={value} />
    </ThemeProvider>,
  );

describe("A2ASkillItem", () => {
  it("renders name, description, tags, modes and examples", () => {
    renderItem(skill);
    expect(screen.getByText("order-status")).toBeInTheDocument();
    expect(screen.getByText("Where an order is")).toBeInTheDocument();
    expect(screen.getAllByText("orders")).toHaveLength(2);
    screen.getAllByText("orders").forEach((tag) => {
      expect(tag.closest(".MuiChip-root")).toHaveClass("MuiChip-outlined");
    });
    expect(screen.getByText("Input")).toBeInTheDocument();
    expect(screen.getByText("text/plain")).toBeInTheDocument();
    expect(screen.getByText("Output")).toBeInTheDocument();
    expect(screen.getByText("application/json")).toBeInTheDocument();
    expect(screen.getByText("e.g. Where is order 7?")).toBeInTheDocument();
  });

  it("omits empty parts", () => {
    renderItem({ ...skill, description: "", tags: [], examples: [], inputModes: [], outputModes: [] });
    expect(screen.queryByText("Input")).not.toBeInTheDocument();
    expect(screen.queryByText("Output")).not.toBeInTheDocument();
    expect(screen.queryByText(/^e\.g\./)).not.toBeInTheDocument();
  });
});
