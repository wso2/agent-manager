/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useCallback } from "react";
import { copyToClipboard } from "@agent-management-platform/shared-component";
import { useSnackBar } from "@agent-management-platform/views";

/**
 * Copies `value` to the clipboard and reports success/failure via a snackbar,
 * labelled with `label` (e.g. "Context", "Upstream URL"). Shared by the
 * Overview tab's field copy buttons and the page header's Context field.
 */
export function useCopyWithFeedback() {
  const { pushSnackBar } = useSnackBar();

  return useCallback(
    (value: string, label: string) => {
      void copyToClipboard(value).then((succeeded) => {
        pushSnackBar(
          succeeded
            ? { message: `${label} copied to clipboard`, type: "success" }
            : { message: `Failed to copy ${label}`, type: "error" },
        );
      });
    },
    [pushSnackBar],
  );
}
