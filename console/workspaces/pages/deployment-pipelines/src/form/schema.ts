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

import { z } from "zod";
import { INPUT_LIMITS } from '@agent-management-platform/types';

// Exported so the create/edit drawers cap their inputs at exactly what this
// schema accepts.
export const PIPELINE_DISPLAY_NAME_MAX_LENGTH = 128;


const pipelineSchema = z.object({
  displayName: z.string().min(1, "Display name is required").max(PIPELINE_DISPLAY_NAME_MAX_LENGTH, `Display name must be ${PIPELINE_DISPLAY_NAME_MAX_LENGTH} characters or less`),
  description: z
    .string()
    .max(INPUT_LIMITS.DESCRIPTION, `Description must be at most ${INPUT_LIMITS.DESCRIPTION} characters`)
    .optional(),
  chain: z
    .array(z.string().min(1, "Select an environment"))
    .min(1, "Select at least one environment"),
});

export const createPipelineSchema = pipelineSchema;
export type CreatePipelineFormValues = z.infer<typeof pipelineSchema>;

export const editPipelineSchema = pipelineSchema;
export type EditPipelineFormValues = z.infer<typeof pipelineSchema>;
