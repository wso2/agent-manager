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

import { useCallback } from "react";
import { Trash } from "@wso2/oxygen-ui-icons-react";
import { useDeleteAgentKindVersion } from "@agent-management-platform/api-client";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";

export interface UseDeleteVersionActionParams {
  orgName?: string;
  kindName?: string;
  onDeleted?: () => void;
}

/** Shared "delete version" confirm flow for the list row action and the details danger zone. */
export function useDeleteVersionAction(
  { orgName, kindName, onDeleted }: UseDeleteVersionActionParams,
) {
  const { addConfirmation } = useConfirmationDialog();
  const { mutate: deleteAgentKindVersion, isPending: isDeletingVersion } =
    useDeleteAgentKindVersion();

  const confirmDeleteVersion = useCallback((versionTag: string) => {
    addConfirmation({
      analytics: { entity: "agent-kind-version", action: "delete" },
      title: "Delete Version",
      description: `Are you sure you want to delete version "${versionTag}"? This action cannot be undone.`,
      confirmButtonText: "Delete",
      confirmButtonColor: "error",
      confirmButtonIcon: <Trash size={16} />,
      onConfirm: () => deleteAgentKindVersion(
        { orgName: orgName!, kindName: kindName!, versionTag },
        onDeleted ? { onSuccess: onDeleted } : undefined,
      ),
    });
  }, [addConfirmation, deleteAgentKindVersion, orgName, kindName, onDeleted]);

  return { confirmDeleteVersion, isDeletingVersion };
}
