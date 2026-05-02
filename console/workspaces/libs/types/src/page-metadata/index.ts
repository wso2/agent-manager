/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import type { ComponentType } from 'react';

export type IconType = ComponentType<{ size?: number | string; className?: string }>;
/**
 * Base fields shared by all page metadata shapes.
 */
export interface PageMetadataBase {
  title: string;
  description: string;
  icon: IconType;
  path?: string;
  component: ComponentType;
}

/**
 * Standard page metadata — includes per-level component variants.
 */
export interface PageMetadata extends PageMetadataBase {
  path: string;
  levels?: Record<string, ComponentType>;
}

/**
 * Metadata for a single entry inside EvalPageMetadata.pages.
 */
export interface EvalPageEntry {
  component: ComponentType;
  icon: IconType;
  title: string;
  description: string;
  path?: string;
}

/**
 * Eval page metadata — nested pages structure.
 */
export interface EvalPageMetadata {
  pages: {
    component: Record<string, EvalPageEntry>;
  };
}

/**
 * Combined type covering all page metadata shapes in the platform.
 */
export type AnyPageMetadata = PageMetadata | EvalPageMetadata;
