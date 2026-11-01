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
import { describe, expect, it } from "vitest";
import { RolesGroupsReadOnlyNote } from "./RolesGroupsReadOnlyNote";

describe("RolesGroupsReadOnlyNote", () => {
  it("says roles and groups are read only here and managed in the first environment", () => {
    render(<RolesGroupsReadOnlyNote homeEnvironment="Development" />);
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Roles and groups are managed in the Development environment and carried to this environment when the identity is promoted, so they are read only here.",
    );
  });

  it("falls back to the first environment when its name is unknown", () => {
    render(<RolesGroupsReadOnlyNote />);
    expect(screen.getByRole("alert")).toHaveTextContent("managed in the first environment");
  });
});
