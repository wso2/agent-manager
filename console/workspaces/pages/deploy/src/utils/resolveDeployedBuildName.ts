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

import type { BuildResponse } from "@agent-management-platform/types";

const LEGACY_IMAGE_SUFFIX = "-image";

/**
 * Returns the name of the build that produced the deployed image, or "" when
 * no build can be found.
 *
 * The build is looked up by its image first, so this does not depend on how
 * the build workflow names images. A workflow that pushes every build to one
 * repository per component, with a tag per build, has no build name in the
 * repository name.
 *
 * When no listed build has the image (the build list is still loading, or the
 * image comes from another agent through an agent kind), it falls back to the
 * `<build name>-image:<tag>` repository naming the default build workflows use.
 */
export function resolveDeployedBuildName(
  imageId?: string | null,
  builds?: ReadonlyArray<Pick<BuildResponse, "buildName" | "imageId">> | null,
): string {
  const normalized = imageId?.trim();
  if (!normalized) {
    return "";
  }

  const match = builds?.find((build) => build.imageId?.trim() === normalized);
  if (match?.buildName) {
    return match.buildName;
  }

  return buildNameFromLegacyImage(normalized);
}

function buildNameFromLegacyImage(imageId: string): string {
  const lastSegment = imageId.split("/").pop() ?? "";
  const repository = lastSegment.split(":")[0] ?? "";
  if (
    repository.length <= LEGACY_IMAGE_SUFFIX.length ||
    !repository.endsWith(LEGACY_IMAGE_SUFFIX)
  ) {
    return "";
  }
  return repository.slice(0, -LEGACY_IMAGE_SUFFIX.length);
}
