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

import { z } from 'zod';
import { INPUT_LIMITS } from '@agent-management-platform/types';

/** The backend's MaxResourceNameLength for agent names. */
export const AGENT_NAME_MAX_LENGTH = 25;

export const connectAgentSchema = z.object({
  displayName: z
    .string()
    .trim()
    .min(1, 'Name is required')
    .min(3, 'Name must be at least 3 characters')
    .max(100, 'Name must be at most 100 characters'),
  name: z
    .string()
    .trim()
    .max(AGENT_NAME_MAX_LENGTH, `Name must be at most ${AGENT_NAME_MAX_LENGTH} characters`)
    .refine(
      (v) => v === '' || /^[a-z][a-z0-9-]*[a-z0-9]$|^[a-z]$/.test(v),
      'Name must be lowercase alphanumeric or "-", start with a letter and end with a letter or digit',
    ),
  description: z
    .string()
    .trim()
    .max(INPUT_LIMITS.DESCRIPTION, `Description must be at most ${INPUT_LIMITS.DESCRIPTION} characters`)
    .optional(),
  // Per-entry validation is enforced inline by LabelsEditor (mirroring the
  // backend rules), so the schema only constrains the overall shape.
  labels: z.record(z.string(), z.string()).optional(),
});

export type ConnectAgentFormValues = z.infer<typeof connectAgentSchema>;
