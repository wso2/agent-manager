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


import type { ListQuery, OrgPathParams } from "./common";

/** One model's prices: USD per 1M tokens as exact decimal strings, null when there is none. */
export interface ModelPrice {
  model: string;
  provider: string | null;
  inputPer1M: string | null;
  outputPer1M: string | null;
  cachedInputPer1M: string | null;
  maxInputTokens: number | null;
  maxOutputTokens: number | null;
}

/** The pricing catalog, one page of it. */
export interface PricingResponse {
  /** When the newest model was added; null with no prices. */
  updatedAt: string | null;
  /** Every provider in the catalog, sorted (not only those on the page). */
  providers: string[];
  /** Models matching the search and provider, before paging. */
  total: number;
  limit: number;
  offset: number;
  /** By model name. */
  models: ModelPrice[];
}

export type GetPricingPathParams = OrgPathParams;
export interface GetPricingQuery extends ListQuery {
  /** Part of the model name, ignoring case. */
  search?: string;
  /** Exactly this provider. */
  provider?: string;
  /** Order of the catalog: by model name (default) or by provider A to Z. */
  sort?: "model" | "provider";
}
