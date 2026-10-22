/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { describe, expect, it } from 'vitest';
import {
  catalogueListQuery,
  EMPTY_FILTERS,
  isFiltering,
  lastPageIndex,
  parsePageParam,
  readCatalogueFilters,
  writePage,
} from './catalogueQuery';

describe('catalogueQuery', () => {
  it.each<[string | null, number]>([
    ['-1', 0],
    ['1.5', 1],
    ['abc', 0],
    [null, 0],
    ['1abc', 0],
    ['', 0],
    [' 2 ', 2],
    ['3', 3],
  ])('parsePageParam(%j) → %i', (raw, expected) => {
    expect(parsePageParam(raw)).toBe(expected);
  });

  it.each([
    [0, 0],
    [20, 0],
    [24, 0],
    [25, 1],
    [50, 2],
  ])('lastPageIndex(%i) → %i', (total, expected) => {
    expect(lastPageIndex(total)).toBe(expected);
  });

  it('builds the request with undefined for empty values', () => {
    expect(catalogueListQuery(EMPTY_FILTERS, '', 2)).toEqual({
      kind: undefined,
      environment: undefined,
      project: undefined,
      status: undefined,
      q: undefined,
      limit: 24,
      offset: 48,
    });
    expect(
      catalogueListQuery({ ...EMPTY_FILTERS, kind: 'proxied' }, 'refund', 0),
    ).toEqual(
      expect.objectContaining({ kind: 'proxied', q: 'refund', limit: 24, offset: 0 }),
    );
  });

  it('reads the four filters raw', () => {
    expect(
      readCatalogueFilters(new URLSearchParams('kind=bogus&status=failed')),
    ).toEqual({ kind: 'bogus', environment: '', project: '', status: 'failed' });
  });

  it('isFiltering is true for a filter or a search', () => {
    expect(isFiltering(EMPTY_FILTERS, '')).toBe(false);
    expect(isFiltering({ ...EMPTY_FILTERS, status: 'failed' }, '')).toBe(true);
    expect(isFiltering(EMPTY_FILTERS, 'x')).toBe(true);
  });

  it('writePage omits page 0', () => {
    const params = new URLSearchParams('page=3&q=a');
    writePage(params, 0);
    expect(params.has('page')).toBe(false);
    writePage(params, 2);
    expect(params.get('page')).toBe('2');
    expect(params.get('q')).toBe('a');
  });
});
