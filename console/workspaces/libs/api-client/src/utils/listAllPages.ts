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

// The most items one list request returns.
const PAGE_SIZE = 100;

/**
 * Loads every page of a paginated list. The first page gives the total, so the
 * rest are requested together rather than one after another.
 */
export async function listAllPages<Page extends { total: number }, Item>(
  fetchPage: (query: { limit: number; offset: number }) => Promise<Page>,
  items: (page: Page) => Item[],
): Promise<{ first: Page; all: Item[] }> {
  const first = await fetchPage({ limit: PAGE_SIZE, offset: 0 });
  const offsets: number[] = [];
  for (let offset = items(first).length; offset < first.total; offset += PAGE_SIZE) {
    offsets.push(offset);
  }
  const rest = await Promise.all(offsets.map((offset) => fetchPage({ limit: PAGE_SIZE, offset })));
  const all = [...items(first)];
  for (const page of rest) {
    // Items deleted meanwhile can leave total short of what is there.
    if (items(page).length === 0) break;
    all.push(...items(page));
  }
  return { first, all };
}
