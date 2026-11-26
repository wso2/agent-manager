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

/** Shown where the catalog has no value. */
export const NO_VALUE = "-";

/** A price: the API's exact decimal string with a dollar sign ("2.50" -> "$2.50"). */
export function formatPrice(value: string | null | undefined): string {
  return value ? `$${value}` : NO_VALUE;
}

/** A token count with thousands separators (128000 -> "128,000"). */
/** When the catalog last got a model, in the viewer's time zone. */
export function formatUpdated(iso: string | null | undefined): string {
  if (!iso) return "";
  const date = new Date(iso);
  return Number.isNaN(date.getTime())
    ? ""
    : date.toLocaleString("en-US", { dateStyle: "medium", timeStyle: "short" });
}
