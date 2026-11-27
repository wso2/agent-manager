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

import { beforeEach, describe, expect, it } from "vitest";
import { dropStaleRefreshTimer } from "./browserSession";

describe("dropStaleRefreshTimer", () => {
  const key = "temporary_data-instance_0-AGENT_MANAGER_CONSOLE";

  beforeEach(() => {
    window.localStorage.clear();
  });

  it("removes only the stored refresh timer of this console's SDK data", () => {
    window.localStorage.setItem(
      key,
      JSON.stringify({ op_config_initiated: true, refresh_token_timer: "7" }),
    );
    const other = "temporary_data-instance_0-WSO2_CLOUD_CONSOLE";
    window.localStorage.setItem(other, JSON.stringify({ refresh_token_timer: "3" }));

    dropStaleRefreshTimer("AGENT_MANAGER_CONSOLE");

    expect(JSON.parse(window.localStorage.getItem(key)!)).toEqual({ op_config_initiated: true });
    expect(JSON.parse(window.localStorage.getItem(other)!)).toEqual({ refresh_token_timer: "3" });
  });

  it("leaves data without a timer, or that is not JSON, as it is", () => {
    window.localStorage.setItem(key, "not json");
    const second = "temporary_data-instance_1-AGENT_MANAGER_CONSOLE";
    window.localStorage.setItem(second, '{"op_config_initiated":true}');

    dropStaleRefreshTimer("AGENT_MANAGER_CONSOLE");

    expect(window.localStorage.getItem(key)).toBe("not json");
    expect(window.localStorage.getItem(second)).toBe('{"op_config_initiated":true}');
  });

  it("does nothing without a client ID", () => {
    window.localStorage.setItem(key, JSON.stringify({ refresh_token_timer: "7" }));

    dropStaleRefreshTimer(undefined);

    expect(JSON.parse(window.localStorage.getItem(key)!)).toEqual({ refresh_token_timer: "7" });
  });
});
