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

import { Trash2 } from "@wso2/oxygen-ui-icons-react";
import { useDeleteBudget } from "@agent-management-platform/api-client";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";

/**
 * Delete a budget after a confirmation, worded as the console's other deletes (agents,
 * projects, LLM providers). Used by the budgets list and the budget's page.
 */
export function useConfirmDeleteBudget(orgName?: string) {
  const { mutate: remove, isPending } = useDeleteBudget();
  const { addConfirmation } = useConfirmationDialog();
  const confirmDelete = (
    budgetName: string,
    displayName: string,
    onDeleted?: () => void,
  ) =>
    addConfirmation({
      analytics: { entity: "budget", action: "delete" },
      title: "Delete Budget?",
      description: `Are you sure you want to delete the budget "${displayName}"? Its spend history is deleted too. This action cannot be undone.`,
      confirmButtonText: "Delete",
      confirmButtonColor: "error",
      confirmButtonIcon: <Trash2 size={16} />,
      onConfirm: () =>
        remove({ orgName, budgetName }, { onSuccess: () => onDeleted?.() }),
    });
  return { confirmDelete, isDeleting: isPending };
}
