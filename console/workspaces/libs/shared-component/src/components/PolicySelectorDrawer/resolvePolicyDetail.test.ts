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

import { describe, expect, it } from "vitest";
import { policy } from "./__test__/helpers";
import { resolvePolicyDetail } from "./resolvePolicyDetail";

describe("resolvePolicyDetail", () => {
  it("treats an installed policy without parameters as an empty schema", () => {
    const detail = resolvePolicyDetail(
      policy({ status: "installed", parameters: undefined }),
    );
    expect(detail).not.toBeNull();
    expect(detail?.name).toBe("pii-masking");
  });

  it("has no definition for a not-installed policy without parameters", () => {
    expect(
      resolvePolicyDetail(policy({ status: "not_installed", parameters: undefined })),
    ).toBeNull();
  });
});
