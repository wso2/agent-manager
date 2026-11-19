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

import { useAuthHooks } from "@agent-management-platform/auth";
import type { UIManifestResponse } from "@agent-management-platform/types";
import { useApiQuery } from "../../shared/react-query-notifications";
import { getUIManifest } from "../apis/ui-manifest";

/**
 * The console UI the backend offers the signed-in user: which pages of the
 * built-in UI modules to turn on. Read once per user and session (the key
 * carries both, so the next sign-in never sees the last one's pages); a
 * module enabled or disabled on the server shows on the next page load.
 */
export function useUIManifest() {
  const { getToken, userInfo } = useAuthHooks();
  const who = userInfo?.sub ?? userInfo?.username ?? "";
  return useApiQuery<UIManifestResponse>({
    queryKey: ["ui-manifest", who, userInfo?.sessionState ?? ""],
    queryFn: () => getUIManifest(getToken),
    staleTime: Infinity,
    retry: 1,
  });
}
