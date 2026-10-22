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

import type { A2ACatalogListQuery } from '@agent-management-platform/types';

export const PAGE_SIZE = 24;

export type AdvancedFilterKey = 'environment' | 'project' | 'status';

export const ADVANCED_FILTER_KEYS: readonly AdvancedFilterKey[] = [
  'environment',
  'project',
  'status',
];

export interface CatalogueFilters {
  kind: string;
  environment: string;
  project: string;
  status: string;
}

export const EMPTY_FILTERS: CatalogueFilters = {
  kind: '',
  environment: '',
  project: '',
  status: '',
};

export function readCatalogueFilters(params: URLSearchParams): CatalogueFilters {
  return {
    kind: params.get('kind') ?? '',
    environment: params.get('environment') ?? '',
    project: params.get('project') ?? '',
    status: params.get('status') ?? '',
  };
}

export function parsePageParam(raw: string | null): number {
  const page = Number(raw);
  if (!Number.isFinite(page) || page < 0) {
    return 0;
  }
  return Math.trunc(page);
}

export function lastPageIndex(total: number): number {
  return Math.max(0, Math.ceil(total / PAGE_SIZE) - 1);
}

export function isFiltering(filters: CatalogueFilters, search: string): boolean {
  return Object.values(filters).some((value) => value !== '') || search !== '';
}

export function catalogueListQuery(
  filters: CatalogueFilters,
  search: string,
  page: number,
): A2ACatalogListQuery {
  return {
    kind: (filters.kind || undefined) as A2ACatalogListQuery['kind'],
    environment: filters.environment || undefined,
    project: filters.project || undefined,
    status: (filters.status || undefined) as A2ACatalogListQuery['status'],
    q: search || undefined,
    limit: PAGE_SIZE,
    offset: page * PAGE_SIZE,
  };
}

export function setOrDelete(
  params: URLSearchParams,
  key: string,
  value: string,
): void {
  if (value === '') {
    params.delete(key);
  } else {
    params.set(key, value);
  }
}

export function writePage(params: URLSearchParams, page: number): void {
  setOrDelete(params, 'page', page > 0 ? String(page) : '');
}
