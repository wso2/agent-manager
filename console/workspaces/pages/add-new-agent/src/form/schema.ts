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
import type { InputInterfaceType } from '@agent-management-platform/types';

export type InterfaceType = InputInterfaceType;

// Base fields shared by both flows
const baseAgentFields = {
  displayName: z
    .string()
    .trim()
    .min(1, 'Name is required')
    .min(3, 'Name must be at least 3 characters')
    .max(100, 'Name must be at most 100 characters'),
  name: z
    .string()
    .trim()
    .min(1, 'Name is required')
    .regex(/^[a-z0-9-]+$/, 'Name must be lowercase letters, numbers, and hyphens only (no spaces)')
    .min(3, 'Name must be at least 3 characters')
    .max(50, 'Name must be at most 50 characters'),
  description: z.string().trim().optional(),
};

// Schema for connecting to an existing agent (minimal fields)
export const connectAgentSchema = z.object({
  ...baseAgentFields,
  deploymentType: z.literal('existing').optional(),
});

// Schema for creating a new agent from source (full validation)
export const createAgentSchema = z.object({
  ...baseAgentFields,
  deploymentType: z.literal('new').optional(),
  enableAutoInstrumentation: z.boolean().default(true),
  repositoryUrl: z
    .string()
    .trim()
    .min(1, 'Repository URL is required')
    .url('Must be a valid URL'),
  branch: z.string().trim().min(1, 'Branch is required'),
  appPath: z
    .string()
    .trim()
    .min(1, 'App path is required')
    .refine((value) => value.startsWith('/'), {
      message: 'App path must start with /',
    })
    .refine((value) => !/\.\./.test(value), {
      message: 'Path traversal is not allowed',
    })
    .refine((value) => /^\/[A-Za-z0-9._\-/]*$/.test(value), {
      message: 'App path can only contain letters, numbers, ., _, -, and /',
    })
    .refine(
      (value) => {
        if (value === '/') return true;
        return !value.endsWith('/');
      },
      { message: 'App path must be a valid path (use / for root directory)' }
    ),
  runCommand: z.string().trim().optional(),
  language: z.string().trim().min(1, 'Language is required'),
  languageVersion: z.string().trim().optional(),
  dockerfilePath: z.string().trim().optional(),
  interfaceType: z.enum(['DEFAULT', 'CUSTOM']),
  port: z
    .union([z.number(), z.string(), z.undefined()])
    .transform((val) => {
      if (val === '' || val === null || val === undefined) return undefined;
      return typeof val === 'string' ? Number(val) : val;
    })
    .optional(),
  basePath: z.string().trim().optional(),
  openApiPath: z.string().trim().optional(),
  env: z
    .array(
      z.object({
        key: z
          .string()
          .trim()
          .min(1, 'Environment variable key is required')
          .max(64, 'Environment variable key must be at most 64 characters')
          .regex(/^[A-Za-z_][A-Za-z0-9_]*$/, 'Env keys must match /^[A-Za-z_][A-Za-z0-9_]*$/')
          .optional(),
        value: z
          .string()
          .trim()
          .max(2048, 'Environment variable value must be at most 2048 characters')
          .optional(),
      })
    )
    .max(50, 'A maximum of 50 environment variables is allowed'),
}).refine(
  (data) => {
    if (data.interfaceType === 'CUSTOM' && !data.port) {
      return false;
    }
    return true;
  },
  { message: 'Port is required when using custom interface', path: ['port'] }
).refine(
  (data) => {
    if (data.interfaceType === 'CUSTOM' && data.port !== undefined) {
      if (!Number.isInteger(data.port)) return false;
      if (data.port < 1 || data.port > 65535) return false;
    }
    return true;
  },
  { message: 'Port must be between 1 and 65535', path: ['port'] }
).refine(
  (data) => {
    if (data.interfaceType === 'CUSTOM' && !data.basePath) {
      return false;
    }
    return true;
  },
  { message: 'Base path is required when using custom interface', path: ['basePath'] }
).refine(
  (data) => {
    if (data.interfaceType === 'CUSTOM' && !data.openApiPath) {
      return false;
    }
    return true;
  },
  { message: 'OpenAPI spec path is required when using custom interface', path: ['openApiPath'] }
).refine(
  (data) => {
    // Validate Python-specific fields: runCommand is required for Python
    if (data.language === 'python' && !data.runCommand?.trim()) {
      return false;
    }
    return true;
  },
  { message: 'Start Command is required for Python agents', path: ['runCommand'] }
).refine(
  (data) => {
    // Validate Python-specific fields: languageVersion is required for Python
    if (data.language === 'python' && !data.languageVersion?.trim()) {
      return false;
    }
    return true;
  },
  { message: 'Python version is required for Python agents', path: ['languageVersion'] }
).refine(
  (data) => {
    // Validate Docker-specific fields: dockerfilePath is required for Docker
    if (data.language === 'docker' && !data.dockerfilePath?.trim()) {
      return false;
    }
    return true;
  },
  { message: 'Dockerfile path is required for Docker agents', path: ['dockerfilePath'] }
).refine(
  (data) => {
    // Validate dockerfilePath must start with /
    if (data.language === 'docker' && data.dockerfilePath?.trim() && !data.dockerfilePath.startsWith('/')) {
      return false;
    }
    return true;
  },
  { message: 'Dockerfile path must start with /', path: ['dockerfilePath'] }
);

// Union type for form values
export type ConnectAgentFormValues = z.infer<typeof connectAgentSchema>;
export type CreateAgentFormValues = z.infer<typeof createAgentSchema>;
export type AddAgentFormValues = ConnectAgentFormValues | CreateAgentFormValues;


