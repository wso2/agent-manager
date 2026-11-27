/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import { describe, it, expect } from "vitest";
import { resolveDeployedBuildName } from "./resolveDeployedBuildName";

const legacyImage =
  "registry.openchoreo-workflow-plane.svc.cluster.local:10082/my-agent-1794542961573-image:v1-a1b2c3d4";
const perComponentImage =
  "541673202425.dkr.ecr.us-east-2.amazonaws.com/wc-019f6604-e4641047_default_my-agent:my-agent-1794542961573-a1b2c3d4";

describe("resolveDeployedBuildName", () => {
  it("returns an empty name when there is no image", () => {
    expect(resolveDeployedBuildName(undefined, [])).toBe("");
    expect(resolveDeployedBuildName(null, [])).toBe("");
    expect(resolveDeployedBuildName("   ", [])).toBe("");
  });

  it("finds the build that produced the image", () => {
    const builds = [
      { buildName: "my-agent-1794542000000", imageId: "other:v1-00000000" },
      { buildName: "my-agent-1794542961573", imageId: perComponentImage },
    ];
    expect(resolveDeployedBuildName(perComponentImage, builds)).toBe(
      "my-agent-1794542961573",
    );
  });

  it("finds a legacy-named build by its image too", () => {
    const builds = [{ buildName: "my-agent-1794542961573", imageId: legacyImage }];
    expect(resolveDeployedBuildName(legacyImage, builds)).toBe("my-agent-1794542961573");
  });

  it("falls back to the build name in a legacy image when the builds are not loaded", () => {
    expect(resolveDeployedBuildName(legacyImage, undefined)).toBe("my-agent-1794542961573");
    expect(resolveDeployedBuildName(legacyImage, [])).toBe("my-agent-1794542961573");
    expect(resolveDeployedBuildName("my-agent-1-image", [])).toBe("my-agent-1");
  });

  it("returns an empty name for an unmatched image without the legacy suffix", () => {
    expect(resolveDeployedBuildName(perComponentImage, [])).toBe("");
    expect(resolveDeployedBuildName("docker.io/library/nginx:1.27", [])).toBe("");
    expect(resolveDeployedBuildName("registry/-image:v1", [])).toBe("");
  });
});
