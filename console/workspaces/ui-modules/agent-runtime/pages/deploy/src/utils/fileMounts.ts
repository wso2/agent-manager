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

import type { FileMount } from "@agent-management-platform/types";

// File mounts are keyed by a stable client-side id rather than array index, so prepending a new
// blank row doesn't cause React to reuse an existing FileMountEditor instance (and its already
// resolved isEditing state) for the new item.
let nextFileMountId = 1;

export type FileMountRow = FileMount & { id: number };

export function seedFileMountRows(files: FileMount[] | undefined): FileMountRow[] {
  return (files ?? []).map((f) => ({ ...f, id: nextFileMountId++ }));
}

export function newFileMountRow(): FileMountRow {
  return { key: "", mountPath: "", value: "", id: nextFileMountId++ };
}

export function toFileMount(
  { key, mountPath, value, isSensitive, secretRef }: FileMountRow,
): FileMount {
  return { key, mountPath, value, isSensitive, secretRef };
}
