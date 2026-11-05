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

export interface ListEmptyStateInput {
  isLoading?: boolean;
  /** Item count *before* any client-side filtering. */
  count: number;
  /** Whether a search term or any other filter is currently narrowing the list. */
  isFiltered?: boolean;
}

/**
 * Whether a list holds nothing at all, as opposed to a filter that matched
 * nothing. Only the former should drop the search/filter toolbar — leaving it
 * in place is what lets someone clear a filter that hid everything.
 */
export function isListEmpty({
  isLoading = false,
  count,
  isFiltered = false,
}: ListEmptyStateInput): boolean {
  return !isLoading && count === 0 && !isFiltered;
}
