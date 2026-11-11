/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
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

// Exported so the create/edit inputs cap at exactly what these schemas
// accept, instead of a generic name limit that would let a user type past
// the cap and only learn about it on submit.
export const IDENTITY_NAME_MAX_LENGTH = 50;


// Shared Zod schemas + value types for the identity creation flows. Mirrors the
// canonical pattern used by add-new-project/src/form/schema.ts so these forms
// validate identically (field-level on change, whole-form on submit) via the
// shared useFormValidation hook.

export interface CreateRoleFormValues {
  name: string;
  description?: string;
}

export const createRoleSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, "Name is required")
    .max(IDENTITY_NAME_MAX_LENGTH, `Name must be at most ${IDENTITY_NAME_MAX_LENGTH} characters`),
  description: z
    .string()
    .trim()
    .max(INPUT_LIMITS.DESCRIPTION, `Description must be at most ${INPUT_LIMITS.DESCRIPTION} characters`)
    .optional(),
});

export interface CreateGroupFormValues {
  name: string;
  description?: string;
}

export const createGroupSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, "Name is required")
    .max(IDENTITY_NAME_MAX_LENGTH, `Name must be at most ${IDENTITY_NAME_MAX_LENGTH} characters`),
  description: z
    .string()
    .trim()
    .max(INPUT_LIMITS.DESCRIPTION, `Description must be at most ${INPUT_LIMITS.DESCRIPTION} characters`)
    .optional(),
});

export interface AddUserFormValues {
  username: string;
  password: string;
  firstName?: string;
  lastName?: string;
  email?: string;
}

export const addUserSchema = z.object({
  username: z
    .string()
    .trim()
    .min(1, "Username is required")
    .max(100, "Username must be at most 100 characters")
    .regex(/^\S+$/, "Username cannot contain spaces"),
  password: z
    .string()
    .min(1, "Password is required")
    .min(8, "Password must be at least 8 characters"),
  firstName: z.string().trim().optional(),
  lastName: z.string().trim().optional(),
  email: z
    .string()
    .trim()
    .optional()
    .refine((v) => !v || z.string().email().safeParse(v).success, {
      message: "Enter a valid email address",
    }),
});

export interface InviteUserFormValues {
  email: string;
}

export const inviteUserSchema = z.object({
  email: z
    .string()
    .trim()
    .min(1, "Email is required")
    .email("Enter a valid email address"),
});
