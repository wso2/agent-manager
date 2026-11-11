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

import React, { useMemo, useState } from "react";

/**
 * Tracks an unsaved add/remove delta against a server-known set of member
 * IDs — re-adding a not-yet-saved removal just un-marks it instead of
 * queuing a duplicate add, and removing a not-yet-saved addition drops it
 * instead of queuing a no-op remove. Shared by the group-members and
 * role-agents/role-groups assignment pickers.
 */
export function useAssignmentDelta<T>(initialIds: string[], idOf: (item: T) => string) {
  const [pendingAdds, setPendingAdds] = useState<T[]>([]);
  const [removedIds, setRemovedIds] = useState<Set<string>>(new Set());

  const activeIds = useMemo(
    () => initialIds.filter((id) => !removedIds.has(id)),
    [initialIds, removedIds],
  );
  const pendingAddIds = useMemo(() => pendingAdds.map(idOf), [pendingAdds, idOf]);
  const excludedIds = useMemo(
    () => new Set([...activeIds, ...pendingAddIds]),
    [activeIds, pendingAddIds],
  );

  const handleAdd = (_e: React.SyntheticEvent, value: T | null) => {
    if (!value) return;
    const id = idOf(value);
    if (removedIds.has(id)) {
      setRemovedIds((prev) => {
        const next = new Set(prev);
        next.delete(id);
        return next;
      });
    } else {
      setPendingAdds((prev) => [...prev, value]);
    }
  };

  const handleRemove = (id: string) => {
    if (pendingAdds.some((item) => idOf(item) === id)) {
      setPendingAdds((prev) => prev.filter((item) => idOf(item) !== id));
    } else {
      setRemovedIds((prev) => new Set([...prev, id]));
    }
  };

  const reset = () => {
    setPendingAdds([]);
    setRemovedIds(new Set());
  };

  // Partial-reset helpers used when a save only partially succeeds (e.g. one
  // batch call in a Promise.allSettled fails): clear just the ids that were
  // confirmed saved, leaving the rest queued so the user can retry.
  const clearPendingAdds = (ids: Iterable<string>) => {
    const idSet = ids instanceof Set ? ids : new Set(ids);
    setPendingAdds((prev) => prev.filter((item) => !idSet.has(idOf(item))));
  };

  const clearRemovedIds = (ids: Iterable<string>) => {
    const idSet = ids instanceof Set ? ids : new Set(ids);
    setRemovedIds((prev) => {
      const next = new Set(prev);
      idSet.forEach((id) => next.delete(id));
      return next;
    });
  };

  const isDirty = pendingAdds.length > 0 || removedIds.size > 0;

  return {
    pendingAdds,
    removedIds,
    activeIds,
    pendingAddIds,
    excludedIds,
    handleAdd,
    handleRemove,
    reset,
    clearPendingAdds,
    clearRemovedIds,
    isDirty,
  };
}
