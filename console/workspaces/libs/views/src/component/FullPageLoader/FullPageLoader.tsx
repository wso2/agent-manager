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

import { withBasePath } from "@agent-management-platform/types";
import { Box, useColorScheme } from "@wso2/oxygen-ui";

// Animated loader artwork lives in the web app's public/images. The navy file
// is for light backgrounds, the white one for dark backgrounds.
const LOADER_FOR_LIGHT = "/images/choreo-labs-loader-navy.svg";
const LOADER_FOR_DARK = "/images/choreo-labs-loader-white.svg";

export function FullPageLoader() {
  const { colorScheme } = useColorScheme();
  const src = withBasePath(colorScheme === "dark" ? LOADER_FOR_DARK : LOADER_FOR_LIGHT);
  return (
    <Box
      sx={{
        display: "flex",
        justifyContent: "center",
        alignItems: "center",
        height: "100vh",
        width: "100vw",
      }}
    >
      <img src={src} alt="Loading" aria-label="Loading" width={60} height={60} />
    </Box>
  );
}
